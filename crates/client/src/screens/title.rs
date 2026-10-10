//! The title screen: campaign start/continue, chronicle, endings
//! gallery, and the settings hints.

use bevy::prelude::*;

use crate::screens::{button, focus_marker, text, Face, ScreenCtx};
use crate::theme;
use crate::{GameAction, ViewCache};

/// Menu entries for the current model state (Continue only mid-campaign).
pub fn menu_labels(has_run: bool) -> Vec<(&'static str, GameAction)> {
    let mut out = Vec::new();
    if has_run {
        out.push(("Continue", GameAction::GoTable));
    }
    out.push(("New game", GameAction::NewGame));
    out.push(("Chronicle", GameAction::GoChronicle));
    out
}

pub fn build(ctx: ScreenCtx<'_, '_, '_>, root: Entity) {
    let view = &ctx.view;
    let bg = ctx
        .commands
        .spawn((
            Node {
                width: Val::Percent(100.0),
                height: Val::Percent(100.0),
                ..default()
            },
            BackgroundColor(theme::BG),
        ))
        .id();
    ctx.commands.entity(root).add_children(&[bg]);

    text(ctx.commands, ctx.theme,
        Some(bg),
        "Alexander",
        Face::Display(theme::FACE_TITLE),
        theme::GOLD,
        48.0,
        40.0,
        0.0,
        false,
    );
    text(ctx.commands, ctx.theme,
        Some(bg),
        "a card adventure",
        Face::Italic(theme::FACE_SCENE),
        theme::MUTED,
        52.0,
        108.0,
        0.0,
        false,
    );

    // Menu buttons.
    let menu = menu_labels(!view.scene_id.is_empty());
    let focused = ctx
        .focus
        .0
        .as_ref()
        .and_then(|f| f.parse::<usize>().ok())
        .map(|i| i.min(menu.len() - 1))
        .unwrap_or(0);
    for (i, (label, action)) in menu.iter().enumerate() {
        let (x, y) = (52.0, 180.0 + i as f32 * 56.0);
        if i == focused {
            focus_marker(ctx.commands, ctx.theme, bg, x - 26.0, y + 8.0);
        }
        button(ctx.commands, ctx.theme,
            bg,
            label,
            x,
            y,
            240.0,
            44.0,
            action.clone(),
            *label == "New game",
        );
    }

    // Endings gallery tiles: ??? until discovered.
    let tiles_y = 180.0 + menu.len() as f32 * 56.0 + 24.0;
    for (i, tile) in view.endings.iter().enumerate() {
        let label = if tile.discovered { tile.class.clone() } else { "???".to_string() };
        let color = if tile.discovered {
            theme::ending_color(&tile.class)
        } else {
            theme::MUTED
        };
        text(ctx.commands, ctx.theme,
            Some(bg),
            &label,
        Face::Serif(theme::FACE_HINT),
            color,
            52.0 + i as f32 * 72.0,
            tiles_y,
            60.0,
            false,
        );
    }

    let hint = if ctx.settings.muted {
        "M unmute · F reduced motion"
    } else {
        "M mute · F reduced motion"
    };
    text(ctx.commands, ctx.theme,
        Some(bg),
        hint,
        Face::Serif(theme::FACE_HINT),
        theme::MUTED,
        52.0,
        crate::DESIGN_H - 40.0,
        0.0,
        false,
    );
}


/// Keyboard/gamepad handling for the title menu.
pub fn keys(
    nav: &crate::input::Nav,
    view: &ViewCache,
    focus: &mut bevy::ecs::system::ResMut<crate::FocusId>,
    writer: &mut MessageWriter<GameAction>,
) {
    let menu = menu_labels(!view.view.scene_id.is_empty());
    let idx = focus
        .0
        .as_ref()
        .and_then(|f| f.parse::<usize>().ok())
        .unwrap_or(0);
    let mut idx = idx.min(menu.len() - 1);
    if nav.vertical_step() != 0 {
        idx = (idx as i32 + nav.vertical_step()).rem_euclid(menu.len() as i32) as usize;
    }
    if nav.confirm {
        let (_, action) = menu[idx].clone();
        writer.write(action);
    }
    let next = idx.to_string();
    if focus.0.as_deref() != Some(next.as_str()) {
        focus.0 = Some(next);
    }
}
