//! Ports internal/content/content_test.go.

use alexander_core::content::{self, load, LoadError};

const HAPPY_CARDS: &str = r#"[
  {"id":"phalanx","name":"Phalanx","text":"A wall.","start":true,"effects":[{"kind":"stat","delta":{"army":2}}]},
  {"id":"decreed_alliance","name":"Decreed Alliance","text":"Paper walls.","start":true,"effects":[{"kind":"gain_card","cardId":"phalanx"}]}
]"#;

const HAPPY_SCENES: &str = r#"[
  {"id":"title","text":"You stand.","choices":[{"text":"March","effects":[{"kind":"goto","next":"field"}]}]},
  {"id":"field","text":"A field.","choices":[{"text":"Return","effects":[{"kind":"stat","delta":{"treasury":1}},{"kind":"goto","next":"title"}]}]}
]"#;

fn err_of(cards: &str, scenes: &str) -> String {
    match load(cards, scenes) {
        Err(LoadError::Invalid(m)) | Err(LoadError::Decode(m)) => m,
        Ok(_) => panic!("Load must fail"),
    }
}

#[test]
fn load_valid() {
    let lib = load(HAPPY_CARDS, HAPPY_SCENES).expect("valid content");
    assert!(lib.cards.contains_key("phalanx"));
    assert!(lib.scenes.contains_key("title"));
    let list = lib.card_list();
    assert_eq!(list.len(), 2);
    assert_eq!(list[0].id, "decreed_alliance");
    assert_eq!(list[1].id, "phalanx");
}

#[test]
fn load_dangling_next() {
    let scenes = r#"[
      {"id":"title","text":"You stand.","choices":[{"text":"March","effects":[{"kind":"goto","next":"nowhere"}]}]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(err.contains("nowhere"), "error must name the dangling ID: {err}");
}

#[test]
fn load_aggregates_violations() {
    let cards = r#"[{"id":"x","name":"X","text":"","effects":[{"kind":"gain_card","cardId":"ghost"}]}]"#;
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","requiresCard":"ghost2","effects":[]},{"text":"B","effects":[{"kind":"goto","next":"void"}]}]},
      {"id":"title","text":"","choices":[]}
    ]"#;
    let err = err_of(cards, scenes);
    for want in ["ghost", "ghost2", "void", "duplicate scene id"] {
        assert!(err.contains(want), "error missing {want:?}:\n{err}");
    }
}

#[test]
fn load_unknown_stat_key_rejected() {
    let cards = r#"[{"id":"x","name":"X","text":"","effects":[{"kind":"stat","delta":{"legassy":1}}]}]"#;
    let err = err_of(cards, HAPPY_SCENES);
    assert!(err.contains("legassy"), "must name the bad stat key: {err}");
}

#[test]
fn load_random_effect_validation() {
    let cards = r#"[{"id":"x","name":"X","text":"","effects":[
      {"kind":"random","outcomes":[
        {"weight":0,"effects":[{"kind":"stat","delta":{"army":1}}]},
        {"weight":1,"effects":[{"kind":"gain_card","cardId":"ghost"}]}
      ]}
    ]}]"#;
    let err = err_of(cards, HAPPY_SCENES);
    for want in ["weight must be >= 1", "outcome 1 effect 0", "ghost"] {
        assert!(err.contains(want), "error missing {want:?}:\n{err}");
    }
}

#[test]
fn load_requires_stat_validation() {
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","requiresStat":{"armee":2},"effects":[]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(err.contains("armee"), "must name the bad stat: {err}");
}

#[test]
fn load_ending_rules() {
    let cards = r#"[{"id":"x","name":"X","text":"","effects":[]}]"#;
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next":"end1"}]}]},
      {"id":"end1","text":"","ending":"triumph","choices":[{"text":"nope","effects":[]}]},
      {"id":"end2","text":"","ending":"apolcalypse"}
    ]"#;
    let err = err_of(cards, scenes);
    for want in ["must have no choices", "unknown ending"] {
        assert!(err.contains(want), "error missing {want:?}:\n{err}");
    }
}

#[test]
fn load_unreachable_scene_rejected() {
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next":"field"}]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]},
      {"id":"island","text":"","choices":[{"text":"C","effects":[{"kind":"goto","next":"title"}]}]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(
        err.contains(r#"scene "island" is unreachable"#),
        "must name the unreachable scene: {err}"
    );
}

#[test]
fn load_random_outcomes_make_scenes_reachable() {
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","effects":[
        {"kind":"random","outcomes":[
          {"weight":1,"effects":[{"kind":"goto","next":"field"}]},
          {"weight":1,"effects":[{"kind":"goto","next":"end1"}]}
        ]}
      ]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]},
      {"id":"end1","text":"","ending":"triumph"}
    ]"#;
    load(HAPPY_CARDS, scenes).expect("gotos inside random outcomes must count as edges");
}

#[test]
fn load_consumes_card_requires_card() {
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","consumesCard":true,"effects":[]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(
        err.contains("consumesCard requires a requiresCard"),
        "must name the rule: {err}"
    );
}

#[test]
fn load_negative_cost_rejected() {
    let cards = r#"[{"id":"x","name":"X","text":"","cost":-1,"effects":[]}]"#;
    let err = err_of(cards, HAPPY_SCENES);
    assert!(err.contains("negative cost"), "must name the rule: {err}");
}

#[test]
fn load_unobtainable_card_rejected() {
    let cards = r#"[
      {"id":"core","name":"Core","text":"","start":true,"effects":[]},
      {"id":"ghost_card","name":"Ghost","text":"","effects":[]},
      {"id":"chain","name":"Chain","text":"","start":true,"effects":[{"kind":"gain_card","cardId":"via_chain"}]},
      {"id":"via_chain","name":"ViaChain","text":"","effects":[]}
    ]"#;
    let err = err_of(cards, HAPPY_SCENES);
    assert!(
        err.contains(r#""ghost_card" is unobtainable"#),
        "must name the unobtainable card: {err}"
    );
    assert!(
        !err.contains("via_chain"),
        "gain_card chains from obtainable cards must count as obtainable: {err}"
    );
    assert!(
        !err.contains(r#""core""#),
        "start cards are obtainable by definition: {err}"
    );
}

#[test]
fn load_all_gated_scene_rejected() {
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next": "vault"}]}]},
      {"id":"vault","text":"","choices":[
        {"text":"locked","requiresStat":{"army":5},"effects":[{"kind":"goto","next":"title"}]},
        {"text":"sealed","requiresCard":"phalanx","effects":[{"kind":"goto","next":"title"}]}
      ]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(err.contains("escape path"), "must name the escape-path rule: {err}");
}

#[test]
fn load_scene_pool_unknown_card() {
    let scenes = r#"[
      {"id":"title","text":"","cards":["ghost"],"choices":[{"text":"A","effects":[]}]}
    ]"#;
    let err = err_of(HAPPY_CARDS, scenes);
    assert!(
        err.contains(r#"card pool: unknown card "ghost""#),
        "must name the pool violation: {err}"
    );
}

#[test]
fn load_scene_pool_makes_card_obtainable() {
    let cards = r#"[
      {"id":"core","name":"Core","text":"","start":true,"effects":[]},
      {"id":"regional","name":"Regional","text":"","effects":[]}
    ]"#;
    let scenes = r#"[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next":"field"}]}]},
      {"id":"field","text":"","cards":["regional"],"choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]}
    ]"#;
    load(cards, scenes).expect("scene pools must make cards obtainable");
}

#[test]
fn load_unknown_fields_rejected() {
    let cards = r#"[{"id":"x","name":"X","text":"","effects":[],"bogus":1}]"#;
    match load(cards, HAPPY_SCENES) {
        Err(LoadError::Decode(m)) => assert!(m.contains("cards.json"), "{m}"),
        other => panic!("must reject unknown JSON fields, got {other:?}"),
    }
}

#[test]
fn shipped_embedded_content_validates() {
    // The content embedded in the release binary must always validate.
    let lib = content::load_embedded();
    assert!(lib.scenes.contains_key("title"));
    assert!(lib.scenes.contains_key(content::BATTLE_SCENE));
    assert!(!lib.starting_cards().is_empty());
}
