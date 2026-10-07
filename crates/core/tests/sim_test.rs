//! Ports internal/sim/sim_test.go, re-pinned to Rust RNG seeds.

use std::collections::HashSet;

use alexander_core::content;
use alexander_core::sim::{self, Result, MAX_TURNS};

fn shipped_lib() -> content::Library {
    content::load_embedded()
}

/// The safety proof: random-greedy play must never soft-lock on the
/// shipped campaign — every run reaches a declared ending within the cap.
#[test]
fn no_stuck_runs_on_shipped_content() {
    let lib = shipped_lib();
    let report = sim::aggregate(&sim::simulate(&lib, 500, 1));
    assert_eq!(report.stuck, 0, "soft-locked runs at {:?}", report.stuck_here);
    assert_eq!(report.runs, 500);
    assert!(!report.endings.is_empty());
    assert!(report.turns_avg > 0.0);
    assert!(report.turns_max < MAX_TURNS, "max {} vs cap {}", report.turns_max, MAX_TURNS);
}

/// Every ending class that exists in the shipped content must be reachable
/// by undirected play.
#[test]
fn all_shipped_endings_reachable() {
    let lib = shipped_lib();
    let report = sim::aggregate(&sim::simulate(&lib, 500, 7));
    let want: HashSet<&str> = lib
        .scenes
        .values()
        .filter(|sc| !sc.ending.is_empty())
        .map(|sc| sc.ending.as_str())
        .collect();
    for class in want {
        assert!(
            report.endings.get(class).copied().unwrap_or(0) > 0,
            "ending {class:?} never observed in 500 runs;\n{report}"
        );
    }
}

/// Same seed must reproduce the same campaign outcomes exactly.
#[test]
fn simulate_deterministic() {
    let lib = shipped_lib();
    let a = sim::simulate(&lib, 25, 42);
    let b = sim::simulate(&lib, 25, 42);
    assert_eq!(a, b);
}

#[test]
fn aggregate_counts() {
    let r = sim::aggregate(&[
        Result {
            ending: "triumph".into(),
            scene_id: String::new(),
            turns: 10,
            battle_arrived: false,
        },
        Result {
            ending: "triumph".into(),
            scene_id: String::new(),
            turns: 20,
            battle_arrived: false,
        },
        Result {
            ending: "stuck".into(),
            scene_id: "vault".into(),
            turns: MAX_TURNS,
            battle_arrived: false,
        },
    ]);
    assert_eq!(r.runs, 3);
    assert_eq!(r.endings["triumph"], 2);
    assert_eq!(r.stuck, 1);
    assert_eq!(r.stuck_here["vault"], 1);
    assert_eq!(r.turns_avg, (10 + 20 + MAX_TURNS) as f64 / 3.0);
    assert_eq!(r.turns_max, MAX_TURNS);
}
