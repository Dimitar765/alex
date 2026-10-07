//! Persistence: state and history JSON in the user's save directory,
//! written atomically. Corrupt or missing files read as empty so the
//! client shows the title screen rather than failing.

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use crate::game::{State, Stats};

/// Save file names inside the save directory.
pub const SAVE_FILE: &str = "save.json";
pub const HISTORY_FILE: &str = "history.json";

/// Records one completed campaign for the gallery and chronicle.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RunSummary {
    pub ending: String,
    #[serde(default)]
    pub stats: Stats,
    #[serde(default)]
    pub turns: i32,
    #[serde(rename = "cardsPlayed", default)]
    pub cards_played: i32,
    #[serde(default)]
    pub date: jiff::Timestamp,
}

/// The per-user saves directory:
///
/// - Windows: `%APPDATA%\Alexander\saves`
/// - macOS:   `~/Library/Application Support/Alexander/saves`
/// - Linux:   `$XDG_CONFIG_HOME/Alexander/saves` (or `~/.config`)
///
/// Returns `None` when no config root exists; the caller then keeps
/// everything in memory.
pub fn user_saves_dir() -> Option<PathBuf> {
    dirs::config_dir().map(|base| base.join("Alexander").join("saves"))
}

/// Reads a persisted campaign, returning `None` when absent or corrupt —
/// the client then shows the title screen rather than failing.
pub fn load_state(dir: &Path) -> Option<State> {
    let b = fs::read(dir.join(SAVE_FILE)).ok()?;
    serde_json::from_slice(&b).ok()
}

/// Reads the completed-run history; corrupt reads as empty.
pub fn load_history(dir: &Path) -> Vec<RunSummary> {
    let Ok(b) = fs::read(dir.join(HISTORY_FILE)) else {
        return Vec::new();
    };
    serde_json::from_slice(&b).unwrap_or_default()
}

/// Persists the live state atomically.
pub fn save_state(dir: &Path, st: &State) -> io::Result<()> {
    let b = serde_json::to_vec_pretty(st).map_err(io::Error::other)?;
    write_file_atomic(&dir.join(SAVE_FILE), &b)
}

/// Persists the completed-run history atomically.
pub fn save_history(dir: &Path, runs: &[RunSummary]) -> io::Result<()> {
    let b = serde_json::to_vec_pretty(runs).map_err(io::Error::other)?;
    write_file_atomic(&dir.join(HISTORY_FILE), &b)
}

/// Writes `b` to `path` via a temp file in the same directory followed by
/// rename, so readers never observe a torn file.
fn write_file_atomic(path: &Path, b: &[u8]) -> io::Result<()> {
    let dir = path.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(dir)?;
    let tmp = dir.join(format!(
        "{}.{}.tmp",
        path.file_name().unwrap_or_default().to_string_lossy(),
        std::process::id()
    ));
    fs::write(&tmp, b)?;
    let result = fs::rename(&tmp, path);
    if result.is_err() {
        let _ = fs::remove_file(&tmp);
    }
    result
}
