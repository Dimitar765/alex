//! Structured FX events derived by diffing the state before and after a
//! committed action, so the UI never needs game-rule knowledge.

/// Classifies one visualizable consequence of an action.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EffectKind {
    /// A stat moved. `stat` names the key, `delta` the change.
    Stat,
    /// A card left the hand (played or spent).
    CardLeft,
    /// A card entered the run's pool (scene arrival or a gain effect).
    CardGained,
    /// A card left the pool for good.
    CardGone,
    /// The narrative moved. `from`/`to` hold scene IDs.
    SceneChanged,
    /// The run reached a terminal scene. `text` holds the ending
    /// classification (e.g. "triumph").
    Ending,
    /// The action resolved a random effect.
    Fate,
    /// The threat track moved. `stat` is "threat", `delta` the change.
    Threat,
}

/// One consequence of a committed action. Which fields are meaningful
/// depends on the kind.
#[derive(Debug, Clone, PartialEq)]
pub struct Effect {
    pub kind: EffectKind,
    pub stat: String,
    pub delta: i32,
    pub card_id: String,
    pub from: String,
    pub to: String,
    pub text: String,
}

impl Effect {
    pub fn stat(key: &str, delta: i32) -> Effect {
        Effect {
            kind: EffectKind::Stat,
            stat: key.to_string(),
            delta,
            ..Effect::default()
        }
    }

    pub fn threat(delta: i32) -> Effect {
        Effect {
            kind: EffectKind::Threat,
            stat: "threat".to_string(),
            delta,
            ..Effect::default()
        }
    }

    pub fn card_left(id: &str) -> Effect {
        Effect {
            kind: EffectKind::CardLeft,
            card_id: id.to_string(),
            ..Effect::default()
        }
    }

    pub fn card_gained(id: &str) -> Effect {
        Effect {
            kind: EffectKind::CardGained,
            card_id: id.to_string(),
            ..Effect::default()
        }
    }

    pub fn card_gone(id: &str) -> Effect {
        Effect {
            kind: EffectKind::CardGone,
            card_id: id.to_string(),
            ..Effect::default()
        }
    }

    pub fn scene_changed(from: &str, to: &str) -> Effect {
        Effect {
            kind: EffectKind::SceneChanged,
            from: from.to_string(),
            to: to.to_string(),
            ..Effect::default()
        }
    }

    pub fn ending(class: &str) -> Effect {
        Effect {
            kind: EffectKind::Ending,
            text: class.to_string(),
            ..Effect::default()
        }
    }

    pub fn fate() -> Effect {
        Effect {
            kind: EffectKind::Fate,
            ..Effect::default()
        }
    }
}

impl Default for Effect {
    fn default() -> Self {
        Effect {
            kind: EffectKind::Stat,
            stat: String::new(),
            delta: 0,
            card_id: String::new(),
            from: String::new(),
            to: String::new(),
            text: String::new(),
        }
    }
}
