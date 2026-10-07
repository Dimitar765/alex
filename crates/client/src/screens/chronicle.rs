//! The chronicle screen: ending breakdown, averages, best legacy, and
//! the completed-run table.

use bevy::prelude::*;

use crate::screens::{button, text, Face, ScreenCtx};
use crate::theme;
use crate::GameAction;

pub fn build(ctx: ScreenCtx<'_, '_, '_>, root: Entity) {
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
        "Chronicle",
        Face::Display(theme::FACE_HEADER),
        theme::GOLD,
        48.0,
        40.0,
        0.0,
        false,
    );
    text(ctx.commands, ctx.theme,
        Some(bg),
        "Esc — back to the table",
        Face::Serif(theme::FACE_HINT),
        theme::MUTED,
        52.0,
        92.0,
        0.0,
        false,
    );

    if ctx.chronicle.runs.is_empty() {
        text(ctx.commands, ctx.theme,
            Some(bg),
            "No campaigns recorded yet.",
            Face::Serif(theme::FACE_BODY),
            theme::MUTED,
            52.0,
            140.0,
            0.0,
            false,
        );
        return;
    }

    // Ending breakdown and averages.
    let mut x = 52.0;
    for e in &ctx.chronicle.endings {
        let label = format!("{} ×{}", e.class, e.count);
        text(ctx.commands, ctx.theme,
            Some(bg),
            &label,
            Face::Serif(theme::FACE_BODY),
            theme::ending_color(&e.class),
            x,
            140.0,
            0.0,
            false,
        );
        x += 130.0;
    }
    text(ctx.commands, ctx.theme,
        Some(bg),
        &format!(
            "avg turns {:.1} · avg cards {:.1}",
            ctx.chronicle.avg_turns, ctx.chronicle.avg_cards
        ),
        Face::Serif(theme::FACE_SMALL),
        theme::MUTED,
        52.0,
        168.0,
        0.0,
        false,
    );
    if let Some(best) = &ctx.chronicle.best {
        text(ctx.commands, ctx.theme,
            Some(bg),
            &format!("best legacy {}", best.stats.legacy),
            Face::Serif(theme::FACE_SMALL),
            theme::GOLD,
            52.0,
            190.0,
            0.0,
            false,
        );
    }

    // Run table, newest first.
    let mut y = 232.0;
    text(ctx.commands, ctx.theme,
        Some(bg),
        "ENDING     LEGACY  TURNS  CARDS  DATE",
        Face::Serif(theme::FACE_SMALL),
        theme::MUTED,
        52.0,
        y,
        0.0,
        false,
    );
    y += 26.0;
    for (i, run) in ctx.chronicle.runs.iter().enumerate() {
        if y > crate::DESIGN_H - 60.0 {
            let more = format!("… and {} earlier runs", ctx.chronicle.runs.len() - i);
            text(ctx.commands, ctx.theme,
                Some(bg),
                &more,
                Face::Serif(theme::FACE_SMALL),
                theme::MUTED,
                52.0,
                y,
                0.0,
                false,
            );
            break;
        }
        let date = run
            .date
            .to_string()
            .chars()
            .take(10)
            .collect::<String>();
        let line = format!(
            "{:<10} {:>5}   {:>5}  {:>5}  {}",
            run.ending, run.stats.legacy, run.turns, run.cards_played, date
        );
        text(ctx.commands, ctx.theme,
            Some(bg),
            &line,
            Face::Serif(theme::FACE_BODY),
            theme::INK,
            52.0,
            y,
            0.0,
            false,
        );
        y += 26.0;
    }
    let _ = &ctx;
    button(ctx.commands, ctx.theme,
        bg,
        "Back",
        52.0,
        crate::DESIGN_H - 52.0,
        140.0,
        36.0,
        GameAction::GoTitle,
        false,
    );
}
