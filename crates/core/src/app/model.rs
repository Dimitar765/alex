//! The UI-agnostic view-model: it wraps the game rules and content with
//! everything a client needs to draw and act — projected views, structured
//! FX events, run history, and persistence.

use std::collections::BTreeMap;
use std::path::PathBuf;

use rand::rngs::StdRng;
use rand::SeedableRng;

use crate::app::events::Effect;
use crate::app::save::{self, RunSummary};
use crate::app::view::{Chronicle, EndingCount, EndingTile, View};
use crate::content::{self, Library, STAT_ARMY, STAT_LEGACY, STAT_TREASURY};
use crate::game::{self, GameError, State};

/// Format used when rendering run dates in the chronicle.
pub const CHRONICLE_DATE_FMT: &str = "%Y-%m-%d";

/// The ID of the campaign's opening scene.
pub const START_SCENE: &str = "title";


/// Player-facing refusals beyond the engine's own errors.
#[derive(Debug, Clone, PartialEq, thiserror::Error)]
pub enum AppError {
    #[error("{0}")]
    Engine(#[from] GameError),
    #[error("no campaign is underway")]
    NoRun,
    #[error("the campaign is over — start a new game")]
    CampaignOver,
    #[error("no such choice")]
    NoSuchChoice,
}

/// The outcome of an action: the projected view (always present,
/// reflecting the unchanged state on refusal), the engine error (`None` on
/// success), and the derived FX effects (empty on refusal).
#[derive(Debug, Clone)]
pub struct Result_ {
    pub view: View,
    pub err: Option<AppError>,
    pub effects: Vec<Effect>,
}

/// One local player's game: the live state (`None` until a campaign
/// starts), completed-run history, and the save directory (`None` keeps
/// everything in memory, which tests use).
pub struct Model {
    lib: Library,
    st: Option<State>,
    runs: Vec<RunSummary>,
    dir: Option<PathBuf>,
    start_scene: String,
}

impl Model {
    /// Builds a model persisting to `dir`. An existing save and history
    /// are loaded; a corrupt pair reads as empty rather than failing.
    pub fn new(lib: Library, dir: Option<PathBuf>) -> Model {
        let st = dir.as_deref().and_then(save::load_state);
        let runs = dir.as_deref().map(save::load_history).unwrap_or_default();
        Model {
            lib,
            st,
            runs,
            dir,
            start_scene: START_SCENE.to_string(),
        }
    }

    /// Whether a campaign is underway (even one that ended).
    pub fn has_run(&self) -> bool {
        self.st.is_some()
    }

    /// The live state, for clients that need engine-level access
    /// (e.g. deterministic replays in tests).
    pub fn state(&self) -> Option<&State> {
        self.st.as_ref()
    }

    /// The completed runs, oldest first.
    pub fn history(&self) -> &[RunSummary] {
        &self.runs
    }

    /// Starts a fresh campaign, replacing any current one. An abandoned
    /// run is not recorded in history — only endings are.
    pub fn new_game(&mut self) -> Result_ {
        self.st = Some(State::new(
            &self.lib,
            &self.start_scene,
            StdRng::from_os_rng(),
        ));
        self.persist_state();
        Result_ {
            view: self.projected(self.st.as_ref(), None),
            err: None,
            effects: Vec::new(),
        }
    }

    /// Plays a hand card by ID.
    pub fn play_card(&mut self, id: &str) -> Result_ {
        self.act(|lib, st| st.play_card(id, lib).map_err(AppError::Engine))
    }

    /// Takes the scene choice at index `i`.
    pub fn choose(&mut self, i: usize) -> Result_ {
        self.act(|lib, st| {
            let scene = lib.scenes.get(&st.scene_id).cloned().unwrap_or_default();
            let Some(choice) = scene.choices.get(i) else {
                return Err(AppError::NoSuchChoice);
            };
            st.choose(choice, lib).map_err(AppError::Engine)
        })
    }

    /// Reveals the deck's top card; the turn is spent.
    pub fn scout(&mut self) -> Result_ {
        self.act(|lib, st| st.peek(lib).map(|_| ()).map_err(AppError::Engine))
    }

    /// Recycles the discard pile into the deck for one treasury.
    pub fn shuffle(&mut self) -> Result_ {
        self.act(|lib, st| st.shuffle(lib).map_err(AppError::Engine))
    }

    /// Applies `fn` to a clone and, on success, commits it, persists it,
    /// and derives FX effects from the before/after diff. On failure the
    /// state is untouched and the refusal surfaces in the view's log as a
    /// transient newest line, never persisted.
    fn act<F>(&mut self, f: F) -> Result_
    where
        F: FnOnce(&Library, &mut State) -> Result<(), AppError>,
    {
        let Some(st) = self.st.as_ref() else {
            let err = AppError::NoRun;
            return Result_ {
                view: self.projected(None, Some(&err)),
                err: Some(err),
                effects: Vec::new(),
            };
        };
        if st
            .scene_ending(&self.lib)
            .is_some()
        {
            let err = AppError::CampaignOver;
            return Result_ {
                view: self.projected(self.st.as_ref(), Some(&err)),
                err: Some(err),
                effects: Vec::new(),
            };
        }
        let mut next = st.clone();
        if let Err(err) = f(&self.lib, &mut next) {
            return Result_ {
                view: self.projected(self.st.as_ref(), Some(&err)),
                err: Some(err),
                effects: Vec::new(),
            };
        }

        let mut effects = diff_effects(&self.lib, st, &next);
        if let Some(ending) = next.scene_ending(&self.lib) {
            effects.push(Effect::ending(ending));
            self.record_run(&next);
        }
        let view = self.projected(Some(&next), None);
        self.st = Some(next);
        self.persist_state();
        Result_ {
            view,
            err: None,
            effects,
        }
    }

    /// Appends a completed run to the history and persists it.
    fn record_run(&mut self, st: &State) {
        let ending = st
            .scene_ending(&self.lib)
            .unwrap_or_default()
            .to_string();
        self.runs.push(RunSummary {
            ending,
            stats: st.stats,
            turns: st.turns,
            cards_played: st.cards_played,
            date: jiff::Timestamp::now(),
        });
        self.persist_history();
    }

    fn persist_state(&self) {
        if let (Some(dir), Some(st)) = (self.dir.as_deref(), self.st.as_ref())
            && save::save_state(dir, st).is_err()
        {
            // Unwritable save location: keep playing in memory.
        }
    }

    fn persist_history(&self) {
        if let Some(dir) = self.dir.as_deref()
            && save::save_history(dir, &self.runs).is_err()
        {
            // Unwritable save location: keep playing in memory.
        }
    }


    /// The current projection without acting.
    pub fn view(&self) -> View {
        self.projected(self.st.as_ref(), None)
    }

    /// Builds draw data from `st`. A `None` st projects the title-screen
    /// zero view. A non-`None` err is surfaced as the newest log line
    /// only; the underlying state is unchanged.
    fn projected(&self, st: Option<&State>, err: Option<&AppError>) -> View {
        self.project_ext(st, err, &self.runs)
    }

    /// Shared projection used by both the live model and chronicle tests.
    pub(crate) fn project_ext(
        &self,
        st: Option<&State>,
        err: Option<&AppError>,
        runs: &[RunSummary],
    ) -> View {
        let mut v = View {
            campaigns: runs.len() as i32,
            endings: gallery(runs),
            ..View::default()
        };
        let Some(st) = st else {
            return v;
        };
        let scene = self
            .lib
            .scenes
            .get(&st.scene_id)
            .cloned()
            .unwrap_or_default();
        v.scene_id = st.scene_id.clone();
        v.scene = scene.clone();
        v.stats = st.stats;
        v.threat = st.threat;
        v.max_threat = game::THREAT_MAX;
        v.fate = st.rolled;
        v.turns = st.turns;
        v.cards_played = st.cards_played;

        // Deck inspector: every card the run owns, across all piles.
        let mut counts: BTreeMap<String, i32> = BTreeMap::new();
        for pile in [&st.hand, &st.deck, &st.discard] {
            for id in pile {
                *counts.entry(id.clone()).or_insert(0) += 1;
            }
        }
        for (id, n) in &counts {
            v.deck.push(crate::app::view::PileEntry {
                id: id.clone(),
                name: self.card_name(id),
                count: *n,
            });
            v.deck_total += *n;
        }
        v.deck_count = st.deck.len() as i32;
        v.discard_count = st.discard.len() as i32;

        let mut dcounts: BTreeMap<String, i32> = BTreeMap::new();
        for id in &st.discard {
            *dcounts.entry(id.clone()).or_insert(0) += 1;
        }
        for (id, n) in &dcounts {
            v.discard.push(crate::app::view::PileEntry {
                id: id.clone(),
                name: self.card_name(id),
                count: *n,
            });
        }

        let mut log = st.log.clone();
        if let Some(err) = err {
            log.push(format!("Error: {err}"));
        }
        log.reverse();
        v.log = log;

        for (i, c) in scene.choices.iter().enumerate() {
            v.choices.push(crate::app::view::ChoiceView {
                index: i,
                text: c.text.clone(),
                requires: self.requires(c),
                available: st.can_choose(c),
            });
        }
        for id in &st.hand {
            let c = self.lib.cards.get(id).cloned().unwrap_or_default();
            v.hand.push(crate::app::view::CardView {
                id: id.clone(),
                name: c.name.clone(),
                text: c.text.clone(),
                cost: c.cost,
                playable: st.can_play(id, &self.lib.cards),
            });
        }
        v
    }

    fn card_name(&self, id: &str) -> String {
        game::display_name(id, Some(&self.lib.cards))
    }

    /// Formats a choice's unmet-requirement hint (e.g. "Phalanx · Legacy 2").
    fn requires(&self, c: &content::Choice) -> String {
        let mut reqs: Vec<String> = Vec::new();
        if !c.requires_card.is_empty() {
            let mut label = self.card_name(&c.requires_card);
            if c.consumes_card {
                label += " (spent)";
            }
            reqs.push(label);
        }
        if let Some(stats) = &c.requires_stat {
            for (k, n) in stats {
                reqs.push(format!("{} {n}", game::stat_label(k)));
            }
        }
        reqs.join(" · ")
    }

    /// Projects the completed-run history for the chronicle screen.
    pub fn stats_page(&self) -> Chronicle {
        let mut runs: Vec<RunSummary> = self.runs.clone();
        runs.reverse();
        let mut c = Chronicle {
            runs,
            ..Chronicle::default()
        };
        let mut counts: BTreeMap<String, i32> = BTreeMap::new();
        let (mut turns, mut cards) = (0, 0);
        for run in &self.runs {
            *counts.entry(run.ending.clone()).or_insert(0) += 1;
            turns += run.turns;
            cards += run.cards_played;
            if c.best.is_none() || run.stats.legacy > c.best.as_ref().unwrap().stats.legacy {
                c.best = Some(run.clone());
            }
        }
        if !self.runs.is_empty() {
            let n = self.runs.len() as f64;
            c.avg_turns = turns as f64 / n;
            c.avg_cards = cards as f64 / n;
        }
        for class in content::ENDING_ORDER {
            if let Some(count) = counts.get(class) {
                c.endings.push(EndingCount {
                    class: class.to_string(),
                    count: *count,
                });
            }
        }
        c
    }
}


/// Builds the endings tiles over [`content::ENDING_ORDER`].
fn gallery(runs: &[RunSummary]) -> Vec<EndingTile> {
    let found: std::collections::HashSet<&str> =
        runs.iter().map(|r| r.ending.as_str()).collect();
    content::ENDING_ORDER
        .iter()
        .map(|class| EndingTile {
            class: class.to_string(),
            discovered: found.contains(class),
        })
        .collect()
}

/// Compares the pre-action and post-action states and reports everything a
/// client might animate, in deterministic order.
pub fn diff_effects(_lib: &Library, prev: &State, next: &State) -> Vec<Effect> {
    let mut fx = Vec::new();
    for key in [STAT_LEGACY, STAT_ARMY, STAT_TREASURY] {
        let d = next.stat(key) - prev.stat(key);
        if d != 0 {
            fx.push(Effect::stat(key, d));
        }
    }
    let d = next.threat - prev.threat;
    if d != 0 {
        fx.push(Effect::threat(d));
    }
    let prev_hand = counts(&prev.hand);
    let next_hand = counts(&next.hand);
    for id in sorted_keys(&next_hand, &prev_hand) {
        if next_hand.get(&id).copied().unwrap_or(0) < prev_hand.get(&id).copied().unwrap_or(0) {
            fx.push(Effect::card_left(&id));
        }
    }
    let mut prev_pool = counts(&prev.hand);
    let p = counts(&prev.deck);
    for (k, v) in p {
        *prev_pool.entry(k).or_insert(0) += v;
    }
    let p = counts(&prev.discard);
    for (k, v) in p {
        *prev_pool.entry(k).or_insert(0) += v;
    }
    let mut next_pool = counts(&next.hand);
    let p = counts(&next.deck);
    for (k, v) in p {
        *next_pool.entry(k).or_insert(0) += v;
    }
    let p = counts(&next.discard);
    for (k, v) in p {
        *next_pool.entry(k).or_insert(0) += v;
    }
    for id in sorted_keys(&next_pool, &prev_pool) {
        let a = next_pool.get(&id).copied().unwrap_or(0);
        let b = prev_pool.get(&id).copied().unwrap_or(0);
        if a > b {
            fx.push(Effect::card_gained(&id));
        } else if a < b {
            fx.push(Effect::card_gone(&id));
        }
    }
    if next.scene_id != prev.scene_id {
        fx.push(Effect::scene_changed(&prev.scene_id, &next.scene_id));
    }
    if next.rolled {
        fx.push(Effect::fate());
    }
    fx
}

/// Tallies multiset membership.
fn counts(ids: &[String]) -> BTreeMap<String, i32> {
    let mut c = BTreeMap::new();
    for id in ids {
        *c.entry(id.clone()).or_insert(0) += 1;
    }
    c
}

/// The union of both maps' keys in stable order.
fn sorted_keys(a: &BTreeMap<String, i32>, b: &BTreeMap<String, i32>) -> Vec<String> {
    a.keys().chain(b.keys()).cloned().collect::<std::collections::BTreeSet<_>>().into_iter().collect()
}

