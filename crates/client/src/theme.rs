//! Palette and type sizes, mirroring the web client's style tokens and
//! the Go desktop theme exactly.

use bevy::prelude::*;

pub const BG: Color = Color::srgb(0x15 as f32 / 255.0, 0x13 as f32 / 255.0, 0x12 as f32 / 255.0);
pub const PANEL: Color = Color::srgb(0x21 as f32 / 255.0, 0x1d as f32 / 255.0, 0x1a as f32 / 255.0);
pub const LINE: Color = Color::srgb(0x35 as f32 / 255.0, 0x30 as f32 / 255.0, 0x2a as f32 / 255.0);
pub const GOLD: Color = Color::srgb(0xc9 as f32 / 255.0, 0xa2 as f32 / 255.0, 0x27 as f32 / 255.0);
pub const INK: Color = Color::srgb(0xe8 as f32 / 255.0, 0xe0 as f32 / 255.0, 0xd0 as f32 / 255.0);
pub const MUTED: Color = Color::srgb(0x9a as f32 / 255.0, 0x8f as f32 / 255.0, 0x7d as f32 / 255.0);
pub const CARD_FACE: Color = Color::srgb(0x26 as f32 / 255.0, 0x21 as f32 / 255.0, 0x1c as f32 / 255.0);
#[allow(dead_code)]
pub const CARD_EDGE: Color = Color::srgb(0x4a as f32 / 255.0, 0x3f as f32 / 255.0, 0x2e as f32 / 255.0);
#[allow(dead_code)]
pub const HOVER: Color = Color::srgb(0x3a as f32 / 255.0, 0x32 as f32 / 255.0, 0x29 as f32 / 255.0);

pub const THREAT_WARN: Color = Color::srgb(0xd8 as f32 / 255.0, 0x84 as f32 / 255.0, 0x2c as f32 / 255.0);
pub const THREAT_CRIT: Color = Color::srgb(0xc2 as f32 / 255.0, 0x5b as f32 / 255.0, 0x4e as f32 / 255.0);

pub const END_TRIUMPH: Color = GOLD;
pub const END_LEGACY: Color = Color::srgb(0xd8 as f32 / 255.0, 0xc2 as f32 / 255.0, 0x7a as f32 / 255.0);
pub const END_SETTLE: Color = Color::srgb(0x8f as f32 / 255.0, 0xae as f32 / 255.0, 0x7a as f32 / 255.0);
pub const END_DEFEAT: Color = Color::srgb(0xb0 as f32 / 255.0, 0x6a as f32 / 255.0, 0x5a as f32 / 255.0);
pub const END_DEATH: Color = THREAT_CRIT;

pub const EMBER: Color = END_DEFEAT;
pub const DUST: Color = MUTED;
pub const BATTLE_ACCENT: Color = THREAT_CRIT;

/// Display color for one ending classification.
pub fn ending_color(class: &str) -> Color {
    match class {
        "triumph" => END_TRIUMPH,
        "legacy" => END_LEGACY,
        "settle" => END_SETTLE,
        "defeat" => END_DEFEAT,
        "death" => END_DEATH,
        _ => MUTED,
    }
}

// Face sizes in design-space px (Go theme.go table). The card-face sizes
// also live in paint.rs's texture layout; both kept for parity.
pub const FACE_BODY: f32 = 17.0;
pub const FACE_SMALL: f32 = 14.0;
pub const FACE_SCENE: f32 = 19.0;
#[allow(dead_code)]
pub const FACE_CARD_NAME: f32 = 15.0;
#[allow(dead_code)]
pub const FACE_CARD_TEXT: f32 = 13.0;
pub const FACE_HEADER: f32 = 30.0;
pub const FACE_TITLE: f32 = 44.0;
pub const FACE_HINT: f32 = 12.0;

/// The embedded typefaces: EB Garamond carries body text, its italic the
/// scene flavor, Cinzel the engraved display face.
#[derive(Resource)]
pub struct Theme {
    pub serif: Handle<Font>,
    pub serif_italic: Handle<Font>,
    pub display: Handle<Font>,
}

impl Theme {
    pub fn load(fonts: &mut Assets<Font>) -> Theme {
        let serif = Font::from_bytes(include_bytes!("../../../assets/fonts/EBGaramond.ttf").to_vec());
        let serif_italic =
            Font::from_bytes(include_bytes!("../../../assets/fonts/EBGaramond-Italic.ttf").to_vec());
        let display = Font::from_bytes(include_bytes!("../../../assets/fonts/Cinzel.ttf").to_vec());
        Theme {
            serif: fonts.add(serif),
            serif_italic: fonts.add(serif_italic),
            display: fonts.add(display),
        }
    }

    pub fn face(&self, size: f32) -> TextFont {
        TextFont {
            font: self.serif.clone().into(),
            font_size: FontSize::Px(size),
            ..default()
        }
    }

    pub fn italic(&self, size: f32) -> TextFont {
        TextFont {
            font: self.serif_italic.clone().into(),
            font_size: FontSize::Px(size),
            ..default()
        }
    }

    pub fn display_face(&self, size: f32) -> TextFont {
        TextFont {
            font: self.display.clone().into(),
            font_size: FontSize::Px(size),
            ..default()
        }
    }
}
