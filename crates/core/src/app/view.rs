//! Pure projection of the model: everything needed to draw one frame,
//! safe to hold while animations play.

use crate::app::save::RunSummary;
use crate::content::Scene;
use crate::game::Stats;

/// One slot in the endings gallery.
#[derive(Debug, Clone, PartialEq)]
pub struct EndingTile {
    pub class: String,
    pub discovered: bool,
}

/// One aggregated deck/discard line.
#[derive(Debug, Clone, PartialEq)]
pub struct PileEntry {
    pub id: String,
    pub name: String,
    pub count: i32,
}

/// One hand card as the UI draws it.
#[derive(Debug, Clone, PartialEq)]
pub struct CardView {
    pub id: String,
    pub name: String,
    pub text: String,
    pub cost: i32,
    pub playable: bool,
}

/// One scene option as the UI draws it.
#[derive(Debug, Clone, PartialEq)]
pub struct ChoiceView {
    pub index: usize,
    pub text: String,
    pub requires: String,
    pub available: bool,
}

/// Everything needed to draw one frame: a pure projection of the model.
#[derive(Debug, Clone, Default)]
pub struct View {
    pub scene_id: String,
    pub scene: Scene,
    pub stats: Stats,
    pub threat: i32,
    pub max_threat: i32,
    /// The last action resolved a random effect.
    pub fate: bool,

    pub choices: Vec<ChoiceView>,
    pub hand: Vec<CardView>,

    /// Every owned card across piles, sorted by ID.
    pub deck: Vec<PileEntry>,
    pub deck_total: i32,
    /// Draw pile size.
    pub deck_count: i32,
    pub discard: Vec<PileEntry>,
    pub discard_count: i32,

    /// Newest first.
    pub log: Vec<String>,
    pub turns: i32,
    pub cards_played: i32,

    pub campaigns: i32,
    pub endings: Vec<EndingTile>,
}

/// One ending class and how many runs ended with it.
#[derive(Debug, Clone, PartialEq)]
pub struct EndingCount {
    pub class: String,
    pub count: i32,
}

/// Summarizes the completed runs for the stats screen.
#[derive(Debug, Clone, Default)]
pub struct Chronicle {
    /// Newest first.
    pub runs: Vec<RunSummary>,
    pub endings: Vec<EndingCount>,
    pub avg_turns: f64,
    pub avg_cards: f64,
    pub best: Option<RunSummary>,
}
