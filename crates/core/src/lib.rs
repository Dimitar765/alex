//! Alexander core: content schema (`content`), rules engine (`game`),
//! UI-agnostic view-model (`app`), and the balance simulator (`sim`).

pub mod app;
pub mod content;
pub mod game;
pub mod sim;

/// The shipped card content, embedded at compile time.
pub const CARDS_JSON: &str = include_str!("../../../content/cards.json");
/// The shipped scene content, embedded at compile time.
pub const SCENES_JSON: &str = include_str!("../../../content/scenes.json");
