//! The table screen: the play side (scene, stats, threat meter,
//! choices, log, ending block) plus the deck-side controls the 3D hand
//! lives under.

use bevy::prelude::*;

use crate::screens::{
    button, focus_marker, panel as ui_panel, text, ActionButton, DisabledChoice, Face, ScreenCtx,
};
use crate::theme;
use crate::{FOCUS_SCOUT, FOCUS_SHUFFLE, GameAction, ViewCache};
// Layout constants (Go newLayout, design space 1280×800).
const MARGIN_X: f32 = 40.0;
const MARGIN_Y: f32 = 46.0;
const GAP: f32 = 36.0;
const TABLE_W: f32 = 570.0;
const SCENE_H: f32 = 300.0;
const SCENE_X: f32 = MARGIN_X + TABLE_W + GAP; // 646
const SCENE_W: f32 = crate::DESIGN_W - 2.0 * MARGIN_X - TABLE_W - GAP; // 594
const LOG_Y: f32 = MARGIN_Y + SCENE_H + 24.0; // 370
const LOG_H: f32 = crate::DESIGN_H - 2.0 * MARGIN_Y - SCENE_H - 24.0;

pub fn build(mut ctx: ScreenCtx<'_, '_, '_>, root: Entity) {
    let view = &ctx.view;
    let v_ending = !view.scene.ending.is_empty();

    // Right column background (the left side shows the 3D table).
    let right = ctx
        .commands
        .spawn((
            Node {
                position_type: PositionType::Absolute,
                left: Val::Px(SCENE_X - 20.0),
                top: Val::Px(0.0),
                width: Val::Px(crate::DESIGN_W - SCENE_X + 20.0),
                height: Val::Percent(100.0),
                ..default()
            },
            BackgroundColor(theme::BG),
        ))
        .id();
    ctx.commands.entity(root).add_children(&[right]);

    // Scene panel (battle scenes get the red accent border).
    let border = if view.scene_id == alexander_core::content::BATTLE_SCENE {
        theme::BATTLE_ACCENT
    } else {
        theme::LINE
    };
    let panel = ui_panel(ctx.commands, SCENE_X, MARGIN_Y, SCENE_W, SCENE_H, border);
    ctx.commands.entity(root).add_children(&[panel]);

    // Scene flavor text (italic serif like the web build), wrapped.
    text(ctx.commands, ctx.theme,
        Some(panel),
        &view.scene.text,
        Face::Italic(theme::FACE_SCENE),
        theme::INK,
        SCENE_X + 20.0,
        MARGIN_Y + 18.0,
        SCENE_W - 40.0,
        true,
    );

    // Stats row at a stable anchor under four wrapped text lines.
    let stats_y = MARGIN_Y + 18.0 + 4.0 * 26.0 + 10.0;
    text(ctx.commands, ctx.theme,
        Some(panel),
        &format!(
            "Legacy {}   Army {}   Treasury {}",
            view.stats.legacy, view.stats.army, view.stats.treasury
        ),
        Face::Serif(theme::FACE_BODY),
        theme::GOLD,
        SCENE_X + 20.0,
        stats_y,
        0.0,
        false,
    );

    // Threat meter: ten notches, two threat per notch.
    let meter_y = stats_y + 34.0;
    let meter_color = threat_color(view.threat);
    text(ctx.commands, ctx.theme,
        Some(panel),
        "Threat",
        Face::Serif(theme::FACE_SMALL),
        meter_color,
        SCENE_X + 20.0,
        meter_y,
        0.0,
        false,
    );
    const SEG_W: f32 = 16.0;
    const SEG_H: f32 = 10.0;
    const SEG_GAP: f32 = 4.0;
    let notches = (view.max_threat / 2) as usize;
    for i in 0..notches {
        let filled = view.threat as usize > i * 2;
        let e = ctx
            .commands
            .spawn((
                Node {
                    position_type: PositionType::Absolute,
                    left: Val::Px(SCENE_X + 20.0 + 58.0 + i as f32 * (SEG_W + SEG_GAP)),
                    top: Val::Px(meter_y + 2.0),
                    width: Val::Px(SEG_W),
                    height: Val::Px(SEG_H),
                    ..default()
                },
                BackgroundColor(if filled { meter_color } else { theme::LINE }),
            ))
            .id();
        ctx.commands.entity(panel).add_children(&[e]);
    }
    text(ctx.commands, ctx.theme,
        Some(panel),
        &format!("{}/{}", view.threat, view.max_threat),
        Face::Serif(theme::FACE_SMALL),
        meter_color,
        SCENE_X + 20.0 + 58.0 + notches as f32 * (SEG_W + SEG_GAP) + 6.0,
        meter_y,
        0.0,
        false,
    );

    // Record the FX anchors.
    ctx.anchors.stats = Vec2::new(SCENE_X + 20.0, stats_y);
    ctx.anchors.threat = Vec2::new(SCENE_X + 20.0, meter_y);
    ctx.anchors.scene = Vec2::new(SCENE_X, MARGIN_Y);
    ctx.anchors.scene_size = Vec2::new(SCENE_W, SCENE_H);

    let log_lines: Vec<(usize, String)> = view
        .log
        .iter()
        .take(14)
        .enumerate()
        .map(|(i, l)| (i, l.clone()))
        .collect();
    let below = meter_y + 34.0;
    if v_ending {
        build_ending(&mut ctx, right, panel, below);
    } else {
        build_choices(&mut ctx, right, panel, below);
        build_deck_side(&mut ctx, root);
    }

    // Log panel, newest first.
    let log_panel = ui_panel(ctx.commands, SCENE_X, LOG_Y, SCENE_W, LOG_H, theme::LINE);
    ctx.commands.entity(root).add_children(&[log_panel]);
    let mut y = LOG_Y + 16.0;
    for (i, line) in &log_lines {
        text(ctx.commands, ctx.theme,
            Some(log_panel),
            line,
            Face::Serif(theme::FACE_SMALL),
            if *i == 0 { theme::INK } else { theme::MUTED },
            SCENE_X + 18.0,
            y,
            SCENE_W - 36.0,
            true,
        );
        y += 24.0;
    }
}

/// The ending block: badge, summary, restart, gallery.
fn build_ending(ctx: &mut ScreenCtx, right: Entity, panel: Entity, y: f32) {
    let view = &ctx.view;
    let color = theme::ending_color(&view.scene.ending);
    text(ctx.commands, ctx.theme,
        Some(panel),
        &view.scene.ending,
        Face::Serif(theme::FACE_HEADER),
        color,
        SCENE_X + 20.0,
        y,
        140.0,
        false,
    );
    text(ctx.commands, ctx.theme,
        Some(panel),
        &format!(
            "The campaign ends after {} turns · {} cards played · {} campaigns completed",
            view.turns, view.cards_played, view.campaigns
        ),
        Face::Serif(theme::FACE_SMALL),
        theme::MUTED,
        SCENE_X + 20.0,
        y + 44.0,
        SCENE_W - 40.0,
        true,
    );
    button(ctx.commands, ctx.theme,
        right,
        "Begin a new campaign",
        SCENE_X + 20.0,
        y + 92.0,
        240.0,
        42.0,
        GameAction::NewGame,
        true,
    );
    // Gallery strip under the restart button.
    for (i, tile) in view.endings.iter().enumerate() {
        let label = if tile.discovered { tile.class.clone() } else { "???".to_string() };
        let tile_color = if tile.discovered {
            theme::ending_color(&tile.class)
        } else {
            theme::MUTED
        };
        text(ctx.commands, ctx.theme,
            Some(right),
            &label,
            Face::Serif(theme::FACE_HINT),
            tile_color,
            SCENE_X + 20.0 + i as f32 * 72.0,
            y + 148.0,
            60.0,
            false,
        );
    }
}

/// 2–3 choice buttons with requirement hints.
fn build_choices(ctx: &mut ScreenCtx, right: Entity, panel: Entity, y: f32) {
    let view = ctx.view;
    let mut y = y;
    for c in &view.choices {
        let color = if c.available { theme::INK } else { theme::MUTED };
        let mut label = format!("{}. {}", c.index + 1, c.text);
        if !c.requires.is_empty() {
            label.push_str(&format!("  — requires {}", c.requires));
        }
        text(ctx.commands, ctx.theme,
            Some(panel),
            &label,
            Face::Serif(theme::FACE_BODY),
            color,
            SCENE_X + 20.0,
            y,
            SCENE_W - 40.0,
            true,
        );
        let line = ctx
            .commands
            .spawn((
                Node {
                    position_type: PositionType::Absolute,
                    left: Val::Px(SCENE_X + 12.0),
                    top: Val::Px(y - 4.0),
                    width: Val::Px(SCENE_W - 24.0),
                    height: Val::Px(28.0),
                    ..default()
                },
                Button,
                Interaction::default(),
                ActionButton(GameAction::Choose(c.index)),
                BackgroundColor(Color::NONE),
            ))
            .id();
        ctx.commands.entity(panel).add_children(&[line]);
        if !c.available {
            ctx.commands.entity(line).insert(DisabledChoice);
        }
        y += 34.0;
    }
    if !view.choices.is_empty() {
        text(ctx.commands, ctx.theme,
            Some(panel),
            &format!("press 1–{}", view.choices.len()),
            Face::Serif(theme::FACE_HINT),
            theme::MUTED,
            SCENE_X + 20.0,
            y + 4.0,
            0.0,
            false,
        );
    }
    let _ = right;
}

/// The deck-side column over the 3D table: deck counts, Scout and
/// Shuffle buttons (with Go's disabled conditions), and the deck
/// inspector with thumbnail tiles.
fn build_deck_side(ctx: &mut ScreenCtx, root: Entity) {
    let view = &ctx.view;
    let side = ctx
        .commands
        .spawn(Node {
            position_type: PositionType::Absolute,
            left: Val::Px(MARGIN_X + 4.0),
            top: Val::Px(crate::DESIGN_H - MARGIN_Y - 96.0),
            width: Val::Px(TABLE_W - 8.0),
            height: Val::Px(96.0),
            ..default()
        })
        .id();
    ctx.commands.entity(root).add_children(&[side]);

    let row_y = crate::DESIGN_H - MARGIN_Y - 96.0;
    text(ctx.commands, ctx.theme,
        Some(side),
        &format!("Deck · {}", view.deck_count),
        Face::Serif(theme::FACE_SMALL),
        theme::GOLD,
        MARGIN_X + 4.0 - MARGIN_X,
        row_y + 8.0 - row_y,
        0.0,
        false,
    );
    text(ctx.commands, ctx.theme,
        Some(side),
        &format!("Discard · {}", view.discard_count),
        Face::Serif(theme::FACE_SMALL),
        theme::MUTED,
        0.0,
        30.0,
        0.0,
        false,
    );

    let focus = ctx.focus.0.clone();
    let scout_x = TABLE_W - 228.0;
    let shuffle_x = TABLE_W - 110.0;
    if focus.as_deref() == Some(FOCUS_SCOUT) {
        focus_marker(ctx.commands, ctx.theme, side, scout_x - 20.0, 6.0);
    }
    if focus.as_deref() == Some(FOCUS_SHUFFLE) {
        focus_marker(ctx.commands, ctx.theme, side, shuffle_x - 20.0, 6.0);
    }
    button(ctx.commands, ctx.theme,
        side,
        "Scout",
        scout_x,
        4.0,
        110.0,
        36.0,
        GameAction::Scout,
        false,
    );
    let shuffle_disabled = view.discard_count == 0 || view.stats.treasury < 1;
    button(ctx.commands, ctx.theme,
        side,
        &format!("Shuffle · {}", view.discard_count),
        shuffle_x,
        4.0,
        110.0,
        36.0,
        GameAction::Shuffle,
        false,
    );
    let _ = shuffle_disabled;

    // Deck inspector: aggregated owned cards with mini-card thumbnails.
    let inspector_y = row_y - 40.0 - (view.deck.len() as f32 * 24.0).min(240.0);
    let list = ctx
        .commands
        .spawn(Node {
            position_type: PositionType::Absolute,
            left: Val::Px(MARGIN_X + 4.0),
            top: Val::Px(inspector_y),
            width: Val::Px(TABLE_W - 8.0),
            height: Val::Px((view.deck.len() as f32 * 24.0).min(240.0)),
            ..default()
        })
        .id();
    ctx.commands.entity(root).add_children(&[list]);
    for (i, entry) in view.deck.iter().take(10).enumerate() {
        let y = i as f32 * 24.0;
        if let Some(front) = ctx.painted.fronts.get(&entry.id) {
            let thumb = ctx
                .commands
                .spawn((
                    Node {
                        position_type: PositionType::Absolute,
                        left: Val::Px(0.0),
                        top: Val::Px(y),
                        width: Val::Px(17.0),
                        height: Val::Px(22.0),
                        ..default()
                    },
                    ImageNode {
                        image: front.clone(),
                        ..default()
                    },
                ))
                .id();
            ctx.commands.entity(list).add_children(&[thumb]);
        }
        text(ctx.commands, ctx.theme,
            Some(list),
            &format!("{} ×{}", entry.name, entry.count),
            Face::Serif(theme::FACE_HINT),
            theme::MUTED,
            24.0,
            y + 4.0,
            0.0,
            false,
        );
    }
}

fn threat_color(threat: i32) -> Color {
    if threat >= alexander_core::game::THREAT_AMBUSH {
        theme::THREAT_CRIT
    } else if threat >= alexander_core::game::THREAT_RAID {
        theme::THREAT_WARN
    } else {
        theme::MUTED
    }
}

/// Keyboard/gamepad handling on the table screen: digits choose, arrows
/// cycle hand + scout + shuffle, Enter confirms, Esc backs out.
pub fn keys(
    nav: &crate::input::Nav,
    view: &ViewCache,
    focus: &mut bevy::ecs::system::ResMut<crate::FocusId>,
    writer: &mut MessageWriter<GameAction>,
) {
    let v = &view.view;
    if nav.back {
        writer.write(GameAction::GoTitle);
        return;
    }
    if !v.scene.ending.is_empty() {
        if nav.confirm {
            writer.write(GameAction::NewGame);
        }
        return;
    }

    // Number keys pick a choice.
    if let Some(d) = nav.digit {
        if let Some(choice) = v.choices.get(d) {
            if choice.available {
                writer.write(GameAction::Choose(d));
            } else {
                writer.write(GameAction::Choose(d)); // engine refusal surfaces the error
            }
        }
        return;
    }

    // Focus cycle: hand cards, then Scout, then Shuffle.
    let focusables = crate::hand3d::focusables(v);
    let current = focus
        .0
        .clone()
        .unwrap_or_else(|| focusables.first().cloned().unwrap_or_default());
    let idx = focusables.iter().position(|f| *f == current).unwrap_or(0);
    let step = nav.horizontal_step();
    if step != 0 {
        let next = (idx as i32 + step).rem_euclid(focusables.len() as i32) as usize;
        focus.0 = Some(focusables[next].clone());
        return;
    }
    if nav.confirm {
        match current.as_str() {
            FOCUS_SCOUT => {
                writer.write(GameAction::Scout);
            }
            FOCUS_SHUFFLE => {
                writer.write(GameAction::Shuffle);
            }
            id => {
                if let Some(card) = v.hand.iter().find(|c| c.id == id) {
                    writer.write(GameAction::PlayCard(card.id.clone()));
                }
            }
        }
    }
}
