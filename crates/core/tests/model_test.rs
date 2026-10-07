//! Ports internal/app/model_test.go.

use std::fs;

use alexander_core::app::{AppError, EffectKind, Model, HISTORY_FILE, SAVE_FILE};
use alexander_core::content::{Card, Choice, Effect, Library, Scene};
use alexander_core::game;

/// testLib builds a tiny campaign:
///
/// ```text
/// title --(free choice)--> field --(win)--> ending "triumph"
///   |                        |
///   └ cards: free, costly    └ arrival grants: gift
/// ```
///
/// "free" costs 0 and grants Legacy +1; "costly" costs 3 (unaffordable at
/// the start); "gift" is only obtainable via field's arrival pool.
fn test_lib() -> Library {
    let mut lib = Library::default();
    for (id, name, text, cost) in [
        ("free", "Free Blade", "Swing.", 0),
        ("costly", "Costly Guard", "Pricy.", 3),
        ("gift", "Field Gift", "From the field.", 0),
    ] {
        let mut card = Card {
            id: id.into(),
            name: name.into(),
            text: text.into(),
            cost,
            start: id != "gift",
            ..Card::default()
        };
        if id == "free" {
            card.effects = vec![Effect {
                kind: "stat".into(),
                delta: Some([("legacy".to_string(), 1)].into_iter().collect()),
                ..Effect::default()
            }];
        }
        lib.cards.insert(id.into(), card);
    }
    lib.scenes.insert(
        "title".into(),
        Scene {
            id: "title".into(),
            text: "The beginning.".into(),
            choices: vec![Choice {
                text: "March".into(),
                effects: vec![Effect {
                    kind: "goto".into(),
                    next: "field".into(),
                    ..Effect::default()
                }],
                ..Choice::default()
            }],
            ..Scene::default()
        },
    );
    lib.scenes.insert(
        "field".into(),
        Scene {
            id: "field".into(),
            text: "The plain.".into(),
            cards: vec!["gift".into()],
            choices: vec![Choice {
                text: "Win".into(),
                effects: vec![Effect {
                    kind: "goto".into(),
                    next: "ending".into(),
                    ..Effect::default()
                }],
                ..Choice::default()
            }],
            ..Scene::default()
        },
    );
    lib.scenes.insert(
        "ending".into(),
        Scene {
            id: "ending".into(),
            text: "It is done.".into(),
            ending: "triumph".into(),
            ..Scene::default()
        },
    );
    lib
}

fn new_test_model() -> Model {
    Model::new(test_lib(), None)
}

fn find_hand<'a>(v: &'a alexander_core::app::View, id: &str) -> &'a alexander_core::app::CardView {
    v.hand
        .iter()
        .find(|c| c.id == id)
        .unwrap_or_else(|| panic!("card {id:?} not in hand"))
}

fn effects_of(r: &alexander_core::app::Result_, kind: EffectKind) -> Vec<&alexander_core::app::Effect> {
    r.effects.iter().filter(|e| e.kind == kind).collect()
}

#[test]
fn new_game_starts_fresh_run() {
    let mut m = new_test_model();
    assert!(!m.has_run(), "fresh model should have no run");
    let r = m.new_game();
    assert!(m.has_run());
    assert!(r.err.is_none());
    let v = &r.view;
    assert_eq!(v.scene_id, "title");
    assert_eq!(v.hand.len(), 2, "only free + costly are start cards");
    assert_eq!(v.deck_count, 0);
    assert_eq!(v.discard_count, 0);
    assert_eq!(v.choices.len(), 1);
}

#[test]
fn actions_without_run_are_refused() {
    let mut m = new_test_model();
    for r in [m.play_card("free"), m.choose(0), m.scout(), m.shuffle()] {
        assert_eq!(r.err, Some(AppError::NoRun), "want refusal");
        assert!(r.effects.is_empty());
    }
}

#[test]
fn play_card_emits_stat_effect_and_refills() {
    let mut m = new_test_model();
    m.new_game();
    let r = m.play_card("free");
    assert!(r.err.is_none(), "{:?}", r.err);
    let stats = effects_of(&r, EffectKind::Stat);
    assert_eq!(stats.len(), 1);
    assert_eq!(stats[0].stat, "legacy");
    assert_eq!(stats[0].delta, 1);
    let v = &r.view;
    assert_eq!(v.stats.legacy, 1);
    assert_eq!((v.turns, v.cards_played), (1, 1));
    // The tiny deck recycles the discard immediately on refill, so the
    // run still owns both cards and the hand is full again.
    assert_eq!(v.hand.len(), 2);
    assert_eq!(v.deck_total, 2);
    assert_eq!(
        v.log.first().map(String::as_str),
        Some("Played Free Blade. Legacy +1, Threat +1.")
    );
}

#[test]
fn play_card_refusal_leaves_state_untouched() {
    let mut m = new_test_model();
    m.new_game();
    let before = m.view();
    let r = m.play_card("costly"); // costs 3, treasury is 0
    assert!(r.err.is_some(), "want refusal for unaffordable card");
    assert!(r.effects.is_empty(), "refused action emitted effects");
    let after = m.view();
    assert_eq!(after.turns, before.turns);
    assert_eq!(after.stats, before.stats, "state changed on refused action");
    // The refusal is part of the action's own view, never persisted.
    let err_text = r.err.unwrap().to_string();
    assert_eq!(
        r.view.log.first().map(String::as_str),
        Some(format!("Error: {err_text}").as_str()),
        "error not surfaced in result log"
    );
    assert_eq!(
        m.view().log.len(),
        before.log.len(),
        "refusal leaked into the persisted log"
    );
    // The refused card must render as unplayable in the view.
    assert!(!find_hand(&after, "costly").playable, "not playable at 0 treasury");
}

#[test]
fn choose_triggers_scene_arrival_and_gain() {
    let mut m = new_test_model();
    m.new_game();
    let r = m.choose(0);
    assert!(r.err.is_none(), "{:?}", r.err);
    assert_eq!(r.view.scene_id, "field");
    let gains = effects_of(&r, EffectKind::CardGained);
    assert_eq!(gains.len(), 1);
    assert_eq!(gains[0].card_id, "gift");
    let scenes = effects_of(&r, EffectKind::SceneChanged);
    assert_eq!(scenes.len(), 1);
    assert_eq!((scenes[0].from.as_str(), scenes[0].to.as_str()), ("title", "field"));
}

#[test]
fn ending_records_run_once_and_locks_actions() {
    let mut m = new_test_model();
    m.new_game();
    let r = m.choose(0); // title -> field (grants gift)
    assert!(r.err.is_none());
    let r = m.choose(0); // field -> ending
    assert!(r.err.is_none());
    let endings = effects_of(&r, EffectKind::Ending);
    assert_eq!(endings.len(), 1);
    assert_eq!(endings[0].text, "triumph");
    assert_eq!(m.history().len(), 1);
    assert_eq!(m.history()[0].ending, "triumph");
    // The ending scene offers no choices; a card play is refused and
    // must not record a second run.
    let r = m.play_card("free");
    assert!(r.err.is_some(), "post-ending play should be refused");
    assert_eq!(m.history().len(), 1, "history grew");
}

#[test]
fn new_game_after_ending_keeps_history() {
    let mut m = new_test_model();
    m.new_game();
    m.choose(0);
    m.choose(0); // reach ending
    m.new_game();
    assert_eq!(m.history().len(), 1, "only completed runs");
    assert_eq!(m.view().scene_id, "title");
}

#[test]
fn scout_and_shuffle_refusals() {
    let mut m = new_test_model();
    m.new_game();
    // The two-card opening deck is dealt entirely into the hand, so both
    // the deck and the discard start empty: scout and shuffle refuse.
    assert!(m.shuffle().err.is_some(), "shuffle with empty discard should fail");
    assert!(m.scout().err.is_some(), "scout on empty deck should fail");
    assert_eq!(m.view().turns, 0, "refused actions must not spend turns");
}

#[test]
fn persistence_round_trip() {
    let dir = std::env::temp_dir().join(format!("alex-test-{}", std::process::id()));
    fs::create_dir_all(&dir).unwrap();
    let lib = test_lib();
    let mut m = Model::new(lib.clone(), Some(dir.clone()));
    m.new_game();
    m.play_card("free");
    let want = m.view();

    // A second model over the same directory resumes the run.
    let m2 = Model::new(lib, Some(dir.clone()));
    assert!(m2.has_run(), "save not loaded");
    let got = m2.view();
    assert_eq!(got.scene_id, want.scene_id);
    assert_eq!(got.turns, want.turns);
    assert_eq!(got.stats, want.stats, "resumed view mismatch");
    assert_eq!(got.deck_total, want.deck_total);
    assert_eq!(got.hand.len(), want.hand.len());
    fs::remove_dir_all(&dir).unwrap();
}

#[test]
fn history_persistence() {
    let dir = std::env::temp_dir().join(format!("alex-test-hist-{}", std::process::id()));
    fs::create_dir_all(&dir).unwrap();
    let lib = test_lib();
    let mut m = Model::new(lib.clone(), Some(dir.clone()));
    m.new_game();
    m.choose(0);
    m.choose(0); // ending
    let m2 = Model::new(lib, Some(dir.clone()));
    let runs = m2.history();
    assert_eq!(runs.len(), 1);
    assert_eq!(runs[0].ending, "triumph");
    assert_eq!(m2.view().campaigns, 1);
    // The gallery marks triumph discovered.
    for tile in &m2.view().endings {
        if tile.class == "triumph" {
            assert!(tile.discovered, "triumph tile should be discovered");
        }
    }
    fs::remove_dir_all(&dir).unwrap();
}

#[test]
fn corrupt_save_reads_as_empty() {
    let dir = std::env::temp_dir().join(format!("alex-test-corrupt-{}", std::process::id()));
    fs::create_dir_all(&dir).unwrap();
    fs::write(dir.join(SAVE_FILE), "{not json").unwrap();
    fs::write(dir.join(HISTORY_FILE), "nope").unwrap();
    let m = Model::new(test_lib(), Some(dir.clone()));
    assert!(!m.has_run(), "corrupt save should read as no run");
    assert!(m.history().is_empty(), "corrupt history should read as empty");
    fs::remove_dir_all(&dir).unwrap();
}

#[test]
fn speculative_clone_keeps_model_immutable() {
    // Diffing must never leak next-state into the model: play on a clone,
    // refuse to commit, then confirm the model still shows the old view.
    let mut m = new_test_model();
    m.new_game();
    let mut st = m
        .state()
        .expect("run underway")
        .clone();
    st.play_card("free", &test_lib()).expect("play on clone");
    assert_eq!(m.view().stats.legacy, 0, "clone mutated the live model");
    assert_eq!(game::HAND_SIZE, 4);
}

#[test]
fn threat_effects_and_view() {
    let mut m = new_test_model();
    m.new_game();
    m.play_card("free"); // threat +1 from the turn advance
    let v = m.view();
    assert_eq!(v.threat, 1);
    assert_eq!(v.max_threat, game::THREAT_MAX);
    let r = m.play_card("free");
    assert!(r.err.is_none());
    assert!(
        r.effects.iter().any(|e| e.kind == EffectKind::Threat),
        "threat delta not reported in effects"
    );
}

/// The serialized save layout must match the Go desktop field names, so
/// existing saves keep loading.
#[test]
fn save_json_golden_layout() {
    let st = alexander_core::game::State {
        session: "s1".into(),
        scene_id: "field".into(),
        stats: alexander_core::game::Stats {
            legacy: 2,
            army: -1,
            treasury: 3,
        },
        threat: 4,
        hand: vec!["free".into()],
        deck: vec!["costly".into()],
        discard: vec![],
        log: vec!["Marched.".into()],
        granted: vec!["field".into()],
        turns: 7,
        cards_played: 2,
        ..alexander_core::game::State::default()
    };
    let json = serde_json::to_value(&st).unwrap();
    let golden = serde_json::json!({
        "Session": "s1",
        "SceneID": "field",
        "Stats": {"Legacy": 2, "Army": -1, "Treasury": 3},
        "threat": 4,
        "Hand": ["free"],
        "Deck": ["costly"],
        "Log": ["Marched."],
        "Granted": ["field"],
        "Turns": 7,
        "CardsPlayed": 2
    });
    assert_eq!(json, golden, "save field layout must match the Go format");
    // threat 0 is omitted, matching Go's omitempty.
    let zero = serde_json::to_value(&alexander_core::game::State::default()).unwrap();
    assert!(zero.get("threat").is_none(), "zero threat must be omitted: {zero}");
    // Key order matches Go's struct order.
    let text = serde_json::to_string(&st).unwrap();
    let keys: Vec<&str> = ["Session", "SceneID", "Stats", "threat", "Hand", "Deck", "Log", "Granted", "Turns", "CardsPlayed"]
        .iter()
        .copied()
        .collect();
    let mut last = -1;
    for k in keys {
        let pos = text.find(&format!("\"{k}\"")).expect(k) as i32;
        assert!(pos > last, "key {k} out of order in {text}");
        last = pos;
    }
    // A Go-written save (null slices for empty piles) loads unchanged.
    let go_save = r#"{"Session":"","SceneID":"x","Stats":{"Legacy":0,"Army":0,"Treasury":0},"Hand":null,"Deck":null,"Discard":null,"Log":null,"Granted":null,"Turns":3,"CardsPlayed":1}"#;
    let back: alexander_core::game::State = serde_json::from_str(go_save).expect("Go save loads");
    assert_eq!(back.turns, 3);
    assert!(back.hand.is_empty() && back.deck.is_empty());
}

