//! Content schema (cards and scenes) with full load-time validation,
//! ported 1:1 from the Go `internal/content` package.

use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::fmt;

use serde::{Deserialize, Serialize};

// Effect kinds.
pub const KIND_STAT: &str = "stat";
pub const KIND_GAIN_CARD: &str = "gain_card";
pub const KIND_LOSE_CARD: &str = "lose_card";
pub const KIND_REMOVE_CARD: &str = "remove_card";
pub const KIND_GOTO: &str = "goto";
pub const KIND_RANDOM: &str = "random";
pub const KIND_THREAT: &str = "threat";

// Stat keys usable in stat-effect deltas. The game engine maps each key to
// a player field, so adding one requires a matching case there.
pub const STAT_LEGACY: &str = "legacy";
pub const STAT_ARMY: &str = "army";
pub const STAT_TREASURY: &str = "treasury";

/// The closed set accepted in `Effect::delta` and `Choice::requires_stat`.
pub fn stat_keys() -> &'static BTreeSet<&'static str> {
    static KEYS: std::sync::LazyLock<BTreeSet<&'static str>> = std::sync::LazyLock::new(|| {
        let mut s = BTreeSet::new();
        s.insert(STAT_LEGACY);
        s.insert(STAT_ARMY);
        s.insert(STAT_TREASURY);
        s
    });
    &KEYS
}

/// The closed set of ending classifications for terminal scenes. The values
/// drive the UI badge and its styling.
pub fn endings() -> &'static BTreeSet<&'static str> {
    static KEYS: std::sync::LazyLock<BTreeSet<&'static str>> = std::sync::LazyLock::new(|| {
        let mut s = BTreeSet::new();
        for e in ["triumph", "legacy", "settle", "defeat", "death"] {
            s.insert(e);
        }
        s
    });
    &KEYS
}

/// The display order of ending classifications for galleries.
pub const ENDING_ORDER: [&str; 5] = ["triumph", "legacy", "settle", "defeat", "death"];

/// The final-battle scene the game engine forces when the threat track
/// maxes out. It is entered by rule rather than by a goto effect, so
/// validation seeds it as reachable.
pub const BATTLE_SCENE: &str = "battle";

/// One weighted branch of a random effect. Weight is relative likelihood
/// and must be at least 1.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize, Default)]
#[serde(deny_unknown_fields)]
pub struct Outcome {
    pub weight: i32,
    #[serde(default)]
    pub effects: Vec<Effect>,
}

/// One game-rule step. Kind selects which other field is used:
/// stat -> delta, gain_card/lose_card/remove_card -> card_id, goto -> next,
/// random -> outcomes, threat -> value.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize, Default)]
#[serde(deny_unknown_fields)]
pub struct Effect {
    pub kind: String,
    #[serde(default, rename = "cardId", skip_serializing_if = "String::is_empty")]
    pub card_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub delta: Option<BTreeMap<String, i32>>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub next: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub outcomes: Vec<Outcome>,
    #[serde(default, skip_serializing_if = "is_zero")]
    pub value: i32,
}

fn is_zero(v: &i32) -> bool {
    *v == 0
}

/// A playable card. Cost is the treasury paid to play it (0 is free).
/// Start marks cards dealt into the opening deck; everything else enters a
/// run only through scene pools or gain_card effects. Effects apply in
/// declared order.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize, Default)]
#[serde(deny_unknown_fields)]
pub struct Card {
    pub id: String,
    pub name: String,
    pub text: String,
    #[serde(default)]
    pub cost: i32,
    #[serde(default)]
    pub start: bool,
    #[serde(default)]
    pub effects: Vec<Effect>,
}

/// One option on a scene. RequiresCard and RequiresStat are minimum
/// requirements: the card must be in hand and every stat must be at or
/// above its threshold. ConsumesCard spends the required card to the
/// discard pile when the choice is taken.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize, Default)]
#[serde(deny_unknown_fields)]
pub struct Choice {
    pub text: String,
    #[serde(default, rename = "requiresCard", skip_serializing_if = "String::is_empty")]
    pub requires_card: String,
    #[serde(default, rename = "consumesCard", skip_serializing_if = "is_zero_bool")]
    pub consumes_card: bool,
    #[serde(default, rename = "requiresStat", skip_serializing_if = "Option::is_none")]
    pub requires_stat: Option<BTreeMap<String, i32>>,
    #[serde(default)]
    pub effects: Vec<Effect>,
}

fn is_zero_bool(v: &bool) -> bool {
    !*v
}

/// One narrative location with its choices. Cards is the regional pool
/// granted to the run on first arrival. Threat is the enemy pressure of
/// the region, added to the base advance after every spent turn. A scene
/// with a non-empty ending is terminal: it renders the run summary instead
/// of choices and must declare no choices.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize, Default)]
#[serde(deny_unknown_fields)]
pub struct Scene {
    pub id: String,
    pub text: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub ending: String,
    #[serde(default)]
    pub threat: i32,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub cards: Vec<String>,
    #[serde(default)]
    pub choices: Vec<Choice>,
}

/// Validated content: cards and scenes keyed by ID.
#[derive(Debug, Clone, Default)]
pub struct Library {
    pub cards: HashMap<String, Card>,
    pub scenes: HashMap<String, Scene>,
}

impl Library {
    /// All cards sorted by ID for deterministic iteration.
    pub fn card_list(&self) -> Vec<Card> {
        let mut ids: Vec<&String> = self.cards.keys().collect();
        ids.sort();
        ids.into_iter().map(|id| self.cards[id].clone()).collect()
    }

    /// The opening deck: start-flagged cards, sorted by ID for a
    /// deterministic shuffle base.
    pub fn starting_cards(&self) -> Vec<Card> {
        let mut ids: Vec<&String> = self.cards.keys().collect();
        ids.sort();
        ids.into_iter()
            .filter(|id| self.cards[*id].start)
            .map(|id| self.cards[id].clone())
            .collect()
    }
}

/// Load error: either a decode failure (reported immediately) or the
/// aggregated validation report.
#[derive(Debug)]
pub enum LoadError {
    /// Bad JSON, an unknown field, or a missing content file.
    Decode(String),
    /// Every validation violation, one per line.
    Invalid(String),
}

impl fmt::Display for LoadError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            LoadError::Decode(m) => write!(f, "{m}"),
            LoadError::Invalid(m) => write!(f, "{m}"),
        }
    }
}

impl std::error::Error for LoadError {}

/// Load decodes the cards and scenes JSON strings and validates referential
/// integrity. It returns one error listing every violation.
pub fn load(cards_json: &str, scenes_json: &str) -> Result<Library, LoadError> {
    let cards: Vec<Card> = decode("cards.json", cards_json)?;
    let scenes: Vec<Scene> = decode("scenes.json", scenes_json)?;
    validate(cards, scenes)
}

/// Loads the shipped campaign content embedded in the binary.
pub fn load_embedded() -> Library {
    load(crate::CARDS_JSON, crate::SCENES_JSON).expect("shipped content validates")
}

fn decode<T: serde::de::DeserializeOwned>(name: &str, data: &str) -> Result<T, LoadError> {
    serde_json::from_str(data).map_err(|e| LoadError::Decode(format!("{name}: {e}")))
}

fn validate(cards: Vec<Card>, scenes: Vec<Scene>) -> Result<Library, LoadError> {
    let mut lib = Library::default();
    let mut probs: Vec<String> = Vec::new();

    for c in &cards {
        if c.id.is_empty() {
            probs.push("card with empty id".into());
            continue;
        }
        if lib.cards.contains_key(&c.id) {
            probs.push(format!("duplicate card id {:?}", c.id));
            continue;
        }
        if c.cost < 0 {
            probs.push(format!("card {:?}: negative cost {}", c.id, c.cost));
        }
        lib.cards.insert(c.id.clone(), c.clone());
    }

    for sc in &scenes {
        if sc.id.is_empty() {
            probs.push("scene with empty id".into());
            continue;
        }
        if lib.scenes.contains_key(&sc.id) {
            probs.push(format!("duplicate scene id {:?}", sc.id));
            continue;
        }
        lib.scenes.insert(sc.id.clone(), sc.clone());
    }

    for c in &cards {
        for (i, e) in c.effects.iter().enumerate() {
            if let Err(err) = validate_effect(e, &lib) {
                probs.push(format!("card {:?} effect {i}: {err}", c.id));
            }
        }
    }
    for sc in &scenes {
        if !sc.ending.is_empty() {
            if !endings().contains(sc.ending.as_str()) {
                probs.push(format!("scene {:?}: unknown ending {:?}", sc.id, sc.ending));
            }
            if !sc.choices.is_empty() {
                probs.push(format!("ending scene {:?} must have no choices", sc.id));
            }
        } else if sc.choices.is_empty() {
            probs.push(format!("scene {:?} has no choices (dead end)", sc.id));
        } else if !has_ungated_choice(&sc.choices) {
            probs.push(format!(
                "scene {:?}: every choice is gated; a run must always have an escape path",
                sc.id
            ));
        }
        for id in &sc.cards {
            if !lib.cards.contains_key(id) {
                probs.push(format!("scene {:?} card pool: unknown card {:?}", sc.id, id));
            }
        }
        for (i, ch) in sc.choices.iter().enumerate() {
            if !ch.requires_card.is_empty() {
                if !lib.cards.contains_key(&ch.requires_card) {
                    probs.push(format!(
                        "scene {:?} choice {i}: requires unknown card {:?}",
                        sc.id, ch.requires_card
                    ));
                }
            } else if ch.consumes_card {
                probs.push(format!(
                    "scene {:?} choice {i}: consumesCard requires a requiresCard",
                    sc.id
                ));
            }
            for k in ch.requires_stat.iter().flat_map(|m| m.keys()) {
                if !stat_keys().contains(k.as_str()) {
                    probs.push(format!(
                        "scene {:?} choice {i}: requiresStat with unknown stat {:?}",
                        sc.id, k
                    ));
                }
            }
            for (j, e) in ch.effects.iter().enumerate() {
                if let Err(err) = validate_effect(e, &lib) {
                    probs.push(format!("scene {:?} choice {i} effect {j}: {err}", sc.id));
                }
            }
        }
    }
    for id in unobtainable(&lib, &cards, &scenes) {
        probs.push(format!(
            "card {id:?} is unobtainable: not a start card, in no scene pool, and granted by nothing reachable"
        ));
    }
    match lib.scenes.get("title") {
        None => probs.push(r#"no "title" scene"#.into()),
        Some(_) => {
            for id in unreachable(&lib, &cards) {
                probs.push(format!("scene {id:?} is unreachable from title"));
            }
        }
    }

    if !probs.is_empty() {
        return Err(LoadError::Invalid(format!(
            "invalid content:\n  - {}",
            probs.join("\n  - ")
        )));
    }
    Ok(lib)
}

fn validate_effect(e: &Effect, lib: &Library) -> Result<(), String> {
    match e.kind.as_str() {
        KIND_STAT => {
            let empty = BTreeMap::new();
            let delta = e.delta.as_ref().unwrap_or(&empty);
            if delta.is_empty() {
                return Err("stat effect with empty delta".into());
            }
            for k in delta.keys() {
                if !stat_keys().contains(k.as_str()) {
                    return Err(format!("stat effect with unknown stat {k:?}"));
                }
            }
        }
        KIND_GAIN_CARD | KIND_LOSE_CARD | KIND_REMOVE_CARD => {
            if !lib.cards.contains_key(&e.card_id) {
                return Err(format!("{} references unknown card {:?}", e.kind, e.card_id));
            }
        }
        KIND_GOTO => {
            if !lib.scenes.contains_key(&e.next) {
                return Err(format!("goto references unknown scene {:?}", e.next));
            }
        }
        KIND_THREAT => {
            if e.value == 0 {
                return Err("threat effect with zero value".into());
            }
        }
        KIND_RANDOM => {
            if e.outcomes.is_empty() {
                return Err("random effect with no outcomes".into());
            }
            let mut errs = Vec::new();
            for (i, o) in e.outcomes.iter().enumerate() {
                if o.weight < 1 {
                    errs.push(format!("random outcome {i}: weight must be >= 1"));
                    continue;
                }
                for (j, sub) in o.effects.iter().enumerate() {
                    if let Err(err) = validate_effect(sub, lib) {
                        errs.push(format!("random outcome {i} effect {j}: {err}"));
                    }
                }
            }
            if !errs.is_empty() {
                return Err(errs.join("\n"));
            }
        }
        other => return Err(format!("unknown kind {other:?}")),
    }
    Ok(())
}

/// Reports whether at least one choice needs no card and no stat, keeping
/// the scene escapable whatever the run's luck.
fn has_ungated_choice(choices: &[Choice]) -> bool {
    choices.iter().any(|ch| {
        ch.requires_card.is_empty() && ch.requires_stat.as_ref().is_none_or(|m| m.is_empty())
    })
}

/// Adds every card id gained by effects, recursing through random
/// outcomes, to `out`.
fn granted_cards(effects: &[Effect], out: &mut HashSet<String>) {
    for e in effects {
        match e.kind.as_str() {
            KIND_GAIN_CARD => {
                out.insert(e.card_id.clone());
            }
            KIND_RANDOM => {
                for o in &e.outcomes {
                    granted_cards(&o.effects, out);
                }
            }
            _ => {}
        }
    }
}

/// Returns the set of cards a run can never acquire. Seeds are start cards
/// and scene pools; scene-choice grants always count (their scenes are
/// reachability-checked); card-effect grants count only when the granting
/// card is itself obtainable, so the closure runs to a fixpoint.
fn unobtainable(lib: &Library, cards: &[Card], scenes: &[Scene]) -> BTreeSet<String> {
    let mut have: HashSet<String> = HashSet::new();
    for c in cards {
        if c.start {
            have.insert(c.id.clone());
        }
    }
    for sc in scenes {
        for id in &sc.cards {
            have.insert(id.clone());
        }
        for ch in &sc.choices {
            granted_cards(&ch.effects, &mut have);
        }
    }
    loop {
        let mut changed = false;
        let mut granted = HashSet::new();
        for id in &have {
            if let Some(c) = lib.cards.get(id) {
                granted_cards(&c.effects, &mut granted);
            }
        }
        for t in granted {
            if have.insert(t) {
                changed = true;
            }
        }
        if !changed {
            break;
        }
    }
    cards
        .iter()
        .filter(|c| !have.contains(&c.id))
        .map(|c| c.id.clone())
        .collect()
}

/// Adds every scene id referenced by effects, recursing through random
/// outcomes, to `out`.
fn goto_targets(effects: &[Effect], out: &mut HashSet<String>) {
    for e in effects {
        match e.kind.as_str() {
            KIND_GOTO => {
                out.insert(e.next.clone());
            }
            KIND_RANDOM => {
                for o in &e.outcomes {
                    goto_targets(&o.effects, out);
                }
            }
            _ => {}
        }
    }
}

/// Returns the set of scenes that cannot be reached from title. Edges are
/// scene choices; card effects may be played from any scene, so their goto
/// targets count as edges from everywhere.
fn unreachable(lib: &Library, cards: &[Card]) -> BTreeSet<String> {
    let mut card_edges = HashSet::new();
    for c in cards {
        goto_targets(&c.effects, &mut card_edges);
    }
    let mut reached = HashSet::new();
    let mut queue: Vec<String> = vec!["title".into(), BATTLE_SCENE.into()];
    while let Some(id) = queue.pop() {
        if reached.contains(&id) {
            continue;
        }
        reached.insert(id.clone());
        let mut next = HashSet::new();
        if let Some(sc) = lib.scenes.get(&id) {
            for ch in &sc.choices {
                goto_targets(&ch.effects, &mut next);
            }
        }
        next.extend(card_edges.iter().cloned());
        for t in next {
            if !reached.contains(&t) {
                queue.push(t);
            }
        }
    }
    lib.scenes
        .keys()
        .filter(|id| !reached.contains(*id))
        .cloned()
        .collect()
}

