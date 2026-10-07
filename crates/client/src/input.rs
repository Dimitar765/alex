//! Input plumbing: keyboard, mouse, and gamepad navigation with edge
//! detection, plus the persisted player settings (mute, reduced motion,
//! FX toggle) stored next to the saves directory.

use std::fs;
use std::path::PathBuf;

use bevy::prelude::*;

use bevy::window::PrimaryWindow;
/// Player-facing settings, persisted as `settings.json` beside the saves
/// directory (`<config>/Alexander/settings.json`).
#[derive(Resource, Clone, Copy, Default)]
pub struct Settings {
    pub muted: bool,
    pub reduced_motion: bool,
    pub fx_off: bool,
}


impl Settings {
    pub fn path() -> Option<PathBuf> {
        crate::saves_root().map(|root| root.join("settings.json"))
    }

    pub fn load() -> Settings {
        let Some(path) = Self::path() else {
            return Settings::default();
        };
        let Ok(b) = fs::read(path) else {
            return Settings::default();
        };
        serde_json::from_slice(&b).unwrap_or_default()
    }

    pub fn save(&self) {
        if let Some(path) = Self::path() {
            if let Some(dir) = path.parent() {
                let _ = fs::create_dir_all(dir);
            }
            if let Ok(b) = serde_json::to_vec_pretty(self) {
                let _ = fs::write(path, b);
            }
        }
    }
}

impl serde::Serialize for Settings {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        use serde::ser::SerializeStruct;
        let mut s = serializer.serialize_struct("Settings", 3)?;
        s.serialize_field("mute", &self.muted)?;
        s.serialize_field("reduced_motion", &self.reduced_motion)?;
        s.serialize_field("fx_off", &self.fx_off)?;
        s.end()
    }
}

impl<'de> serde::Deserialize<'de> for Settings {
    fn deserialize<D: serde::Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        #[derive(serde::Deserialize)]
        struct Raw {
            #[serde(default)]
            mute: bool,
            #[serde(default)]
            reduced_motion: bool,
            #[serde(default)]
            fx_off: bool,
        }
        let raw = Raw::deserialize(deserializer)?;
        Ok(Settings {
            muted: raw.mute,
            reduced_motion: raw.reduced_motion,
            fx_off: raw.fx_off,
        })
    }
}

/// The motion/FX gates every effect consults.
pub fn effects_enabled(settings: &Settings) -> bool {
    !settings.fx_off && !settings.reduced_motion
}

// --- edge-detected navigation ----------------------------------------------

/// Stick axis magnitude treated as "pressed".
pub const STICK_DEADZONE: f32 = 0.5;

/// One tick of navigational intent, aggregated across keyboard, mouse,
/// and every connected gamepad (any pad acts).
#[derive(Resource, Default, Clone, Copy)]
pub struct Nav {
    pub left: bool,
    pub right: bool,
    pub up: bool,
    pub down: bool,
    pub confirm: bool,
    pub back: bool,
    /// The M/F/settings toggle edges.
    pub toggle_mute: bool,
    pub toggle_motion: bool,
    pub toggle_fx: bool,
    /// 1..=9 selects that scene choice.
    pub digit: Option<usize>,
    /// Mouse click position in window pixels (top-left origin), and
    /// whether it was just pressed.
    pub click: Option<Vec2>,
    pub cursor: Vec2,
}

impl Nav {
    pub fn horizontal_step(&self) -> i32 {
        (self.right as i32) - (self.left as i32)
    }

    pub fn vertical_step(&self) -> i32 {
        (self.down as i32) - (self.up as i32)
    }
}

/// Previous-frame gamepad state for edge detection.
#[derive(Resource, Default, Clone, Copy)]
pub struct PadPrev {
    left: bool,
    right: bool,
    up: bool,
    down: bool,
    a: bool,
    b: bool,
}

pub fn update_nav(
    mut nav: ResMut<Nav>,
    keys: Res<ButtonInput<KeyCode>>,
    mouse: Res<ButtonInput<MouseButton>>,
    pad_entities: Query<&Gamepad>,
    mut pad_prev: Local<PadPrev>,
    windows: Query<&Window, With<PrimaryWindow>>,
) {
    *nav = Nav {
        cursor: nav.cursor,
        ..Nav::default()
    };

    // Keyboard.
    if keys.just_pressed(KeyCode::ArrowLeft) {
        nav.left = true;
    }
    if keys.just_pressed(KeyCode::ArrowRight) {
        nav.right = true;
    }
    if keys.just_pressed(KeyCode::ArrowUp) {
        nav.up = true;
    }
    if keys.just_pressed(KeyCode::ArrowDown) {
        nav.down = true;
    }
    if keys.just_pressed(KeyCode::Enter) || keys.just_pressed(KeyCode::NumpadEnter) {
        nav.confirm = true;
    }
    if keys.just_pressed(KeyCode::Escape) || keys.just_pressed(KeyCode::Backspace) {
        nav.back = true;
    }
    if keys.just_pressed(KeyCode::KeyM) {
        nav.toggle_mute = true;
    }
    if keys.just_pressed(KeyCode::KeyF) {
        nav.toggle_motion = true;
    }
    if keys.just_pressed(KeyCode::KeyX) {
        nav.toggle_fx = true;
    }
    for (i, code) in [
        KeyCode::Digit1,
        KeyCode::Digit2,
        KeyCode::Digit3,
        KeyCode::Digit4,
        KeyCode::Digit5,
        KeyCode::Digit6,
        KeyCode::Digit7,
        KeyCode::Digit8,
        KeyCode::Digit9,
    ]
    .into_iter()
    .enumerate()
    {
        if keys.just_pressed(code) {
            nav.digit = Some(i);
        }
    }

    // Mouse.
    if let Ok(window) = windows.single()
        && let Some(pos) = window.cursor_position()
    {
        nav.cursor = pos;
    }
    if mouse.just_pressed(MouseButton::Left) {
        nav.click = Some(nav.cursor);
    }

    // Gamepads: d-pad or left stick moves (with edge detection), A
    // confirms, B backs out. Any pad acts.
    let mut pad = PadPrev::default();
    for gamepad in pad_entities.iter() {
        let stick = gamepad.left_stick();
        pad.left |= gamepad.pressed(GamepadButton::DPadLeft) || stick.x < -STICK_DEADZONE;
        pad.right |= gamepad.pressed(GamepadButton::DPadRight) || stick.x > STICK_DEADZONE;
        pad.up |= gamepad.pressed(GamepadButton::DPadUp) || stick.y > STICK_DEADZONE;
        pad.down |= gamepad.pressed(GamepadButton::DPadDown) || stick.y < -STICK_DEADZONE;
        pad.a |= gamepad.pressed(GamepadButton::South);
        pad.b |= gamepad.pressed(GamepadButton::East);
    }
    let prev = *pad_prev;
    nav.left |= pad.left && !prev.left;
    nav.right |= pad.right && !prev.right;
    nav.up |= pad.up && !prev.up;
    nav.down |= pad.down && !prev.down;
    nav.confirm |= pad.a && !prev.a;
    nav.back |= pad.b && !prev.b;
    *pad_prev = pad;
}
