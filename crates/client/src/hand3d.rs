//! The 3D card table, reproducing hand3d.js in Bevy: a fanned hand of
//! card meshes on a lit table, a deck stack off the right frame edge,
//! staggered deal-ins, play arcs, shuffle riffles, and scout pulses.

use bevy::light::NotShadowCaster;
use bevy::prelude::*;

use alexander_core::app::View;

use crate::tween::{Fade, Prop, Tweens};
use crate::{CardTable, FocusId, FOCUS_SCOUT, FOCUS_SHUFFLE, GameAction, TableCamera, ViewCache};
use bevy::window::PrimaryWindow;
pub const CARD_W: f32 = 1.7;
pub const CARD_H: f32 = 2.38;
pub const CARD_T: f32 = 0.04;
/// hand3d's base display scalar.
pub const CARD_SCALE: f32 = 0.96;

const CAM_BASE: Vec3 = Vec3::new(0.0, 2.4, 6.3);
const CAM_TARGET: Vec3 = Vec3::new(0.0, -0.2, 0.0);
const TABLE_COLOR: Color =
    Color::srgb(0x14 as f32 / 255.0, 0x10 as f32 / 255.0, 0x0c as f32 / 255.0);
const EDGE_COLOR: Color =
    Color::srgb(0x2b as f32 / 255.0, 0x24 as f32 / 255.0, 0x1c as f32 / 255.0);
const DECK_POS: Vec3 = Vec3::new(5.5, -1.32, 2.5);
const DECK_ROT: (f32, f32, f32) = (std::f32::consts::FRAC_PI_2 - 0.22, 0.0, 0.3);

/// The fan slot for card i of n (hand3d `fanSlot`): position + z rotation.
pub fn fan_slot(i: usize, n: usize) -> (Vec3, f32) {
    let t = if n == 1 {
        0.0
    } else {
        i as f32 - (n - 1) as f32 / 2.0
    };
    (
        Vec3::new(t * 1.3, -t.abs() * 0.16 - 0.1, 0.6 - t.abs() * 0.1),
        -t * 0.085,
    )
}

/// Dolly distance so the fanned hand stays framed at any aspect
/// (hand3d `fitZ`).
fn fit_z(aspect: f32) -> f32 {
    let fov = 34f32.to_radians();
    let t = (fov * std::f32::consts::PI / 360.0).tan();
    let z_for = |half: f32, pz: f32, py: f32| -> f32 {
        let d = half / (t * aspect);
        let vy = CAM_BASE.y - py;
        pz + (d * d - vy * vy).max(0.25).sqrt()
    };
    CAM_BASE.z.max(z_for(3.5, 0.6, 0.0)).min(14.0)
}

fn pseudo_random(i: usize) -> f32 {
    let x = (i as u32).wrapping_mul(0x9E3779B1).rotate_left(13);
    (x % 1000) as f32 / 1000.0
}

/// One card in the live fan.
#[derive(Component)]
pub struct HandCard {
    pub id: String,
    pub playable: bool,
}

/// A departing card flying out (played or discarded) while fading.
#[derive(Component)]
pub struct FlyAway {
    pub life: f32,
}

/// The visual-only deck stack.
#[derive(Component)]
pub struct DeckCard;

/// The deck stack holder.
#[derive(Component)]
pub struct DeckGroup;

/// One riffle-burst card.
#[derive(Component)]
pub struct RiffleCard {
    pub life: f32,
}

/// The scout's peeking deck card.
#[derive(Component)]
pub struct ScoutCard {
    pub t: f32,
}

/// Hand-side animation requests from dispatched actions.
#[derive(Message)]
pub enum HandFx {
    Riffle,
    Scout,
}

/// Persistent 3D-scene state.
#[derive(Resource, Default)]
pub struct Hand3D {
    pub version_seen: u64,
    pub hovered: Option<String>,
    pending_play: Option<String>,
    deck_seen: i32,
}

/// The table, key light, and deck group.
pub fn setup_scene(
    commands: &mut Commands,
    meshes: &mut Assets<Mesh>,
    materials: &mut Assets<StandardMaterial>,
) {
    // Table plane.
    commands.spawn((
        Mesh3d(meshes.add(Plane3d::default().mesh().size(30.0, 30.0))),
        MeshMaterial3d(materials.add(StandardMaterial {
            base_color: TABLE_COLOR,
            perceptual_roughness: 0.95,
            ..default()
        })),
        Transform::from_xyz(0.0, -1.35, 0.0),
    ));

    // Key light: 0xffe8c0 at (2.5, 4, 3).
    commands.spawn((
        DirectionalLight {
            color: Color::srgb(1.0, 0.91, 0.75),
            illuminance: 0.85 * 20_000.0,
            ..default()
        },
        Transform::from_xyz(2.5, 4.0, 3.0).looking_at(Vec3::ZERO, Vec3::Y),
    ));

    // Deck stack group (visual only; synced by draw-pile count).
    commands.spawn((
        DeckGroup,
        Transform {
            translation: DECK_POS,
            rotation: Quat::from_euler(EulerRot::XYZ, DECK_ROT.0, DECK_ROT.1, DECK_ROT.2),
            scale: Vec3::ONE,
        },
    ));
}

/// One card mesh assembly: thin body + textured front + back plane.
struct CardSpawn {
    body: Handle<Mesh>,
    plane: Handle<Mesh>,
    edge: Handle<StandardMaterial>,
    face: Handle<StandardMaterial>,
    back: Handle<StandardMaterial>,
}

fn card_spawn(
    meshes: &mut Assets<Mesh>,
    materials: &mut Assets<StandardMaterial>,
    face_tex: Handle<Image>,
    back_tex: Handle<Image>,
    fade: bool,
) -> CardSpawn {
    let body = meshes.add(Cuboid::new(CARD_W, CARD_H, CARD_T));
    let plane = meshes.add(Plane3d::default().mesh().size(CARD_W, CARD_H));
    let edge = materials.add(StandardMaterial {
        base_color: EDGE_COLOR,
        perceptual_roughness: 0.9,
        alpha_mode: if fade { AlphaMode::Blend } else { AlphaMode::Opaque },
        ..default()
    });
    let face = materials.add(StandardMaterial {
        base_color_texture: Some(face_tex),
        perceptual_roughness: 0.85,
        alpha_mode: if fade { AlphaMode::Blend } else { AlphaMode::Opaque },
        ..default()
    });
    let back = materials.add(StandardMaterial {
        base_color_texture: Some(back_tex),
        perceptual_roughness: 0.85,
        alpha_mode: if fade { AlphaMode::Blend } else { AlphaMode::Opaque },
        ..default()
    });
    CardSpawn {
        body,
        plane,
        edge,
        face,
        back,
    }
}

fn spawn_card_children(p: &mut ChildSpawnerCommands, s: &CardSpawn) {
    p.spawn((Mesh3d(s.body.clone()), MeshMaterial3d(s.edge.clone()), NotShadowCaster));
    p.spawn((
        Mesh3d(s.plane.clone()),
        MeshMaterial3d(s.face.clone()),
        NotShadowCaster,
        Transform::from_translation(Vec3::new(0.0, 0.0, CARD_T / 2.0 + 0.001)),
    ));
    p.spawn((
        Mesh3d(s.plane.clone()),
        MeshMaterial3d(s.back.clone()),
        NotShadowCaster,
        Transform {
            translation: Vec3::new(0.0, 0.0, -CARD_T / 2.0 - 0.001),
            rotation: Quat::from_rotation_y(std::f32::consts::PI),
            scale: Vec3::ONE,
        },
    ));
}

/// Diffs the view's hand against the live card entities: removed cards
/// fly out, new cards deal in from the deck, survivors re-fan (hand3d
/// `sync` over the readDOM diff).
#[allow(clippy::too_many_arguments)]
pub fn sync_hand(
    mut commands: Commands,
    view: Res<ViewCache>,
    mut hand3d: ResMut<Hand3D>,
    cards: Query<(Entity, &HandCard)>,
    mut tweens: ResMut<Tweens>,
    mut meshes: ResMut<Assets<Mesh>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
    mut painted: ResMut<crate::paint::PaintedCards>,
    mut images: ResMut<Assets<Image>>,
    lib: Res<CardTable>,
    deck: Query<Entity, With<DeckGroup>>,
    deck_cards: Query<Entity, With<DeckCard>>,
    screen: Res<State<crate::Screen>>,
) {
    if view.version == hand3d.version_seen {
        return;
    }
    hand3d.version_seen = view.version;

    // Outside the table screen the fan is empty.
    let hand: Vec<(String, String, i32, bool)> = if screen.get() == &crate::Screen::Table {
        view.view
            .hand
            .iter()
            .map(|c| (c.id.clone(), c.name.clone(), c.cost, c.playable))
            .collect()
    } else {
        Vec::new()
    };

    // Remove meshes whose cards left the hand.
    let mut kept: Vec<Entity> = Vec::new();
    for (e, card) in &cards {
        if hand.iter().any(|(id, _, _, _)| *id == card.id) {
            kept.push(e);
        } else {
            let played = hand3d.pending_play.as_deref() == Some(card.id.as_str());
            fly_away(&mut commands, &mut tweens, e, played);
        }
    }
    hand3d.pending_play = None;

    // Deal new cards in from the deck stack, staggered.
    let mut fresh: Vec<(usize, Entity)> = Vec::new();
    let mut deal_index = 0usize;
    for (i, (id, name, cost, _)) in hand.iter().enumerate() {
        if kept
            .iter()
            .any(|&e| cards.get(e).map(|(_, c)| c.id == *id).unwrap_or(false))
        {
            continue;
        }
        let text = lib.0.get(id).map(|c| c.text.clone()).unwrap_or_default();
        let face_tex = painted.front(id, name, &text, *cost, &mut images);
        let back_tex = painted.back.clone().expect("paint boot");
        let s = card_spawn(&mut meshes, &mut materials, face_tex, back_tex, false);
        let spawn = Transform::from_xyz(4.2, -0.9 + deal_index as f32 * 0.05, DECK_POS.z)
            .with_rotation(Quat::from_euler(EulerRot::XYZ, -0.3, -0.45, 0.1))
            .with_scale(Vec3::splat(CARD_SCALE));
        let entity = commands
            .spawn((
                HandCard {
                    id: id.clone(),
                    playable: hand.iter().any(|(h, _, _, p)| h == id && *p),
                },
                Fade::full(),
                spawn,
            ))
            .with_children(|p| spawn_card_children(p, &s))
            .id();
        fresh.push((i, entity));
        let (slot, rz) = fan_slot(i, hand.len());
        let delay = 0.07 * deal_index as f32;
        tweens.to(entity, Prop::X, slot.x, 0.42, delay);
        tweens.to(entity, Prop::Y, slot.y, 0.42, delay);
        tweens.to(entity, Prop::Z, slot.z, 0.42, delay);
        tweens.to(entity, Prop::RotX, 0.0, 0.42, delay);
        tweens.to(entity, Prop::RotY, 0.0, 0.42, delay);
        tweens.to(entity, Prop::RotZ, rz, 0.42, delay);
        deal_index += 1;
    }

    // Survivors re-fan into their (possibly shifted) slots.
    for (i, (id, _, _, _)) in hand.iter().enumerate() {
        if fresh.iter().any(|(idx, _)| *idx == i) {
            continue;
        }
        if let Some(&e) = kept.iter().find(|&&e| {
            cards.get(e).map(|(_, c)| c.id == *id).unwrap_or(false)
        }) {
            let (slot, rz) = fan_slot(i, hand.len());
            tweens.kill(e);
            tweens.to(e, Prop::X, slot.x, 0.3, 0.0);
            tweens.to(e, Prop::Y, slot.y, 0.3, 0.0);
            tweens.to(e, Prop::Z, slot.z, 0.3, 0.0);
            tweens.to(e, Prop::RotZ, rz, 0.3, 0.0);
        }
    }

    apply_tint(&mut commands, &cards, &hand, None);
    let deck_group = deck.single().ok();
    let old_cards: Vec<Entity> = deck_cards.iter().collect();
    sync_deck(
        &mut commands,
        &mut meshes,
        &mut materials,
        &painted,
        view.view.deck_count,
        &mut hand3d,
        deck_group,
        &old_cards,
    );
}

/// Pending face-color change on a card entity (child 1 = face plane).
#[derive(Component)]
pub struct FaceTint(pub Color);

/// Launches a leaving card: played cards arc toward the scene side and
/// tumble; the rest drop away. Both fade out.
fn fly_away(commands: &mut Commands, tweens: &mut Tweens, e: Entity, played: bool) {
    tweens.kill(e);
    commands
        .entity(e)
        .remove::<HandCard>()
        .insert(FlyAway { life: 0.5 });
    if played {
        tweens.to(e, Prop::X, 2.6, 0.34, 0.0);
        tweens.to(e, Prop::Y, 1.6, 0.34, 0.0);
        tweens.to(e, Prop::Z, 3.2, 0.34, 0.0);
        tweens.to(e, Prop::RotX, -0.9, 0.34, 0.0);
    } else {
        tweens.to(e, Prop::Y, -2.6, 0.34, 0.0);
        tweens.to(e, Prop::Z, 0.0, 0.34, 0.0);
    }
    tweens.to(e, Prop::Opacity, 0.0, 0.3, 0.0);
}

/// Unaffordable cards stay full size but dim to 0.45; a brighter 1.25
/// tint marks the focused card (hand3d `applyTint`; focus grows ~4%).
fn apply_tint(
    commands: &mut Commands,
    cards: &Query<(Entity, &HandCard)>,
    hand: &[(String, String, i32, bool)],
    focused: Option<&str>,
) {
    for (id, playable) in hand.iter().map(|(id, _, _, p)| (id, *p)) {
        let Some((e, _)) = cards.iter().find(|(_, c)| c.id == *id) else {
            continue;
        };
        let tint = if focused == Some(id.as_str()) {
            Color::srgb(1.25, 1.25, 1.25)
        } else if playable {
            Color::WHITE
        } else {
            Color::srgb(0.45, 0.45, 0.45)
        };
        commands.entity(e).insert(FaceTint(tint));
    }
}

/// Matches tweened `Fade` opacity into every card's materials.
pub fn apply_fades(
    mut cards: Query<(&Fade, &Children), Changed<Fade>>,
    mats: Query<&MeshMaterial3d<StandardMaterial>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
) {
    for (fade, children) in &mut cards {
        if fade.opacity >= 0.999 {
            continue;
        }
        for &child in children {
            let Ok(mat) = mats.get(child) else { continue };
            if let Some(mut m) = materials.get_mut(&mat.0) {
                let mut c = m.base_color;
                c.set_alpha(fade.opacity);
                m.base_color = c;
            }
        }
    }
}

/// Applies pending `FaceTint`s to the textured face plane.
pub fn apply_tints(
    mut commands: Commands,
    tinted: Query<(Entity, &FaceTint, &Children)>,
    mats: Query<&MeshMaterial3d<StandardMaterial>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
) {
    for (e, tint, children) in &tinted {
        for child in children.iter().take(3).skip(1) {
            let Ok(mat) = mats.get(child) else { continue };
            if let Some(mut m) = materials.get_mut(&mat.0) {
                m.base_color = tint.0;
            }
        }
        commands.entity(e).remove::<FaceTint>();
    }
}

/// Despawns finished fly-aways.
pub fn despawn_away(
    mut commands: Commands,
    time: Res<Time>,
    mut away: Query<(Entity, &mut FlyAway, &Fade)>,
) {
    for (e, mut life, fade) in &mut away {
        life.life -= time.delta_secs();
        if life.life <= 0.0 || fade.opacity <= 0.01 {
            commands.entity(e).despawn();
        }
    }
}

/// Rebuilds the deck stack when the draw-pile count changes: at most 8
/// back-face cards, half scale, jittered like a loose pile.
#[allow(clippy::too_many_arguments)]
fn sync_deck(
    commands: &mut Commands,
    meshes: &mut Assets<Mesh>,
    materials: &mut Assets<StandardMaterial>,
    painted: &crate::paint::PaintedCards,
    count: i32,
    hand3d: &mut Hand3D,
    group: Option<Entity>,
    old_cards: &[Entity],
) {
    if hand3d.deck_seen == count {
        return;
    }
    hand3d.deck_seen = count;
    let n = count.clamp(0, 8) as usize;
    let Some(group) = group else {
        return;
    };
    for e in old_cards {
        commands.entity(*e).despawn();
    }
    let Some(back) = painted.back.clone() else {
        return;
    };
    let s = card_spawn(meshes, materials, back.clone(), back, false);
    for i in 0..n {
        let jitter_y = (i % 2) as f32 * 0.02;
        let jitter_z = ((i % 3) as f32 - 1.0) * 0.015;
        let rot_y = (pseudo_random(i) - 0.5) * 0.16;
        commands.entity(group).with_children(|p| {
            p.spawn((
                DeckCard,
                NotShadowCaster,
                Transform {
                    translation: Vec3::new(0.0, i as f32 * 0.045 + jitter_y, jitter_z),
                    rotation: Quat::from_rotation_y(rot_y),
                    scale: Vec3::splat(0.5),
                },
            ))
            .with_children(|card| spawn_card_children(card, &s));
        });
    }
}

/// The shuffle riffle: six back-face cards burst from the deck, fly
/// leftward into the visible table, spin, and fade.
#[allow(clippy::too_many_arguments)]
pub fn riffle(
    mut commands: Commands,
    mut reader: MessageReader<HandFx>,
    settings: Res<crate::input::Settings>,
    painted: Res<crate::paint::PaintedCards>,
    mut meshes: ResMut<Assets<Mesh>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
    mut tweens: ResMut<Tweens>,
    deck: Query<Entity, With<DeckGroup>>,
    mut deck_cards: Query<&mut Transform, (With<DeckCard>, Without<DeckGroup>)>,
) {
    let mut want = false;
    for msg in reader.read() {
        if matches!(msg, HandFx::Riffle) {
            want = true;
        }
    }
    if !want || settings.reduced_motion {
        return;
    }
    let Some(back) = painted.back.clone() else {
        return;
    };
    let s = card_spawn(&mut meshes, &mut materials, back.clone(), back, true);
    for i in 0..6 {
        let e = commands
            .spawn((
                RiffleCard { life: 0.45 },
                Fade::full(),
                NotShadowCaster,
                Transform::from_translation(Vec3::new(DECK_POS.x, DECK_POS.y + 0.2, DECK_POS.z))
                    .with_rotation(Quat::from_euler(
                        EulerRot::XYZ,
                        DECK_ROT.0,
                        DECK_ROT.1,
                        DECK_ROT.2,
                    ))
                    .with_scale(Vec3::splat(0.5)),
            ))
            .with_children(|p| spawn_card_children(p, &s))
            .id();
        let r = |k: usize| pseudo_random(i * 7 + k + 1);
        tweens.to(e, Prop::X, 0.5 + r(0) * 3.0, 0.4, 0.0);
        tweens.to(e, Prop::Y, 0.9 + r(1) * 0.8, 0.4, 0.0);
        tweens.to(e, Prop::Z, (r(2) - 0.5) * 2.0, 0.4, 0.0);
        tweens.to(e, Prop::RotY, (r(3) - 0.5) * 2.4, 0.4, 0.0);
        tweens.to(e, Prop::Opacity, 0.0, 0.36, 0.0);
    }
    // Re-jitter the settled stack.
    if deck.single().is_ok() {
        for (i, mut t) in deck_cards.iter_mut().enumerate() {
            t.rotation = Quat::from_rotation_y((pseudo_random(i + 31) - 0.5) * 0.5);
        }
    }
}

/// Advances and despawns riffle cards.
pub fn step_riffle(mut commands: Commands, time: Res<Time>, mut riffle: Query<(Entity, &mut RiffleCard)>) {
    for (e, mut r) in &mut riffle {
        r.life -= time.delta_secs();
        if r.life <= 0.0 {
            commands.entity(e).despawn();
        }
    }
}

/// The scout pulse: a brightened deck card peeks up from the table's
/// right edge, then sinks away.
pub fn scout_pulse(
    mut commands: Commands,
    mut reader: MessageReader<HandFx>,
    settings: Res<crate::input::Settings>,
    painted: Res<crate::paint::PaintedCards>,
    mut meshes: ResMut<Assets<Mesh>>,
    mut materials: ResMut<Assets<StandardMaterial>>,
) {
    let mut want = false;
    for msg in reader.read() {
        if matches!(msg, HandFx::Scout) {
            want = true;
        }
    }
    if !want || settings.reduced_motion {
        return;
    }
    let Some(back) = painted.back.clone() else {
        return;
    };
    let s = card_spawn(&mut meshes, &mut materials, back.clone(), back, false);
    // Brightened like hand3d's cloned back material at scalar 1.5.
    if let Some(mut m) = materials.get_mut(&s.face) {
        m.base_color = Color::srgb(1.5, 1.5, 1.5);
    }
    commands
        .spawn((
            ScoutCard { t: 0.0 },
            Fade::full(),
            NotShadowCaster,
            Transform::from_xyz(2.45, -1.2, 1.9)
                .with_rotation(Quat::from_euler(EulerRot::XYZ, -0.9, 0.3, 0.15))
                .with_scale(Vec3::splat(0.7)),
        ))
        .with_children(|p| spawn_card_children(p, &s));
}

/// Rise-then-sink of the scout's peeking card.
pub fn step_scout(mut commands: Commands, time: Res<Time>, mut scouts: Query<(Entity, &mut ScoutCard, &mut Transform)>) {
    for (e, mut s, mut t) in &mut scouts {
        s.t += time.delta_secs();
        if s.t < 0.22 {
            t.translation.y = -1.2 + ((-0.95) - (-1.2)) * (s.t / 0.22).min(1.0);
        } else if s.t < 0.22 + 0.28 {
            let k = ((s.t - 0.22) / 0.28).min(1.0);
            t.translation.y = -0.95 + ((-1.2) - (-0.95)) * k;
        } else {
            commands.entity(e).despawn();
        }
    }
}

/// Parallax, fitZ framing, and the shared screen shake (hand3d's
/// pointermove camera nudge).
pub fn camera_update(
    mut shake: ResMut<crate::fx::ScreenShake>,
    _nav: Res<crate::input::Nav>,
    windows: Query<&Window, With<PrimaryWindow>>,
    mut camera: Query<(&mut Transform, &Camera), With<TableCamera>>,
    time: Res<Time>,
) {
    let Ok((mut transform, camera)) = camera.single_mut() else {
        return;
    };
    let Ok(window) = windows.single() else {
        return;
    };
    let Some(viewport) = camera.viewport.as_ref() else {
        return;
    };
    let vw = viewport.physical_size.x as f32;
    let vh = viewport.physical_size.y as f32;
    let aspect = vw / vh.max(1.0);

    // Pointer parallax, normalized inside the table viewport.
    let (mut px, mut py) = (0.0f32, 0.0f32);
    if let Some(cursor) = window.cursor_position()
        && cursor.x < vw
        && cursor.y < vh
    {
        px = (cursor.x / vw) * 2.0 - 1.0;
        py = -((cursor.y / vh) * 2.0 - 1.0);
    }
    let mut pos = Vec3::new(px * 0.22, CAM_BASE.y + py * 0.12, fit_z(aspect));
    // Shared screen shake: decaying horizontal wobble.
    if shake.0 > 0.0 {
        pos.x += (shake.0 * 44.0).sin() * 0.02 * shake.0;
        shake.0 = (shake.0 - time.delta_secs()).max(0.0);
    }
    transform.translation = pos;
    transform.look_at(CAM_TARGET, Vec3::Y);
}

/// Ray-cast hover and click on the fan, plus the unaffordable
/// head-shake. Playable and unplayable clicks both dispatch a play
/// request: the engine's refusal surfaces the `Error: …` log line.
#[allow(clippy::too_many_arguments)]
pub fn interact(
    nav: Res<crate::input::Nav>,
    mut hand3d: ResMut<Hand3D>,
    mut writer: MessageWriter<GameAction>,
    mut sfx: MessageWriter<crate::audio::Sfx>,
    camera: Query<(&Camera, &GlobalTransform), With<TableCamera>>,
    cards: Query<(Entity, &HandCard, &Transform)>,
    mut tweens: ResMut<Tweens>,
    screen: Res<State<crate::Screen>>,
) {
    if screen.get() != &crate::Screen::Table || view_is_ending(&cards) {
        return;
    }
    let Ok((camera, cam_transform)) = camera.single() else {
        return;
    };
    let Some(viewport) = camera.viewport.as_ref() else {
        return;
    };
    let vw = viewport.physical_size.x as f32;
    let vh = viewport.physical_size.y as f32;
    let cursor = nav.cursor;
    let in_viewport = cursor.x < vw && cursor.y < vh && cursor.x >= 0.0 && cursor.y >= 0.0;
    let ray = if in_viewport {
        camera.viewport_to_world(cam_transform, cursor).ok()
    } else {
        None
    };

    // Nearest card whose oriented rect the ray crosses.
    let mut hit: Option<(String, bool, f32)> = None;
    if let Some(ray) = ray {
        let dir: Vec3 = ray.direction * 1.0;
        for (_, card, transform) in &cards {
            let normal: Vec3 = transform.forward() * 1.0;
            let to_card = transform.translation - ray.origin;
            let denom = dir.dot(normal);
            if denom.abs() < 1e-5 {
                continue;
            }
            let t = to_card.dot(normal) / denom;
            if t < 0.0 {
                continue;
            }
            let point = ray.origin + dir * t;
            let local = transform.rotation.inverse() * (point - transform.translation);
            if local.x.abs() <= CARD_W / 2.0 * CARD_SCALE && local.y.abs() <= CARD_H / 2.0 * CARD_SCALE
            {
                let better = hit.as_ref().map(|(_, _, ht)| t < *ht).unwrap_or(true);
                if better {
                    hit = Some((card.id.clone(), card.playable, t));
                }
            }
        }
    }

    // Hover lift in and out.
    let hovered_id = hit.as_ref().map(|(id, _, _)| id.clone());
    if hovered_id != hand3d.hovered {
        if let Some(old) = hand3d.hovered.clone()
            && let Some((e, _, _)) = cards.iter().find(|(_, c, _)| c.id == old)
        {
            let idx = cards.iter().position(|(_, c, _)| c.id == old).unwrap_or(0);
            let (slot, _) = fan_slot(idx, cards.iter().count());
            tweens.to(e, Prop::Y, slot.y, 0.2, 0.0);
            tweens.to(e, Prop::Z, slot.z, 0.2, 0.0);
        }
        hand3d.hovered = hovered_id;
        if let Some((id, playable, _)) = &hit
            && *playable
            && let Some((e, _, _)) = cards.iter().find(|(_, c, _)| c.id == *id)
        {
            tweens.to(e, Prop::Y, 0.55, 0.2, 0.0);
            tweens.to(e, Prop::Z, 1.55, 0.2, 0.0);
        }
    }

    // Click: play, or head-shake on an unaffordable card.
    if nav.click.is_some()
        && in_viewport
        && let Some((id, _, _)) = &hit
    {
        hand3d.pending_play = Some(id.clone());
        if !hit.as_ref().map(|(_, p, _)| *p).unwrap_or(false) {
            sfx.write(crate::audio::Sfx("error"));
            if let Some((e, _, t)) = cards.iter().find(|(_, c, _)| c.id == *id) {
                let x0 = t.translation.x;
                tweens.kill(e);
                tweens.to(e, Prop::X, x0 - 0.14, 0.07, 0.0);
                tweens.to(e, Prop::X, x0, 0.07, 0.07);
            }
        }
        writer.write(GameAction::PlayCard(id.clone()));
    }
}

fn view_is_ending(_cards: &Query<(Entity, &HandCard, &Transform)>) -> bool {
    // The ending scene has no hand, so the ray-cast fan is empty anyway.
    false
}

/// Re-applies tint (and nothing else) when the keyboard focus changes.
pub fn focus_tint(
    focus: Res<FocusId>,
    view: Res<ViewCache>,
    mut commands: Commands,
    cards: Query<(Entity, &HandCard)>,
) {
    if !focus.is_changed() {
        return;
    }
    let hand: Vec<(String, String, i32, bool)> = view
        .view
        .hand
        .iter()
        .map(|c| (c.id.clone(), c.name.clone(), c.cost, c.playable))
        .collect();
    if view.view.scene.ending.is_empty() {
        apply_tint(&mut commands, &cards, &hand, focus.0.as_deref());
    }
}


/// The id focusables cycle through on the table screen: hand ids then
/// scout then shuffle.
pub fn focusables(view: &View) -> Vec<String> {
    let mut ids: Vec<String> = view.hand.iter().map(|c| c.id.clone()).collect();
    ids.push(FOCUS_SCOUT.to_string());
    ids.push(FOCUS_SHUFFLE.to_string());
    ids
}
