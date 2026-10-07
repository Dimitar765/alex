//! UI screens: shared builders, the rebuild-on-change driver, button
//! clicks, and keyboard/gamepad routing. Screens are rebuilt like the Go
//! web client's server-rendered swaps — cheap and always consistent.

use bevy::prelude::*;

use crate::input::Nav;
use crate::theme::{self, Theme};
use crate::{GameAction, Screen, ViewCache};

pub mod chronicle;
pub mod table;
pub mod title;

/// The root all screen content hangs from; rebuilt when the view
/// version, state, or settings change.
#[derive(Component)]
pub struct ScreenRoot;

/// Rebuild bookkeeping.
#[derive(Resource)]
pub struct UiBuild {
    pub state: Option<Screen>,
    pub model_version: u64,
    pub settings_version: u64,
    built_state: Option<Screen>,
    built_version: u64,
    built_settings: u64,
}

impl Default for UiBuild {
    fn default() -> Self {
        UiBuild {
            state: None,
            model_version: 0,
            settings_version: 0,
            built_state: None,
            built_version: u64::MAX,
            built_settings: 0,
        }
    }
}

/// A clickable button bound to a game action.
#[derive(Component, Clone)]
pub struct ActionButton(pub GameAction);

/// Gated choice lines render muted; clicks still dispatch so the engine
/// refusal surfaces.
#[derive(Component)]
pub struct DisabledChoice;

pub fn spawn_screen_root(mut commands: Commands) {
    commands.spawn((
        ScreenRoot,
        Node {
            width: Val::Percent(100.0),
            height: Val::Percent(100.0),
            ..default()
        },
    ));
}

/// Rebuilds the active screen when anything it draws changed.
#[allow(clippy::too_many_arguments)]
pub fn update_ui(
    mut commands: Commands,
    root: Query<Entity, With<ScreenRoot>>,
    state: Res<State<Screen>>,
    view: Res<ViewCache>,
    settings: Res<crate::input::Settings>,
    mut build: ResMut<UiBuild>,
    theme: Res<Theme>,
    painted: Res<crate::paint::PaintedCards>,
    _images: Res<Assets<Image>>,
    mut anchors: ResMut<crate::fx::Anchors>,
    focus: Res<crate::FocusId>,
    model: Res<crate::GameModel>,
    children: Query<&Children>,
) {
    let changed = build.built_state != build.state
        || build.built_version != build.model_version
        || build.built_settings != build.settings_version;
    if !changed {
        return;
    }
    let Ok(root) = root.single() else {
        return;
    };
    // Tear down the previous screen.
    if let Ok(kids) = children.get(root) {
        for &kid in kids {
            commands.entity(kid).despawn();
        }
    }
    build.built_state = build.state;
    build.built_version = build.model_version;
    build.built_settings = build.settings_version;

    let ctx = ScreenCtx {
        commands: &mut commands,
        theme: &theme,
        view: &view.view,
        settings: &settings,
        painted: &painted,
        anchors: &mut anchors,
        focus: &focus,
        chronicle: model.0.stats_page(),
    };
    match state.get() {
        Screen::Loading => {}
        Screen::Title => title::build(ctx, root),
        Screen::Table => table::build(ctx, root),
        Screen::Chronicle => chronicle::build(ctx, root),
    }
}

/// Everything a screen builder needs.
pub struct ScreenCtx<'w, 's, 'a> {
    pub commands: &'a mut Commands<'w, 's>,
    pub theme: &'a Theme,
    pub view: &'a crate::View,
    pub settings: &'a crate::input::Settings,
    pub painted: &'a crate::paint::PaintedCards,
    pub anchors: &'a mut crate::fx::Anchors,
    pub focus: &'a crate::FocusId,
    pub chronicle: alexander_core::app::Chronicle,
}

/// A text face selection: which typeface at which size.
#[derive(Clone, Copy)]
pub enum Face {
    Serif(f32),
    Italic(f32),
    Display(f32),
}

/// Absolute-positioned text node; `w > 0` wraps at that width.
#[allow(clippy::too_many_arguments)]
pub fn text(
    commands: &mut Commands,
    theme: &Theme,
    parent: Option<Entity>,
    text: &str,
    face: Face,
    color: Color,
    x: f32,
    y: f32,
    w: f32,
    wrap: bool,
) -> Entity {
    let mut node = Node {
        position_type: PositionType::Absolute,
        left: Val::Px(x),
        top: Val::Px(y),
        ..default()
    };
    if w > 0.0 {
        node.width = Val::Px(w);
    }
    let font = match face {
        Face::Serif(size) => theme.face(size),
        Face::Italic(size) => theme.italic(size),
        Face::Display(size) => theme.display_face(size),
    };
    let layout = TextLayout {
        linebreak: if wrap {
            bevy::text::LineBreak::WordBoundary
        } else {
            bevy::text::LineBreak::NoWrap
        },
        ..default()
    };
    let e = commands
        .spawn((node, Text(text.to_string()), font, TextColor(color), layout))
        .id();
    if let Some(p) = parent {
        commands.entity(p).add_children(&[e]);
    }
    e
}

/// A panel: dark plate with a hairline border.
pub fn panel(commands: &mut Commands, x: f32, y: f32, w: f32, h: f32, border: Color) -> Entity {
    commands
        .spawn((
            Node {
                position_type: PositionType::Absolute,
                left: Val::Px(x),
                top: Val::Px(y),
                width: Val::Px(w),
                height: Val::Px(h),
                ..default()
            },
            BackgroundColor(theme::PANEL),
            BorderColor::all(border),
            crate::fx::ScenePanelTag,
        ))
        .id()
}

/// A labeled button; `primary` gets the gold border.
#[allow(clippy::too_many_arguments)]
pub fn button(
    commands: &mut Commands,
    theme: &Theme,
    parent: Entity,
    label: &str,
    x: f32,
    y: f32,
    w: f32,
    h: f32,
    action: GameAction,
    primary: bool,
) -> Entity {
    let border = if primary { theme::GOLD } else { theme::LINE };
    let font = theme.face(theme::FACE_BODY);
    let e = commands
        .spawn((
            Node {
                position_type: PositionType::Absolute,
                left: Val::Px(x),
                top: Val::Px(y),
                width: Val::Px(w),
                height: Val::Px(h),
                ..default()
            },
            BackgroundColor(theme::CARD_FACE),
            BorderColor::all(border),
            Button,
            Interaction::default(),
            ActionButton(action),
        ))
        .with_children(|p| {
            p.spawn((
                Node {
                    width: Val::Percent(100.0),
                    height: Val::Percent(100.0),
                    align_items: AlignItems::Center,
                    justify_content: JustifyContent::Center,
                    ..default()
                },
                Text(label.to_string()),
                font,
                TextColor(theme::INK),
            ));
        })
        .id();
    commands.entity(parent).add_children(&[e]);
    e
}

/// The focus-ring "▸" hint.
pub fn focus_marker(commands: &mut Commands, theme: &Theme, parent: Entity, x: f32, y: f32) {
    let e = commands
        .spawn((
            Node {
                position_type: PositionType::Absolute,
                left: Val::Px(x),
                top: Val::Px(y),
                ..default()
            },
            Text("▸".to_string()),
            theme.face(theme::FACE_BODY),
            TextColor(theme::GOLD),
        ))
        .id();
    commands.entity(parent).add_children(&[e]);
}

/// Fires the action on the press edge of a button interaction.
pub fn button_clicks(
    interactions: Query<(Entity, &Interaction, &ActionButton), Changed<Interaction>>,
    mut prev: Local<std::collections::HashMap<Entity, bool>>,
    mut writer: MessageWriter<GameAction>,
) {
    for (entity, interaction, button) in &interactions {
        let pressed = matches!(interaction, Interaction::Pressed);
        let was = prev.get(&entity).copied().unwrap_or(false);
        if pressed && !was {
            writer.write(button.0.clone());
        }
        prev.insert(entity, pressed);
    }
}

/// Keyboard/gamepad routing per screen.
pub fn screen_keys(
    nav: Res<Nav>,
    state: Res<State<Screen>>,
    view: Res<ViewCache>,
    mut writer: MessageWriter<GameAction>,
    mut focus: ResMut<crate::FocusId>,
) {
    match state.get() {
        Screen::Title => title::keys(&nav, &view, &mut focus, &mut writer),
        Screen::Table => table::keys(&nav, &view, &mut focus, &mut writer),
        Screen::Chronicle => {
            if nav.back {
                writer.write(GameAction::GoTitle);
            }
        }
        Screen::Loading => {}
    }
}
