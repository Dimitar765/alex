//! The rules engine: player state, scene choices, and card play. Ported
//! 1:1 from the Go `internal/game` package; this is the only place game
//! rules live.

use std::collections::BTreeMap;
use std::collections::HashMap;

use rand::rngs::StdRng;
use rand::{Rng, SeedableRng};
use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::content::{
    self, Choice, Effect, Library, STAT_ARMY, STAT_LEGACY, STAT_TREASURY,
};

/// The number of cards the hand refills to.
pub const HAND_SIZE: usize = 4;
/// The treasury paid to shuffle the discard pile home.
pub const SHUFFLE_COST: i32 = 1;
/// Bounds the human-readable event log.
pub const MAX_LOG_LEN: usize = 50;
/// The Persian response: the campaign pressure cap, reached by base
/// advance + scene pressure after every spent turn. Crossing
/// [`THREAT_RAID`] triggers a raid, [`THREAT_AMBUSH`] an ambush; the max
/// forces the battle scene ([`content::BATTLE_SCENE`]), whose choices
/// decide the campaign's fate.
pub const THREAT_MAX: i32 = 20;
pub const THREAT_RAID: i32 = 8;
pub const THREAT_AMBUSH: i32 = 14;

/// Player-facing refusals. Display strings are byte-identical to the Go
/// engine's.
#[derive(Debug, Clone, PartialEq, Error)]
pub enum GameError {
    #[error("{0} is not in your hand")]
    NotInHand(String),
    #[error("you cannot afford {name} (costs {cost} treasury)")]
    CannotAfford { name: String, cost: i32 },
    #[error("nothing to shuffle — the discard pile is empty")]
    ShuffleEmpty,
    #[error("you cannot afford to shuffle (costs {cost} treasury)")]
    ShuffleCost { cost: i32 },
    #[error("the deck is empty — nothing to scout")]
    PeekEmpty,
    #[error("that path requires {0}")]
    RequiresCard(String),
    #[error("that path requires {label} {want} (you have {have})")]
    RequiresStat { label: String, want: i32, have: i32 },
    #[error("unknown stat {0:?}")]
    UnknownStat(String),
    #[error("unknown effect kind {0:?}")]
    UnknownKind(String),
}

/// The tracked resources. They may go negative by design.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub struct Stats {
    #[serde(rename = "Legacy", default)]
    pub legacy: i32,
    #[serde(rename = "Army", default)]
    pub army: i32,
    #[serde(rename = "Treasury", default)]
    pub treasury: i32,
}

/// One player's full game state. It is plain data so it serializes to the
/// save directory as JSON, byte-compatible with the Go desktop saves.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct State {
    #[serde(rename = "Session", default)]
    pub session: String,
    #[serde(rename = "SceneID", default)]
    pub scene_id: String,
    #[serde(rename = "Stats", default)]
    pub stats: Stats,
    /// The campaign pressure track. Omitted from saves when zero, matching
    /// the Go `json:"threat,omitempty"` tag.
    #[serde(rename = "threat", default, skip_serializing_if = "is_zero")]
    pub threat: i32,
    #[serde(
        rename = "Hand",
        default,
        deserialize_with = "null_to_empty",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub hand: Vec<String>,
    /// Draw pile; the last element is the top.
    #[serde(
        rename = "Deck",
        default,
        deserialize_with = "null_to_empty",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub deck: Vec<String>,
    #[serde(
        rename = "Discard",
        default,
        deserialize_with = "null_to_empty",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub discard: Vec<String>,
    #[serde(
        rename = "Log",
        default,
        deserialize_with = "null_to_empty",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub log: Vec<String>,
    /// Scenes whose card pools have been granted.
    #[serde(
        rename = "Granted",
        default,
        deserialize_with = "null_to_empty",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub granted: Vec<String>,
    /// Successful actions taken (choices and card plays).
    #[serde(rename = "Turns", default)]
    pub turns: i32,
    /// Card plays among those turns.
    #[serde(rename = "CardsPlayed", default)]
    pub cards_played: i32,
    /// Whether the most recent action resolved a random effect; a transient
    /// presentation hint, never persisted.
    #[serde(skip)]
    pub rolled: bool,
    /// Deterministic source for shuffles and random effects. Not
    /// persisted; a loaded save reseeds from entropy like the Go engine's
    /// nil-rng fallback to the global source.
    #[serde(skip, default = "default_rng")]
    pub rng: StdRng,
}

fn is_zero(v: &i32) -> bool {
    *v == 0
}

/// Deserializes `null` as an empty collection, so Go-written saves with
/// nil slices load unchanged.
fn null_to_empty<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Default + serde::Deserialize<'de>,
{
    Ok(Option::<T>::deserialize(deserializer)?.unwrap_or_default())
}

impl Default for State {
    fn default() -> Self {
        State {
            session: String::new(),
            scene_id: String::new(),
            stats: Stats::default(),
            threat: 0,
            hand: Vec::new(),
            deck: Vec::new(),
            discard: Vec::new(),
            log: Vec::new(),
            granted: Vec::new(),
            turns: 0,
            cards_played: 0,
            rolled: false,
            rng: default_rng(),
        }
    }
}

/// The entropy-seeded source used for states that were not constructed
/// with an explicit RNG (loaded saves, `State::default`).
fn default_rng() -> StdRng {
    StdRng::from_os_rng()
}

impl State {
    /// Builds a fresh state from the library's starting cards: a shuffled
    /// opening deck and a hand of [`HAND_SIZE`].
    pub fn new(lib: &Library, start_scene: &str, rng: StdRng) -> State {
        let mut s = State {
            scene_id: start_scene.to_string(),
            rng,
            ..State::default()
        };
        for c in lib.starting_cards() {
            s.deck.push(c.id);
        }
        shuffle(&mut s.rng, &mut s.deck);
        s.draw_up();
        s
    }

    /// A random value in `[0, n)` from the state's random source.
    fn int_n(&mut self, n: usize) -> usize {
        self.rng.random_range(0..n)
    }

    /// Applies a scene choice. It fails if the choice's card or stat
    /// requirements are not met. A consuming choice spends its required
    /// card to the discard pile. Effects apply in declared order; arriving
    /// at a new scene grants its card pool once; the hand then refills.
    pub fn choose(&mut self, choice: &Choice, lib: &Library) -> Result<(), GameError> {
        self.rolled = false;
        if let Some(err) = self.requirement_error(choice, Some(lib)) {
            return Err(err);
        }
        let mut parts = vec![choice.text.clone()];
        if choice.consumes_card {
            take_card(&choice.requires_card, [&mut self.hand]);
            self.discard.push(choice.requires_card.clone());
            parts.push(format!(
                "spent {}",
                display_name(&choice.requires_card, Some(&lib.cards))
            ));
        }
        let before = self.scene_id.clone();
        self.apply_effects(&choice.effects, lib, &mut parts)?;
        if self.scene_id != before {
            self.arrive(lib, &mut parts);
        }
        self.draw_up();
        self.end_turn(lib, &mut parts);
        self.turns += 1;
        self.append_log(&join_parts(&parts));
        Ok(())
    }

    /// Removes a card from the hand to the discard pile, pays its cost,
    /// applies its effects in order, and refills the hand. It fails if the
    /// card is not in hand or the treasury cannot cover the cost.
    pub fn play_card(&mut self, id: &str, lib: &Library) -> Result<(), GameError> {
        self.rolled = false;
        let Some(i) = self.hand.iter().position(|h| h == id) else {
            return Err(GameError::NotInHand(display_name(id, Some(&lib.cards))));
        };
        let card = &lib.cards[id];
        if self.stats.treasury < card.cost {
            return Err(GameError::CannotAfford {
                name: display_name(id, Some(&lib.cards)),
                cost: card.cost,
            });
        }
        self.hand.remove(i);
        self.discard.push(id.to_string());
        let mut parts = vec![format!("Played {}", display_name(id, Some(&lib.cards)))];
        if card.cost > 0 {
            self.stats.treasury -= card.cost;
            parts.push(format!("Treasury -{}", card.cost));
        }
        let before = self.scene_id.clone();
        self.apply_effects(&card.effects, lib, &mut parts)?;
        if self.scene_id != before {
            self.arrive(lib, &mut parts);
        }
        self.draw_up();
        self.end_turn(lib, &mut parts);
        self.turns += 1;
        self.cards_played += 1;
        self.append_log(&join_parts(&parts));
        Ok(())
    }

    /// Whether the card is in hand and affordable right now.
    pub fn can_play(&self, id: &str, cards: &HashMap<String, crate::content::Card>) -> bool {
        if !self.hand.iter().any(|h| h == id) {
            return false;
        }
        let cost = cards.get(id).map(|c| c.cost).unwrap_or(0);
        self.stats.treasury >= cost
    }

    /// Pays [`SHUFFLE_COST`] treasury and spends the turn recycling the
    /// discard pile into the deck. It fails when there is nothing to
    /// shuffle or the treasury cannot cover the cost.
    pub fn shuffle(&mut self, lib: &Library) -> Result<(), GameError> {
        self.rolled = false;
        if self.discard.is_empty() {
            return Err(GameError::ShuffleEmpty);
        }
        if self.stats.treasury < SHUFFLE_COST {
            return Err(GameError::ShuffleCost {
                cost: SHUFFLE_COST,
            });
        }
        self.stats.treasury -= SHUFFLE_COST;
        self.deck.append(&mut self.discard);
        shuffle(&mut self.rng, &mut self.deck);
        let mut parts = vec![
            "Shuffled the discard pile into the deck".to_string(),
            format!("Treasury -{SHUFFLE_COST}"),
        ];
        self.end_turn(lib, &mut parts);
        self.turns += 1;
        self.append_log(&join_parts(&parts));
        Ok(())
    }

    /// Spends the turn revealing the deck's top card, recorded in the log.
    /// It fails when the deck is empty.
    pub fn peek(&mut self, lib: &Library) -> Result<String, GameError> {
        self.rolled = false;
        let Some(top) = self.deck.last().cloned() else {
            return Err(GameError::PeekEmpty);
        };
        let mut parts = vec!["Scouted the deck".to_string()];
        self.end_turn(lib, &mut parts);
        self.turns += 1;
        self.append_log(&format!(
            "{} Next card is {}",
            join_parts(&parts),
            display_name(&top, Some(&lib.cards))
        ));
        Ok(top)
    }

    /// Grants the entered scene's regional card pool on first entry,
    /// stacking the cards on the deck so they are drawn first.
    fn arrive(&mut self, lib: &Library, parts: &mut Vec<String>) {
        let Some(sc) = lib.scenes.get(&self.scene_id) else {
            return;
        };
        if sc.cards.is_empty() || self.granted.contains(&sc.id) {
            return;
        }
        self.granted.push(sc.id.clone());
        for id in &sc.cards {
            self.deck.push(id.clone());
            parts.push(format!("gained {}", display_name(id, Some(&lib.cards))));
        }
    }

    /// The Persian response: after every spent turn the enemy host
    /// advances by one plus the current scene's pressure, raiding the
    /// baggage train at [`THREAT_RAID`], ambushing the line at
    /// [`THREAT_AMBUSH`], and forcing the battle scene once the track
    /// maxes out. Deterministic by design. Terminal scenes are exempt —
    /// nothing moves after the credits roll.
    fn end_turn(&mut self, lib: &Library, parts: &mut Vec<String>) {
        let scene = lib
            .scenes
            .get(&self.scene_id)
            .cloned()
            .unwrap_or_default();
        if !scene.ending.is_empty() {
            return;
        }
        let before = self.threat;
        self.threat += 1 + scene.threat;
        self.threat = self.threat.clamp(0, THREAT_MAX);
        if self.threat > before {
            parts.push(format!("Threat +{}", self.threat - before));
        }
        if before < THREAT_RAID && self.threat >= THREAT_RAID {
            self.stats.treasury -= 1;
            parts.push("the Persians raid your baggage train (Treasury -1)".to_string());
        }
        if before < THREAT_AMBUSH && self.threat >= THREAT_AMBUSH && !self.hand.is_empty() {
            let i = self.int_n(self.hand.len());
            let id = self.hand.remove(i);
            self.discard.push(id.clone());
            parts.push(format!(
                "an ambush scatters {}",
                display_name(&id, Some(&lib.cards))
            ));
        }
        if self.threat >= THREAT_MAX && self.scene_id != content::BATTLE_SCENE {
            self.scene_id = content::BATTLE_SCENE.to_string();
            self.arrive(lib, parts);
            parts.push("the Persian host closes in — battle is joined".to_string());
        }
    }

    /// Whether the current hand and stats meet every requirement on the
    /// choice.
    pub fn can_choose(&self, c: &Choice) -> bool {
        self.requirement_error(c, None).is_none()
    }

    /// The first unmet requirement as a player-facing error, or `None`
    /// when the choice is available.
    fn requirement_error(&self, c: &Choice, lib: Option<&Library>) -> Option<GameError> {
        let cards = lib.map(|l| &l.cards);
        if !c.requires_card.is_empty() && !self.hand.iter().any(|h| h == &c.requires_card) {
            return Some(GameError::RequiresCard(display_name(
                &c.requires_card,
                cards,
            )));
        }
        let no_stats = BTreeMap::new();
        let stats = c.requires_stat.as_ref().unwrap_or(&no_stats);
        for (k, want) in stats {
            let have = self.stat(k);
            if have < *want {
                return Some(GameError::RequiresStat {
                    label: stat_label(k),
                    want: *want,
                    have,
                });
            }
        }
        None
    }

    /// The current value of a stat key, or 0 for unknown keys.
    pub fn stat(&self, k: &str) -> i32 {
        match k {
            STAT_LEGACY => self.stats.legacy,
            STAT_ARMY => self.stats.army,
            STAT_TREASURY => self.stats.treasury,
            _ => 0,
        }
    }

    fn apply_effects(
        &mut self,
        effects: &[Effect],
        lib: &Library,
        parts: &mut Vec<String>,
    ) -> Result<(), GameError> {
        for e in effects {
            self.apply(e, lib, parts)?;
        }
        Ok(())
    }

    fn apply(&mut self, e: &Effect, lib: &Library, parts: &mut Vec<String>) -> Result<(), GameError> {
        match e.kind.as_str() {
            content::KIND_STAT => {
                let empty = BTreeMap::new();
                let delta = e.delta.as_ref().unwrap_or(&empty);
                for (k, v) in delta {
                    match k.as_str() {
                        STAT_LEGACY => self.stats.legacy += v,
                        STAT_ARMY => self.stats.army += v,
                        STAT_TREASURY => self.stats.treasury += v,
                        _ => return Err(GameError::UnknownStat(k.clone())),
                    }
                    parts.push(format!("{} {:+}", stat_label(k), v));
                }
            }
            content::KIND_GAIN_CARD => {
                // Deck top = last element, so an appended card is drawn first.
                self.deck.push(e.card_id.clone());
                parts.push(format!(
                    "gained {}",
                    display_name(&e.card_id, Some(&lib.cards))
                ));
            }
            content::KIND_LOSE_CARD => {
                // Lost for now: the card surfaces again when the discard cycles.
                if take_card(&e.card_id, [&mut self.hand, &mut self.deck]) {
                    self.discard.push(e.card_id.clone());
                    parts.push(format!(
                        "lost {}",
                        display_name(&e.card_id, Some(&lib.cards))
                    ));
                }
            }
            content::KIND_REMOVE_CARD => {
                if take_card(
                    &e.card_id,
                    [&mut self.hand, &mut self.deck, &mut self.discard],
                ) {
                    parts.push(format!(
                        "{} is gone for good",
                        display_name(&e.card_id, Some(&lib.cards))
                    ));
                }
            }
            content::KIND_GOTO => {
                self.scene_id = e.next.clone();
            }
            content::KIND_RANDOM => {
                self.rolled = true;
                let total: i32 = e.outcomes.iter().map(|o| o.weight).sum();
                let mut pick = self.int_n(total.max(1) as usize) as i32;
                for o in &e.outcomes {
                    if pick < o.weight {
                        return self.apply_effects(&o.effects, lib, parts);
                    }
                    pick -= o.weight;
                }
            }
            content::KIND_THREAT => {
                self.threat += e.value;
                self.threat = self.threat.clamp(0, THREAT_MAX);
                parts.push(format!("Threat {:+}", e.value));
            }
            other => return Err(GameError::UnknownKind(other.to_string())),
        }
        Ok(())
    }

    /// Refills the hand to [`HAND_SIZE`], reshuffling the discard pile
    /// into the deck when the deck runs dry.
    fn draw_up(&mut self) {
        while self.hand.len() < HAND_SIZE {
            if self.deck.is_empty() {
                if self.discard.is_empty() {
                    return;
                }
                self.deck = std::mem::take(&mut self.discard);
                shuffle(&mut self.rng, &mut self.deck);
            }
            self.hand.push(self.deck.pop().expect("deck non-empty"));
        }
    }

    fn append_log(&mut self, line: &str) {
        self.log.push(line.to_string());
        if self.log.len() > MAX_LOG_LEN {
            let drop = self.log.len() - MAX_LOG_LEN;
            self.log.drain(0..drop);
        }
    }

    /// The current scene's terminal classification, if any.
    pub fn scene_ending<'a>(&self, lib: &'a Library) -> Option<&'a str> {
        lib.scenes
            .get(&self.scene_id)
            .map(|sc| sc.ending.as_str())
            .filter(|e| !e.is_empty())
    }
}

/// Removes one copy of `id` from the first pile that holds it and reports
/// whether a copy was found.
fn take_card<'a, I>(id: &str, piles: I) -> bool
where
    I: IntoIterator<Item = &'a mut Vec<String>>,
{
    for p in piles {
        if let Some(i) = p.iter().position(|c| c == id) {
            p.remove(i);
            return true;
        }
    }
    false
}

/// Randomizes the slice via Fisher–Yates, descending, using the given
/// random source.
fn shuffle<T>(rng: &mut StdRng, items: &mut [T]) {
    for i in (1..items.len()).rev() {
        let j = rng.random_range(0..=i);
        items.swap(i, j);
    }
}

/// `parts[0] + "."` then the `", "`-joined remainder + `"."`.
pub fn join_parts(parts: &[String]) -> String {
    let mut line = format!("{}.", parts[0]);
    if parts.len() > 1 {
        line += &format!(" {}.", parts[1..].join(", "));
    }
    line
}

/// The display name for a stat key.
pub fn stat_label(k: &str) -> String {
    match k {
        STAT_LEGACY => "Legacy".to_string(),
        STAT_ARMY => "Army".to_string(),
        STAT_TREASURY => "Treasury".to_string(),
        _ => k.to_string(),
    }
}

/// The card's name when known and non-empty; the raw id otherwise.
pub fn display_name(id: &str, cards: Option<&HashMap<String, crate::content::Card>>) -> String {
    if let Some(c) = cards.and_then(|m| m.get(id))
        && !c.name.is_empty()
    {
        return c.name.clone();
    }
    id.to_string()
}
