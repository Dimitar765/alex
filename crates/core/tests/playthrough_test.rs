//! Ports internal/app/playthrough_test.go: scripted full campaigns over
//! the shipped content.

use alexander_core::app::Model;
use alexander_core::content::{self, BATTLE_SCENE};
use alexander_core::game;

fn shipped_lib() -> content::Library {
    content::load_embedded()
}

/// Marches straight through the shipped campaign taking the first
/// available choice every turn. Threat then advances strictly (no card
/// reductions), so the raid, ambush, and forced battle are all guaranteed
/// to fire before an ending arrives.
#[test]
fn shipped_campaign_choices_only_playthrough() {
    let mut m = Model::new(shipped_lib(), None);
    m.new_game();

    let (mut battle_seen, mut raid_seen, mut ambush_seen) = (false, false, false);
    let mut max_threat = 0;
    let mut ending = String::new();
    for turn in 0..100 {
        let v = m.view();
        if !v.scene.ending.is_empty() {
            ending = v.scene.ending.clone();
            break;
        }
        if v.scene_id == BATTLE_SCENE {
            battle_seen = true;
        }
        max_threat = max_threat.max(v.threat);
        if v.threat >= game::THREAT_RAID {
            raid_seen = true;
        }
        if v.threat >= game::THREAT_AMBUSH {
            ambush_seen = true;
        }
        let mut acted = false;
        for ch in &v.choices {
            if ch.available {
                let r = m.choose(ch.index);
                assert!(r.err.is_none(), "turn {turn}: available choice refused: {:?}", r.err);
                acted = true;
                break;
            }
        }
        assert!(acted, "turn {turn}: no available choice at scene {:?}", v.scene_id);
    }
    assert!(!ending.is_empty(), "campaign did not reach an ending within 100 turns");
    assert!(battle_seen, "forced battle never reached");
    assert!(
        raid_seen && ambush_seen,
        "thresholds never crossed: raid={raid_seen} ambush={ambush_seen} max={max_threat}"
    );
    assert_eq!(max_threat, game::THREAT_MAX);
    assert_eq!(m.history().len(), 1);
}

/// Plays cards whenever they are affordable and otherwise takes choices —
/// closer to a human rhythm. It must always reach an ending without
/// engine errors.
#[test]
fn shipped_campaign_mixed_playthrough() {
    let mut m = Model::new(shipped_lib(), None);
    m.new_game();
    let mut ending = String::new();
    for turn in 0..120 {
        let v = m.view();
        if !v.scene.ending.is_empty() {
            ending = v.scene.ending.clone();
            break;
        }
        // One action per turn: prefer playing an affordable card every
        // other turn, otherwise take the first available choice.
        let mut acted = false;
        if turn % 2 == 0 {
            for c in &v.hand {
                if c.playable {
                    let r = m.play_card(&c.id);
                    if r.err.is_none() {
                        acted = true;
                    }
                    break;
                }
            }
        }
        if !acted {
            for ch in &v.choices {
                if ch.available {
                    let r = m.choose(ch.index);
                    assert!(r.err.is_none(), "turn {turn}: {:?}", r.err);
                    acted = true;
                    break;
                }
            }
        }
        let _ = acted;
    }
    assert!(!ending.is_empty(), "mixed playthrough never ended");
}
