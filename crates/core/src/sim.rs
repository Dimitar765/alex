//! Automated campaigns against the rules engine to prove content health:
//! no soft-locked runs, every ending reachable, and a measured ending
//! distribution for balance work.

use std::fmt;

use rand::rngs::StdRng;
use rand::{Rng, SeedableRng};

use crate::content::{self, Library};
use crate::game::State;

/// Bounds one simulated run; hitting it counts as stuck.
pub const MAX_TURNS: i32 = 150;

/// The chance per turn that the policy develops resources by playing a
/// card instead of taking a choice, as `int_n(5) < 2`.
const CARD_PLAY_ODDS: usize = 2;

/// One simulated campaign.
#[derive(Debug, Clone, PartialEq)]
pub struct Result {
    /// Ending classification, or "stuck".
    pub ending: String,
    /// Final scene.
    pub scene_id: String,
    pub turns: i32,
    /// The forced battle scene was entered.
    pub battle_arrived: bool,
}

/// Aggregated simulation results.
#[derive(Debug, Clone, Default)]
pub struct Report {
    pub runs: i32,
    pub endings: std::collections::BTreeMap<String, i32>,
    pub turns_avg: f64,
    pub turns_max: i32,
    pub stuck: i32,
    /// Scene -> times stuck there.
    pub stuck_here: std::collections::BTreeMap<String, i32>,
    /// Runs that faced the forced battle.
    pub battle_arrived: i32,
}

/// Plays `n` campaigns with the random-greedy policy, deterministically
/// for the given seed, and returns the per-run results.
pub fn simulate(lib: &Library, n: usize, seed: u64) -> Vec<Result> {
    (0..n)
        .map(|i| {
            let rng =
                StdRng::seed_from_u64(seed ^ 0x9E3779B97F4A7C15u64.wrapping_mul(i as u64 + 1));
            let st = &mut new_run(lib, rng.clone());
            play(st, lib, rng)
        })
        .collect()
}

/// Starts a fresh state with an explicit random source.
pub fn new_run(lib: &Library, rng: StdRng) -> State {
    State::new(lib, "title", rng)
}

/// Drives one run to an ending (or the turn cap) and returns its result.
fn play(st: &mut State, lib: &Library, mut rng: StdRng) -> Result {
    let mut battle = false;
    while st.turns < MAX_TURNS {
        if st.scene_id == content::BATTLE_SCENE {
            battle = true;
        }
        let scene = lib.scenes.get(&st.scene_id).cloned().unwrap_or_default();
        if !scene.ending.is_empty() {
            return Result {
                ending: scene.ending,
                scene_id: scene.id,
                turns: st.turns,
                battle_arrived: battle,
            };
        }
        if act(st, lib, &mut rng) {
            continue;
        }
        return Result {
            ending: "stuck".into(),
            scene_id: st.scene_id.clone(),
            turns: st.turns,
            battle_arrived: battle,
        };
    }
    Result {
        ending: "stuck".into(),
        scene_id: st.scene_id.clone(),
        turns: st.turns,
        battle_arrived: battle,
    }
}

/// Takes one turn: with a fixed chance it plays a random playable card to
/// develop resources, otherwise it takes a random available choice. It
/// reports `false` only when neither is possible (a true deadlock).
fn act(st: &mut State, lib: &Library, rng: &mut StdRng) -> bool {
    let playable: Vec<&String> = st
        .hand
        .iter()
        .filter(|id| st.can_play(id, &lib.cards))
        .collect();
    if !playable.is_empty() && rng.random_range(0..5usize) < CARD_PLAY_ODDS {
        let id = playable[rng.random_range(0..playable.len())].clone();
        let _ = st.play_card(&id, lib);
        return true;
    }
    let scene = lib.scenes.get(&st.scene_id).cloned().unwrap_or_default();
    let available: Vec<usize> = scene
        .choices
        .iter()
        .enumerate()
        .filter(|(_, ch)| st.can_choose(ch))
        .map(|(i, _)| i)
        .collect();
    if available.is_empty() {
        if playable.is_empty() {
            return false;
        }
        let id = playable[rng.random_range(0..playable.len())].clone();
        let _ = st.play_card(&id, lib);
        return true;
    }
    let i = available[rng.random_range(0..available.len())];
    let _ = st.choose(&scene.choices[i], lib);
    true
}

/// Summarizes simulation results.
pub fn aggregate(results: &[Result]) -> Report {
    let mut r = Report {
        runs: results.len() as i32,
        ..Report::default()
    };
    let mut total = 0;
    for res in results {
        *r.endings.entry(res.ending.clone()).or_insert(0) += 1;
        if res.battle_arrived {
            r.battle_arrived += 1;
        }
        if res.ending == "stuck" {
            r.stuck += 1;
            *r.stuck_here.entry(res.scene_id.clone()).or_insert(0) += 1;
        }
        total += res.turns;
        if res.turns > r.turns_max {
            r.turns_max = res.turns;
        }
    }
    if r.runs > 0 {
        r.turns_avg = total as f64 / r.runs as f64;
    }
    r
}

impl fmt::Display for Report {
    /// Renders the report as an aligned plain-text block.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        writeln!(f, "runs        {}", self.runs)?;
        writeln!(f, "battle      {}", self.battle_arrived)?;
        let mut classes: Vec<&String> = self.endings.keys().collect();
        classes.sort_by(|a, b| {
            if a.as_str() == "stuck" {
                std::cmp::Ordering::Greater // stuck always last
            } else if b.as_str() == "stuck" {
                std::cmp::Ordering::Less
            } else {
                self.endings[b.as_str()].cmp(&self.endings[a.as_str()])
            }
        });
        for c in classes {
            let pct = 100.0 * self.endings[c] as f64 / self.runs as f64;
            writeln!(f, "{c:<11} {:>4} ({pct:>4.1}%)", self.endings[c])?;
        }
        writeln!(f, "turns avg   {:.1}, max {}", self.turns_avg, self.turns_max)?;
        if self.stuck > 0 {
            let scenes: Vec<String> = self
                .stuck_here
                .iter()
                .map(|(k, v)| format!("{k}: {v}"))
                .collect();
            writeln!(f, "STUCK RUNS at: map{{{}}}", scenes.join(" "))?;
        }
        Ok(())
    }
}


#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn report_display_matches_go_layout() {
        let mut r = Report {
            runs: 2,
            ..Report::default()
        };
        r.endings.insert("triumph".into(), 1);
        r.endings.insert("defeat".into(), 1);
        r.turns_avg = 12.0;
        r.turns_max = 20;
        let text = r.to_string();
        assert!(text.starts_with("runs        2\n"));
        assert!(text.contains("turns avg   12.0, max 20"));
    }
}
