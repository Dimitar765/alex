//! Ports internal/game/state_test.go.

use std::collections::HashMap;

use alexander_core::content::{Card, Choice, Effect, Library, Outcome, Scene};
use alexander_core::game::*;

fn test_lib() -> Library {
    let mut lib = Library::default();
    for (id, name) in [
        ("a", "Alpha"),
        ("b", "Bravo"),
        ("c", "Charlie"),
        ("d", "Delta"),
        ("e", "Echo"),
    ] {
        lib.cards.insert(
            id.to_string(),
            Card {
                id: id.to_string(),
                name: name.to_string(),
                text: format!("flavor {id}"),
                start: true,
                ..Card::default()
            },
        );
    }
    lib
}

fn rng() -> rand::rngs::StdRng {
    rand::SeedableRng::seed_from_u64(7)
}

#[test]
fn new_state_deals_opening_hand() {
    let lib = test_lib();
    let s = State::new(&lib, "title", rng());
    assert_eq!(s.scene_id, "title");
    assert_eq!(s.hand.len(), HAND_SIZE);
    let mut all = s.hand.clone();
    all.extend(s.deck.iter().cloned());
    all.sort();
    assert_eq!(all, vec!["a", "b", "c", "d", "e"]);
}

#[test]
fn peek_reveals_top_without_touching_piles() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into(), "c".into()],
        ..State::default()
    };
    let top = s.peek(&lib).expect("peek");
    assert_eq!(top, "c");
    assert_eq!(s.turns, 1, "peek must spend the turn");
    assert_eq!(s.deck.len(), 2);
    assert_eq!(s.deck[1], "c");
    assert_eq!(s.hand.len(), 1);
    assert!(
        s.log.iter().any(|l| l.contains("Next card is Charlie")),
        "log must name the scouted card: {:?}",
        s.log
    );
}

#[test]
fn peek_empty_deck_refused() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        ..State::default()
    };
    let err = s.peek(&lib).unwrap_err();
    assert!(err.to_string().contains("nothing to scout"), "{err}");
    assert_eq!(s.turns, 0, "refused peek must not spend the turn");
}

#[test]
fn rolled_tracks_random_resolution() {
    let lib = test_lib();
    let risky = Choice {
        text: "Fate".into(),
        effects: vec![Effect {
            kind: "random".into(),
            outcomes: vec![Outcome {
                weight: 1,
                effects: vec![Effect {
                    kind: "stat".into(),
                    delta: Some([("army".to_string(), 1)].into_iter().collect()),
                    ..Effect::default()
                }],
            }],
            ..Effect::default()
        }],
        ..Choice::default()
    };
    let plain = Choice {
        text: "March".into(),
        ..Choice::default()
    };
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        ..State::default()
    };
    s.choose(&risky, &lib).expect("risky choose");
    assert!(s.rolled, "rolled must be set after a random resolution");
    s.choose(&plain, &lib).expect("plain choose");
    assert!(!s.rolled, "rolled must reset on the next action");
}

#[test]
fn shuffle_recycles_discard_and_spends_turn() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        discard: vec!["c".into(), "d".into()],
        stats: Stats {
            treasury: 2,
            ..Stats::default()
        },
        ..State::default()
    };
    s.shuffle(&lib).expect("shuffle");
    assert_eq!(s.turns, 1);
    assert_eq!(s.cards_played, 0, "shuffle is not a card play");
    assert_eq!(s.stats.treasury, 1, "shuffle must pay its treasury cost");
    assert!(s.discard.is_empty());
    assert_eq!(s.deck.len(), 3, "b + recycled c, d");
    let mut all = s.deck.clone();
    all.extend(s.hand.iter().cloned());
    all.sort();
    assert_eq!(all, vec!["a", "b", "c", "d"], "cards must be conserved");
    assert!(
        s.log
            .iter()
            .any(|l| l.contains("Shuffled the discard pile into the deck. Treasury -1")),
        "log must record the shuffle and cost: {:?}",
        s.log
    );
}

#[test]
fn shuffle_broke_refused() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        discard: vec!["c".into()],
        ..State::default()
    };
    let err = s.shuffle(&lib).unwrap_err();
    assert!(
        err.to_string()
            .contains("cannot afford to shuffle (costs 1 treasury)"),
        "{err}"
    );
    assert_eq!(s.turns, 0);
    assert_eq!(s.discard.len(), 1);
    assert_eq!(s.stats.treasury, 0, "refused shuffle must not mutate state");
}

#[test]
fn shuffle_empty_discard_refused() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        ..State::default()
    };
    let err = s.shuffle(&lib).unwrap_err();
    assert!(err.to_string().contains("nothing to shuffle"), "{err}");
    assert_eq!(s.turns, 0);
    assert_eq!(s.deck.len(), 1, "refused shuffle must not mutate state");
}

#[test]
fn action_counters() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into(), "c".into(), "d".into(), "e".into()],
        ..State::default()
    };
    let choice = Choice {
        text: "March".into(),
        ..Choice::default()
    };
    s.choose(&choice, &lib).expect("choose");
    s.play_card("a", &lib).expect("play");
    assert!(s.play_card("ghost", &lib).is_err(), "refused play must not count");
    assert_eq!(s.turns, 2);
    assert_eq!(s.cards_played, 1, "refusals excluded");
}

#[test]
fn choose_requires_stat() {
    let lib = test_lib();
    let choice = Choice {
        text: "Break the center".into(),
        requires_stat: Some(
            [
                ("army".to_string(), 3),
                ("legacy".to_string(), 1),
            ]
            .into_iter()
            .collect(),
        ),
        ..Choice::default()
    };
    let mut s = State {
        scene_id: "issus".into(),
        stats: Stats {
            army: 2,
            legacy: 5,
            ..Stats::default()
        },
        hand: vec!["a".into()],
        ..State::default()
    };
    let err = s.choose(&choice, &lib).unwrap_err();
    assert!(
        err.to_string().contains("requires Army 3 (you have 2)"),
        "want Army shortfall named: {err}"
    );
    assert_eq!(s.scene_id, "issus", "rejected choice must not move the scene");
    s.stats.army = 3;
    s.choose(&choice, &lib).expect("met requirements");
}

#[test]
fn can_choose_reports_availability() {
    let choice = Choice {
        requires_card: "e".into(),
        requires_stat: Some([("army".to_string(), 1)].into_iter().collect()),
        ..Choice::default()
    };
    let mut s = State {
        hand: vec!["a".into()],
        ..State::default()
    };
    assert!(!s.can_choose(&choice), "needs card and stat");
    s.stats.army = 1;
    assert!(!s.can_choose(&choice), "needs the card");
    s.hand = vec!["e".into(), "a".into()];
    assert!(s.can_choose(&choice), "has card and stat");
}

#[test]
fn lose_card_moves_to_discard() {
    let mut lib = test_lib();
    // A stocked deck keeps drawUp from reshuffling the discard, so the
    // lost cards provably stay in the discard pile for now.
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into()],
        deck: vec![
            "c".into(),
            "e".into(),
            "e".into(),
            "d".into(),
            "d".into(),
            "e".into(),
        ],
        ..State::default()
    };
    lib.cards.insert(
        "a".into(),
        Card {
            id: "a".into(),
            name: "Alpha".into(),
            effects: vec![
                Effect {
                    kind: "lose_card".into(),
                    card_id: "c".into(),
                    ..Effect::default()
                },
                Effect {
                    kind: "lose_card".into(),
                    card_id: "b".into(),
                    ..Effect::default()
                },
            ],
            ..Card::default()
        },
    );
    s.play_card("a", &lib).expect("play");
    assert!(!s.hand.contains(&"b".to_string()) && !s.deck.contains(&"c".to_string()));
    for id in ["a", "b", "c"] {
        assert!(
            s.discard.contains(&id.to_string()),
            "discard must hold {id}: {:?}",
            s.discard
        );
    }
    assert!(
        s.log.iter().any(|l| l.contains("lost Charlie")),
        "log must name the lost card: {:?}",
        s.log
    );
}

#[test]
fn remove_card_exiles_from_run() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into()],
        deck: vec!["c".into()],
        discard: vec!["d".into()],
        ..State::default()
    };
    let choice = Choice {
        text: "The horse dies".into(),
        effects: vec![
            Effect {
                kind: "remove_card".into(),
                card_id: "a".into(),
                ..Effect::default()
            },
            Effect {
                kind: "remove_card".into(),
                card_id: "c".into(),
                ..Effect::default()
            },
            Effect {
                kind: "remove_card".into(),
                card_id: "d".into(),
                ..Effect::default()
            },
        ],
        ..Choice::default()
    };
    s.choose(&choice, &lib).expect("choose");
    let mut piles = s.hand.clone();
    piles.extend(s.deck.iter().cloned());
    piles.extend(s.discard.iter().cloned());
    for id in ["a", "c", "d"] {
        assert!(
            !piles.contains(&id.to_string()),
            "removed card {id} still present: {piles:?}"
        );
    }
    assert!(
        s.log.iter().any(|l| l.contains("Alpha is gone for good")),
        "log must name the exile: {:?}",
        s.log
    );
}

#[test]
fn random_single_outcome_is_deterministic() {
    let mut lib = test_lib();
    let mut s = State {
        scene_id: "start".into(),
        hand: vec!["a".into()],
        ..State::default()
    };
    lib.scenes.insert(
        "field".into(),
        Scene {
            id: "field".into(),
            ..Scene::default()
        },
    );
    let choice = Choice {
        text: "Fate is kind".into(),
        effects: vec![Effect {
            kind: "random".into(),
            outcomes: vec![Outcome {
                weight: 1,
                effects: vec![Effect {
                    kind: "goto".into(),
                    next: "field".into(),
                    ..Effect::default()
                }],
            }],
            ..Effect::default()
        }],
        ..Choice::default()
    };
    s.choose(&choice, &lib).expect("choose");
    assert_eq!(s.scene_id, "field");
}

#[test]
fn random_respects_weights() {
    let lib = test_lib();
    let mut rare = 0;
    let trials = 3000;
    for _ in 0..trials {
        let mut s = State {
            scene_id: "start".into(),
            hand: vec!["a".into()],
            ..State::default()
        };
        let choice = Choice {
            text: "Roll".into(),
            effects: vec![Effect {
                kind: "random".into(),
                outcomes: vec![
                    Outcome {
                        weight: 3,
                        effects: vec![Effect {
                            kind: "stat".into(),
                            delta: Some([("army".to_string(), 1)].into_iter().collect()),
                            ..Effect::default()
                        }],
                    },
                    Outcome {
                        weight: 1,
                        effects: vec![Effect {
                            kind: "stat".into(),
                            delta: Some([("legacy".to_string(), 1)].into_iter().collect()),
                            ..Effect::default()
                        }],
                    },
                ],
                ..Effect::default()
            }],
            ..Choice::default()
        };
        s.choose(&choice, &lib).expect("trial");
        if s.stats.legacy == 1 {
            rare += 1;
        }
    }
    // Expected 25% of trials; generous bounds keep the test stable.
    let (lo, hi) = (trials * 15 / 100, trials * 35 / 100);
    assert!(
        (lo..=hi).contains(&rare),
        "rare outcome hit {rare}/{trials} times, want {lo}..{hi}"
    );
}

#[test]
fn play_card_costs_treasury() {
    let mut lib = test_lib();
    lib.cards.insert(
        "a".into(),
        Card {
            id: "a".into(),
            name: "Alpha".into(),
            cost: 2,
            effects: vec![Effect {
                kind: "stat".into(),
                delta: Some([("army".to_string(), 2)].into_iter().collect()),
                ..Effect::default()
            }],
            ..Card::default()
        },
    );
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into()],
        stats: Stats {
            treasury: 3,
            ..Stats::default()
        },
        ..State::default()
    };
    s.play_card("a", &lib).expect("play");
    assert_eq!(s.stats.treasury, 1);
    assert_eq!(s.stats.army, 2, "cost not paid correctly");
    assert!(
        s.log.iter().any(|l| l.contains("Treasury -2")),
        "log must name the cost: {:?}",
        s.log
    );
}

#[test]
fn play_card_unaffordable() {
    let mut lib = test_lib();
    lib.cards.insert(
        "a".into(),
        Card {
            id: "a".into(),
            name: "Alpha".into(),
            cost: 2,
            ..Card::default()
        },
    );
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into()],
        stats: Stats {
            treasury: 1,
            ..Stats::default()
        },
        ..State::default()
    };
    let err = s.play_card("a", &lib).unwrap_err();
    assert!(
        err.to_string().contains("cannot afford Alpha (costs 2 treasury)"),
        "{err}"
    );
    assert_eq!(s.hand.len(), 2, "refused play must not mutate state");
    assert_eq!(s.stats.treasury, 1);
    assert!(!s.can_play("a", &lib.cards), "CanPlay must report unaffordable");
    s.stats.treasury = 2;
    assert!(s.can_play("a", &lib.cards), "CanPlay must report affordable");
}

#[test]
fn choose_consumes_required_card() {
    let lib = test_lib();
    let choice = Choice {
        text: "Ride him into the river".into(),
        requires_card: "e".into(),
        consumes_card: true,
        effects: vec![Effect {
            kind: "stat".into(),
            delta: Some([("legacy".to_string(), 2)].into_iter().collect()),
            ..Effect::default()
        }],
        ..Choice::default()
    };
    // A stocked deck keeps drawUp from recycling the consumed card.
    let mut s = State {
        scene_id: "granicus".into(),
        hand: vec!["e".into(), "a".into()],
        deck: vec!["b".into(), "c".into(), "d".into()],
        ..State::default()
    };
    s.choose(&choice, &lib).expect("choose");
    assert!(
        !s.hand.contains(&"e".to_string()) && s.discard.contains(&"e".to_string()),
        "consumed card must move hand->discard: hand={:?} discard={:?}",
        s.hand,
        s.discard
    );
    assert!(
        s.log.iter().any(|l| l.contains("spent Echo")),
        "log must name the spent card: {:?}",
        s.log
    );
}

#[test]
fn new_state_deals_starting_cards_only() {
    let mut lib = test_lib();
    let mut regional = lib.cards["e"].clone();
    regional.start = false; // Echo is regional now
    lib.cards.insert("e".into(), regional);
    let s = State::new(&lib, "title", rng());
    let mut all = s.hand.clone();
    all.extend(s.deck.iter().cloned());
    assert!(!all.contains(&"e".to_string()), "non-start card dealt: {all:?}");
    assert_eq!(all.len(), 4, "want the 4 start cards");
}

#[test]
fn arrival_grants_scene_pool_once() {
    let mut lib = test_lib();
    lib.scenes.insert(
        "field".into(),
        Scene {
            id: "field".into(),
            cards: vec!["e".into()],
            ..Scene::default()
        },
    );
    lib.scenes.insert(
        "start".into(),
        Scene {
            id: "start".into(),
            ..Scene::default()
        },
    );
    let mut s = State {
        scene_id: "start".into(),
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        ..State::default()
    };
    let enter = Choice {
        text: "March".into(),
        effects: vec![Effect {
            kind: "goto".into(),
            next: "field".into(),
            ..Effect::default()
        }],
        ..Choice::default()
    };
    s.choose(&enter, &lib).expect("first entry");
    assert!(s.granted.contains(&"field".to_string()), "field must be marked granted");
    // The granted card sits on the deck top, so the refill drew it.
    assert!(
        s.hand.contains(&"e".to_string()),
        "granted card must be drawn first: hand={:?} deck={:?}",
        s.hand,
        s.deck
    );
    assert!(
        s.log.iter().any(|l| l.contains("gained Echo")),
        "log must name the grant: {:?}",
        s.log
    );

    // Leave and re-enter: the pool never grants twice.
    let exit = Choice {
        text: "Back".into(),
        effects: vec![Effect {
            kind: "goto".into(),
            next: "start".into(),
            ..Effect::default()
        }],
        ..Choice::default()
    };
    let enter2 = Choice {
        text: "March again".into(),
        effects: vec![Effect {
            kind: "goto".into(),
            next: "field".into(),
            ..Effect::default()
        }],
        ..Choice::default()
    };
    let before = s.hand.len() + s.deck.len() + s.discard.len();
    s.choose(&exit, &lib).expect("exit");
    s.choose(&enter2, &lib).expect("re-entry");
    let after = s.hand.len() + s.deck.len() + s.discard.len();
    assert_eq!(after, before, "re-entry must not duplicate grants");
    assert_eq!(
        s.log.iter().filter(|l| l.contains("gained Echo")).count(),
        1,
        "grant must be logged exactly once: {:?}",
        s.log
    );
}

#[test]
fn choose_requires_card() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "river".into(),
        hand: vec!["a".into(), "b".into()],
        ..State::default()
    };
    let choice = Choice {
        text: "Charge".into(),
        requires_card: "e".into(),
        ..Choice::default()
    };
    let err = s.choose(&choice, &lib).unwrap_err();
    assert!(err.to_string().contains("Echo"), "want error naming Echo: {err}");
    assert_eq!(s.scene_id, "river");
    assert!(s.log.is_empty(), "rejected choice must not mutate state");
}

#[test]
fn choose_applies_effects_and_refills() {
    let mut lib = test_lib();
    let mut s = State {
        scene_id: "river".into(),
        hand: vec!["a".into(), "b".into()],
        deck: vec!["c".into(), "d".into()],
        ..State::default()
    };
    let choice = Choice {
        text: "March".into(),
        effects: vec![
            Effect {
                kind: "stat".into(),
                delta: Some(
                    [
                        ("legacy".to_string(), 2),
                        ("army".to_string(), -1),
                    ]
                    .into_iter()
                    .collect(),
                ),
                ..Effect::default()
            },
            Effect {
                kind: "goto".into(),
                next: "field".into(),
                ..Effect::default()
            },
        ],
        ..Choice::default()
    };
    lib.scenes.insert(
        "field".into(),
        Scene {
            id: "field".into(),
            ..Scene::default()
        },
    );
    s.choose(&choice, &lib).expect("choose");
    assert_eq!(
        s.stats,
        Stats {
            legacy: 2,
            army: -1,
            treasury: 0
        },
        "negatives must not clamp"
    );
    assert_eq!(s.scene_id, "field");
    assert_eq!(s.hand.len(), HAND_SIZE, "hand must refill");
    assert_eq!(s.log.len(), 1);
    assert!(
        s.log[0].contains("March") && s.log[0].contains("Legacy +2"),
        "want entry with choice text and deltas: {}",
        s.log[0]
    );
}

#[test]
fn play_card_deck_exhaustion_reshuffles_discard() {
    let lib = test_lib();
    // 5-card deck, opening hand of 4: six plays force repeated reshuffles.
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into(), "c".into(), "d".into()],
        deck: vec!["e".into()],
        ..State::default()
    };
    let plays = ["a", "b", "c", "d", "e", "a"];
    for (i, id) in plays.iter().enumerate() {
        s.play_card(id, &lib).unwrap_or_else(|e| panic!("play {} ({id}): {e}", i + 1));
        assert_eq!(
            s.hand.len(),
            HAND_SIZE,
            "after play {}: discard must reshuffle",
            i + 1
        );
        // Invariant: every card lives in exactly one pile — no loss, no dupes.
        let mut piles = s.hand.clone();
        piles.extend(s.deck.iter().cloned());
        piles.extend(s.discard.iter().cloned());
        piles.sort();
        piles.dedup();
        assert_eq!(piles.len(), 5, "after play {}: want 5 unique cards", i + 1);
    }
    assert_eq!(s.log.len(), plays.len());
}

#[test]
fn play_card_not_in_hand() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into()],
        ..State::default()
    };
    assert!(s.play_card("z", &lib).is_err(), "unknown id must error");
}

#[test]
fn gain_card_reaches_hand() {
    let mut lib = test_lib();
    lib.cards.insert(
        "a".into(),
        Card {
            id: "a".into(),
            name: "Alpha".into(),
            effects: vec![Effect {
                kind: "gain_card".into(),
                card_id: "e".into(),
                ..Effect::default()
            }],
            ..Card::default()
        },
    );
    let mut s = State {
        scene_id: "x".into(),
        hand: vec!["a".into(), "b".into(), "c".into()],
        ..State::default()
    };
    s.play_card("a", &lib).expect("play");
    assert!(
        s.hand.contains(&"e".to_string()),
        "gained card e must be drawn (deck top): {:?}",
        s.hand
    );
}

#[test]
fn log_cap() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        ..State::default()
    };
    let choice = Choice {
        text: "Wait".into(),
        ..Choice::default()
    };
    for _ in 0..(MAX_LOG_LEN + 10) {
        s.choose(&choice, &lib).expect("choose");
    }
    assert_eq!(s.log.len(), MAX_LOG_LEN, "newest entries must be kept");
    assert!(s.log.last().unwrap().contains("Wait"));
}

// --- The Persian response (threat track) ---------------------------------

/// Drives the private endTurn via a plain no-effect choice on a custom
/// scene, capturing the equivalent behavior the Go test checked directly.
fn spend_turn(s: &mut State, lib: &Library) {
    let choice = Choice {
        text: "acted".into(),
        ..Choice::default()
    };
    s.choose(&choice, lib).expect("turn");
}

#[test]
fn threat_raid_crossing_eights() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        threat: 7,
        stats: Stats {
            treasury: 3,
            ..Stats::default()
        },
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        ..State::default()
    };
    spend_turn(&mut s, &lib);
    assert_eq!(s.threat, 8);
    assert_eq!(s.stats.treasury, 2, "raid must cost 1 treasury");
    assert!(
        s.log.iter().any(|l| l.contains("raid")),
        "raid not logged: {:?}",
        s.log
    );
    // Staying at 8 must not raid again.
    let before = s.stats.treasury;
    spend_turn(&mut s, &lib);
    assert_eq!(s.stats.treasury, before, "raid re-triggered without crossing");
}

#[test]
fn threat_ambush_discards_hand_card() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        threat: 13,
        hand: vec!["a".into(), "b".into(), "c".into()],
        ..State::default()
    };
    spend_turn(&mut s, &lib);
    assert_eq!(s.threat, 14);
    assert_eq!(s.hand.len(), 2);
    assert_eq!(s.discard.len(), 1, "ambush must move one hand card to discard");
    let mut kept = s.hand.clone();
    kept.extend(s.discard.iter().cloned());
    kept.sort();
    assert_eq!(kept, vec!["a", "b", "c"], "cards must be conserved");
}

#[test]
fn threat_max_forces_battle_once() {
    let lib = test_lib();
    let mut s = State {
        scene_id: "x".into(),
        threat: 19,
        hand: vec!["a".into()],
        deck: vec!["b".into()],
        ..State::default()
    };
    spend_turn(&mut s, &lib);
    assert_eq!(s.scene_id, "battle");
    assert_eq!(s.threat, THREAT_MAX);
    assert!(
        s.log.iter().any(|l| l.contains("battle is joined")),
        "battle not logged: {:?}",
        s.log
    );
    // Waiting a turn in the battle scene must not re-force the transition.
    spend_turn(&mut s, &lib);
    assert_eq!(s.scene_id, "battle", "battle re-forced");
    assert_eq!(s.threat, THREAT_MAX);
}

#[test]
fn threat_effect_reduces_and_clamps() {
    let mut lib = test_lib();
    lib.cards.insert(
        "calm".into(),
        Card {
            id: "calm".into(),
            name: "Calm".into(),
            cost: 0,
            effects: vec![Effect {
                kind: "threat".into(),
                value: -5,
                ..Effect::default()
            }],
            ..Card::default()
        },
    );
    let mut s = State {
        scene_id: "x".into(),
        threat: 2,
        hand: vec!["calm".into()],
        ..State::default()
    };
    s.play_card("calm", &lib).expect("play calm");
    // Clamped to 0 by the effect, then the turn's own advance adds 1.
    assert_eq!(s.threat, 1, "clamped, then advanced");
    assert!(
        s.log.iter().any(|l| l.contains("Threat -5")),
        "reduction not logged: {:?}",
        s.log
    );
}

#[test]
fn terminal_scene_is_exempt_from_threat() {
    let lib = Library {
        scenes: [(
            "end".to_string(),
            Scene {
                id: "end".into(),
                ending: "defeat".into(),
                threat: 2,
                ..Scene::default()
            },
        )]
        .into_iter()
        .collect::<HashMap<_, _>>(),
        ..Library::default()
    };
    let mut s = State {
        scene_id: "end".into(),
        threat: 19,
        hand: vec!["a".into()],
        ..State::default()
    };
    spend_turn(&mut s, &lib);
    assert_eq!(s.threat, 19, "terminal scene must be exempt");
    assert_eq!(s.scene_id, "end");
}

#[test]
fn threat_persists_through_save() {
    let s = State {
        scene_id: "x".into(),
        threat: 9,
        hand: vec!["a".into()],
        ..State::default()
    };
    let b = serde_json::to_string(&s).unwrap();
    let back: State = serde_json::from_str(&b).unwrap();
    assert_eq!(back.threat, 9, "threat not persisted");
    assert_eq!(back.hand, s.hand);
    assert_eq!(back.scene_id, "x");
}
