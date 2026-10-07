//! View-model layer: model with clone-commit atomicity, FX diff, view
//! projection, saves, and run history.

mod events;
mod model;
mod save;
mod view;

pub use events::{Effect, EffectKind};
pub use model::{diff_effects, AppError, Model, Result_, CHRONICLE_DATE_FMT, START_SCENE};
pub use save::{
    load_history, load_state, save_history, save_state, user_saves_dir, RunSummary,
    HISTORY_FILE, SAVE_FILE,
};
pub use view::{CardView, ChoiceView, Chronicle, EndingCount, EndingTile, PileEntry, View};
