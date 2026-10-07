//! Alexander — a card adventure: the native 3D client. One window: a
//! fanned 3D hand on a lit table (left) and the narrative panel (right),
//! over the shared Rust core's view-model.

mod audio;
mod fx;
mod hand3d;
mod input;
mod paint;
mod screens;
mod theme;
mod tween;

use alexander_core::app::{Effect, EffectKind, Model, View};
use alexander_core::content;
use bevy::prelude::*;

/// The design window size.
pub const DESIGN_W: f32 = 1280.0;
pub const DESIGN_H: f32 = 800.0;
/// The left fraction of the window hosting the 3D table.
pub const TABLE_FRAC: f32 = 570.0 / 1280.0;

/// A request to mutate the game or navigate. UI interactions and key
/// handlers emit these; the `dispatch` system is the only writer to the
/// model.
#[derive(Message, Clone)]
pub enum GameAction {
    NewGame,
    Choose(usize),
    PlayCard(String),
    Scout,
    Shuffle,
    GoTitle,
    GoTable,
    GoChronicle,
}

/// The shared-core model, owned by the app.
#[derive(Resource)]
pub struct GameModel(pub Model);

/// The current projected view plus a version counter; every action —
/// success or refusal — bumps the version, which drives the UI rebuild
/// and the 3D hand diff.
#[derive(Resource)]
pub struct ViewCache {
    pub view: View,
    pub version: u64,
}

/// Keyboard/gamepad focus on the table screen: a hand-card id, or the
/// scout/shuffle buttons.
#[derive(Resource, Default)]
pub struct FocusId(pub Option<String>);

pub const FOCUS_SCOUT: &str = "\0scout";
pub const FOCUS_SHUFFLE: &str = "\0shuffle";

/// The app screens. Loading exists to build fonts/textures on frame one.
#[derive(States, Clone, Copy, PartialEq, Eq, Debug, Hash, Default)]
pub enum Screen {
    #[default]
    Loading,
    Title,
    Table,
    Chronicle,
}

/// Marker: the 3D table camera (left viewport).
#[derive(Component)]
pub struct TableCamera;

/// Marker: the UI camera.
#[derive(Component)]
pub struct UiCamera;

/// Marker: the FX overlay camera (particles, floats, ghosts, vignette).
#[derive(Component)]
pub struct FxCamera;

/// The saves root (`<config>/Alexander`), or None for memory-only play.
pub fn saves_root() -> Option<std::path::PathBuf> {
    alexander_core::app::user_saves_dir()
        .and_then(|p| p.parent().map(|p| p.to_path_buf()))
}

/// The library card table, for painting faces by id.
#[derive(Resource)]
pub struct CardTable(pub std::collections::HashMap<String, content::Card>);


/// Ordered execution phases for the per-frame systems.
#[derive(SystemSet, Clone, Copy, PartialEq, Eq, Hash, Debug)]
pub enum Phase {
    Input,
    Dispatch,
    Hand,
    Fx,
    Audio,
}
fn main() {
    // Corrupt saves read as empty; the model never fails to construct.
    let saves = saves_root();
    let lib = content::load_embedded();
    let cards: std::collections::HashMap<String, content::Card> = lib
        .cards
        .iter()
        .map(|(k, v)| (k.clone(), v.clone()))
        .collect();
    let model = Model::new(lib, saves);
    let settings = input::Settings::load();

    App::new()
        .add_plugins(DefaultPlugins.set(WindowPlugin {
            primary_window: Some(Window {
                title: "Alexander — a card adventure".into(),
                resolution: bevy::window::WindowResolution::new(DESIGN_W as u32, DESIGN_H as u32),
                present_mode: bevy::window::PresentMode::AutoVsync,
                ..default()
            }),
            ..default()
        }))
        .init_state::<Screen>()
        .add_message::<GameAction>()
        .add_message::<fx::FxEvent>()
        .add_message::<audio::Sfx>()
        .add_message::<hand3d::HandFx>()
        .insert_resource(GameModel(model))
        .insert_resource(ViewCache {
            view: View::default(),
            version: 0,
        })
        .insert_resource(CardTable(cards))
        .insert_resource(input::Nav::default())
        .insert_resource(settings)
        .insert_resource(tween::Tweens::new(settings.reduced_motion))
        .insert_resource(hand3d::Hand3D::default())
        .insert_resource(fx::ScreenShake::default())
        .insert_resource(FocusId::default())
        .insert_resource(screens::UiBuild::default())
        .insert_resource(fx::Anchors::default())
        .insert_resource(fx::HandScreenPos::default())
        .add_systems(
            Startup,
            (setup_world, init_view, fx::boot, screens::spawn_screen_root),
        )
        .add_systems(OnEnter(Screen::Loading), load_assets)
        .add_systems(
            Update,
            (input::update_nav, apply_settings_toggles)
                .chain()
                .in_set(Phase::Input),
        )
        .add_systems(
            Update,
            (
                dispatch,
                screens::button_clicks,
                screens::update_ui,
                screens::screen_keys,
            )
                .chain()
                .in_set(Phase::Dispatch),
        )
        .add_systems(
            Update,
            (
                tween::step_tweens_system,
                hand3d::sync_hand,
                hand3d::interact,
                hand3d::camera_update,
                hand3d::riffle,
                hand3d::scout_pulse,
                hand3d::step_riffle,
                hand3d::step_scout,
                hand3d::apply_fades,
                hand3d::apply_tints,
                hand3d::focus_tint,
                hand3d::despawn_away,
            )
                .chain()
                .in_set(Phase::Hand),
        )
        .add_systems(
            Update,
            (fx::track_hand_screen, fx::consume_fx, fx::step_fx)
                .chain()
                .in_set(Phase::Fx),
        )
        .add_systems(
            Update,
            (audio::play_cues, audio::despawn_finished)
                .chain()
                .in_set(Phase::Audio),
        )
        .configure_sets(
            Update,
            (
                Phase::Input,
                Phase::Dispatch,
                Phase::Hand,
                Phase::Fx,
                Phase::Audio,
            )
                .chain(),
        )
        .run();
}

/// Cameras, lights, and the table.
fn setup_world(
    mut commands: Commands,
    mut meshes: ResMut<Assets<Mesh>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
) {
    // 3D table camera: perspective 34° over the left table region. SDR on
    // purpose: the HDR pipeline renders black on NVIDIA+Vulkan (Bevy 0.19),
    // so no bloom / filmic tonemapping until that's fixed upstream.
    let table_w = (DESIGN_W * TABLE_FRAC) as u32;
    let table_h = DESIGN_H as u32;
    commands.spawn((
        TableCamera,
        Camera3d::default(),
        Camera {
            order: 0,
            viewport: Some(bevy::camera::Viewport {
                physical_position: UVec2::new(0, 0),
                physical_size: UVec2::new(table_w, table_h),
                ..default()
            }),
            ..default()
        },
        Projection::Perspective(PerspectiveProjection {
            fov: 34f32.to_radians(),
            ..default()
        }),
        AmbientLight {
            color: Color::srgb(1.0, 0.949, 0.867), // 0xfff2dd
            brightness: 0.75 * 20_000.0,
            ..default()
        },
        Transform::from_xyz(0.0, 2.4, 6.3).looking_at(Vec3::new(0.0, -0.2, 0.0), Vec3::Y),
    ));

    // UI camera: full window, transparent over the 3D render.
    commands.spawn((
        UiCamera,
        Camera2d,
        Camera {
            order: 1,
            clear_color: ClearColorConfig::None,
            ..default()
        },
        IsDefaultUiCamera,
    ));

    // FX overlay camera: 2D sprites above the UI.
    commands.spawn((
        FxCamera,
        Camera2d,
        Camera {
            order: 2,
            clear_color: ClearColorConfig::None,
            ..default()
        },
        bevy::camera::visibility::RenderLayers::layer(2),
    ));

    hand3d::setup_scene(&mut commands, &mut meshes, &mut materials);
}

/// Projects the model once so the title screen knows about Continue.
fn init_view(model: Res<GameModel>, mut view: ResMut<ViewCache>) {
    view.view = model.0.view();
    view.version = 1;
}

/// Loading state: build the embedded fonts and painted card backs, then
/// enter the title screen.
fn load_assets(
    mut commands: Commands,
    mut fonts: ResMut<Assets<Font>>,
    mut images: ResMut<Assets<Image>>,
    mut next_state: ResMut<NextState<Screen>>,
) {
    commands.insert_resource(theme::Theme::load(&mut fonts));
    let mut painted = paint::PaintedCards::default();
    painted.boot(&mut images);
    commands.insert_resource(painted);
    commands.insert_resource(audio::CueBank::build());
    next_state.set(Screen::Title);
}

/// The M/F/X toggles, persisted to settings.json.
fn apply_settings_toggles(
    nav: Res<input::Nav>,
    mut settings: ResMut<input::Settings>,
    mut tweens: ResMut<tween::Tweens>,
    mut build: ResMut<screens::UiBuild>,
) {
    if nav.toggle_mute {
        settings.muted = !settings.muted;
        settings.save();
        build.settings_version += 1;
    }
    if nav.toggle_motion {
        settings.reduced_motion = !settings.reduced_motion;
        tweens.reduced_motion = settings.reduced_motion;
        settings.save();
        build.settings_version += 1;
    }
    if nav.toggle_fx {
        settings.fx_off = !settings.fx_off;
        settings.save();
        build.settings_version += 1;
    }
}

/// The single writer to the model: applies actions, refreshes the view
/// cache (success or refusal projection), emits FX messages and audio
/// cues, and routes navigation.
#[allow(clippy::too_many_arguments)]
fn dispatch(
    mut reader: MessageReader<GameAction>,
    mut model: ResMut<GameModel>,
    mut view: ResMut<ViewCache>,
    mut fx_writer: MessageWriter<fx::FxEvent>,
    mut sfx: MessageWriter<audio::Sfx>,
    mut hand_fx: MessageWriter<hand3d::HandFx>,
    mut next_state: ResMut<NextState<Screen>>,
    mut focus: ResMut<FocusId>,
) {
    for action in reader.read() {
        match action.clone() {
            GameAction::NewGame => {
                let r = model.0.new_game();
                view.view = r.view;
                view.version += 1;
                focus.0 = None;
                next_state.set(Screen::Table);
            }
            GameAction::Choose(i) => {
                let r = model.0.choose(i);
                finish_action(r, "choice", &mut view, &mut fx_writer, &mut sfx, &mut focus);
            }
            GameAction::PlayCard(id) => {
                let r = model.0.play_card(&id);
                finish_action(r, "card", &mut view, &mut fx_writer, &mut sfx, &mut focus);
            }
            GameAction::Scout => {
                let r = model.0.scout();
                sfx.write(audio::Sfx("scout")); // Go plays scout cues unconditionally
                view.view = r.view;
                view.version += 1;
                if r.err.is_none() {
                    hand_fx.write(hand3d::HandFx::Scout);
                }
                for e in &r.effects {
                    fx_writer.write(fx::FxEvent(e.clone()));
                }
            }
            GameAction::Shuffle => {
                let r = model.0.shuffle();
                sfx.write(audio::Sfx("shuffle"));
                view.view = r.view;
                view.version += 1;
                if r.err.is_none() {
                    hand_fx.write(hand3d::HandFx::Riffle);
                }
                for e in &r.effects {
                    fx_writer.write(fx::FxEvent(e.clone()));
                }
            }
            GameAction::GoTitle => {
                focus.0 = None;
                next_state.set(Screen::Title);
            }
            GameAction::GoTable => {
                focus.0 = None;
                next_state.set(Screen::Table);
            }
            GameAction::GoChronicle => {
                focus.0 = None;
                next_state.set(Screen::Chronicle);
            }
        }
    }
}

/// Shared post-action handling: view refresh, FX messages, audio cues.
#[allow(clippy::too_many_arguments)]
fn finish_action(
    r: alexander_core::app::Result_,
    cue_name: &'static str,
    view: &mut ViewCache,
    fx_writer: &mut MessageWriter<fx::FxEvent>,
    sfx: &mut MessageWriter<audio::Sfx>,
    focus: &mut FocusId,
) {
    if r.err.is_some() {
        sfx.write(audio::Sfx("error"));
    } else {
        sfx.write(audio::Sfx(cue_name));
    }
    view.view = r.view;
    view.version += 1;
    for e in &r.effects {
        if e.kind == EffectKind::Ending {
            sfx.write(audio::Sfx(if e.text == "defeat" || e.text == "death" {
                "defeat"
            } else {
                "victory"
            }));
        }
        fx_writer.write(fx::FxEvent(e.clone()));
    }
    focus.0 = None;
}

/// Keeps the Effect import exercised for the FX payload type.
#[allow(dead_code)]
fn _type_check(e: &Effect) {
    let _ = e.clone();
}
