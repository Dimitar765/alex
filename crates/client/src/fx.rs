//! Presentation effects, ported from the web fx.js canvas layer and the
//! desktop anim.go: a hand-rolled particle layer, stat delta floats,
//! scene micro-shakes, ghost cards, ending ambience, and the vignette.
//! Everything is gated on the FX toggle and reduced motion.

use bevy::prelude::*;

use alexander_core::app::EffectKind;

use crate::theme;
use bevy::window::PrimaryWindow;
use crate::ViewCache;
/// One committed action's effect, delivered for animation.
#[derive(Message)]
pub struct FxEvent(pub alexander_core::app::Effect);

/// Remaining screen-shake seconds; the camera and scene panel consume it.
#[derive(Resource, Default)]
pub struct ScreenShake(pub f32);

/// The generated radial vignette texture, built at startup.
#[derive(Resource, Default)]
pub struct VignetteTex(pub Option<Handle<Image>>);

/// Builds the radial vignette texture once.
pub fn boot(mut commands: Commands, mut images: ResMut<Assets<Image>>) {
    const S: u32 = 512;
    let mut data = Vec::with_capacity((S * S * 4) as usize);
    let c = S as f32 / 2.0;
    for y in 0..S {
        for x in 0..S {
            let d = ((x as f32 - c).hypot(y as f32 - c) / c).clamp(0.0, 1.0);
            // Smoothstep from transparent center to opaque edge.
            let a = ((d - 0.35) / 0.65).clamp(0.0, 1.0);
            let a = a * a * (3.0 - 2.0 * a);
            data.extend_from_slice(&[255, 255, 255, (a * 255.0) as u8]);
        }
    }
    let image = Image::new(
        bevy::render::render_resource::Extent3d {
            width: S,
            height: S,
            depth_or_array_layers: 1,
        },
        bevy::render::render_resource::TextureDimension::D2,
        data,
        bevy::render::render_resource::TextureFormat::Rgba8UnormSrgb,
        default(),
    );
    commands.insert_resource(VignetteTex(Some(images.add(image))));
}

/// Design-space anchors the FX layer floats to, recorded by the screen
/// builder (window top-left origin).
#[derive(Resource, Default)]
pub struct Anchors {
    pub stats: Vec2,
    pub threat: Vec2,
    pub scene: Vec2,
    pub scene_size: Vec2,
}

/// Tracked screen positions of the live 3D hand cards (top-left origin).
#[derive(Resource, Default)]
pub struct HandScreenPos(pub Vec<(String, Vec2)>);

// --- particles -------------------------------------------------------------

#[derive(Component)]
pub struct Particle {
    vel: Vec2,
    age: f32,
    ttl: f32,
    size: f32,
}

#[derive(Component)]
pub struct FloatText {
    age: f32,
}

#[derive(Component)]
pub struct GhostCard {
    vel: Vec2,
    spin: f32,
    age: f32,
}

#[derive(Component)]
pub struct TrailMote {
    delay: f32,
    from: Vec2,
    to: Vec2,
    age: f32,
}

#[derive(Component)]
pub struct Ambience {
    t: f32,
    wave: usize,
    embers: bool,
}

#[derive(Component)]
pub struct Vignette {
    t: f32,
}

/// A UI image whose alpha animates (ghost cards fade out, vignettes in).
#[derive(Component)]
pub struct FadingImage {
    base: Color,
    dur: f32,
    fade_out: bool,
    t: f32,
}

/// Marker for the scene panel, shaken by the shake timer.
#[derive(Component)]
pub struct ScenePanelTag;

fn rand_spread(spread: f32) -> f32 {
    (rand_pseudo() - 0.5) * 2.0 * spread
}

fn rand_pseudo() -> f32 {
    use std::cell::Cell;
    thread_local!(static RNG: Cell<u64> = const { Cell::new(0x853C49E6748FEA9B) });
    RNG.with(|r| {
        let mut x = r.get();
        x ^= x << 13;
        x ^= x >> 7;
        x ^= x << 17;
        r.set(x);
        (x >> 11) as f32 / (1u64 << 53) as f32
    })
}

/// Converts window (top-left) design coordinates into the FX camera's
/// world space (center origin, y up).
fn to_world(p: Vec2) -> Vec2 {
    Vec2::new(p.x - crate::DESIGN_W / 2.0, crate::DESIGN_H / 2.0 - p.y)
}

fn spawn_particle(
    commands: &mut Commands,
    at: Vec2,
    vx: f32,
    vy: f32,
    ttl: f32,
    size: f32,
    color: Color,
) {
    commands.spawn((
        Sprite {
            color,
            custom_size: Some(Vec2::splat(size)),
            ..default()
        },
        Transform::from_translation(to_world(at).extend(0.0)),
        bevy::camera::visibility::RenderLayers::layer(2),
        Particle {
            vel: Vec2::new(vx, vy),
            age: 0.0,
            ttl,
            size,
        },
    ));
}

/// fx.js dust: 9 slow-rising motes scattered over a rect.
fn dust(commands: &mut Commands, at: Vec2, size: Vec2, n: usize) {
    for _ in 0..n {
        let p = Vec2::new(
            at.x + rand_pseudo() * size.x,
            at.y + rand_pseudo() * size.y * 0.5,
        );
        spawn_particle(
            commands,
            p,
            rand_spread(0.35) * 60.0,
            -0.25 * 60.0,
            0.7 + rand_pseudo() * 0.5,
            1.3 + rand_pseudo() * 1.2,
            theme::DUST,
        );
    }
}

/// fx.js goldRise: 7 gold motes rising from a rect's bottom.
fn gold_rise(commands: &mut Commands, at: Vec2, size: Vec2, n: usize) {
    for _ in 0..n {
        let p = Vec2::new(at.x + rand_pseudo() * size.x, at.y + size.y);
        spawn_particle(
            commands,
            p,
            rand_spread(0.12) * 60.0,
            -0.35 * 60.0,
            2.2 + rand_pseudo() * 1.4,
            1.5 + rand_pseudo() * 1.6,
            theme::GOLD,
        );
    }
}

/// fx.js embers: 7 falling embers from a rect's top.
fn embers(commands: &mut Commands, at: Vec2, size: Vec2, n: usize) {
    for _ in 0..n {
        let p = Vec2::new(at.x + rand_pseudo() * size.x, at.y);
        spawn_particle(
            commands,
            p,
            rand_spread(0.1) * 60.0,
            0.3 * 60.0,
            2.6 + rand_pseudo() * 1.6,
            1.3 + rand_pseudo() * 1.4,
            theme::EMBER,
        );
    }
}

/// fx.js trail: 6 gold motes flying card -> scene, staggered 28 ms.
fn trail(commands: &mut Commands, from: Vec2, to: Vec2) {
    for i in 0..6 {
        let jitter = Vec2::new(rand_spread(18.0), rand_spread(10.0));
        let start = Vec2::new(from.x + jitter.x, from.y + 10.0 + jitter.y);
        commands.spawn((
            Sprite {
                color: theme::GOLD,
                custom_size: Some(Vec2::splat(1.8)),
                ..default()
            },
            Transform::from_translation(to_world(start).extend(0.0)),
            bevy::camera::visibility::RenderLayers::layer(2),
            TrailMote {
                delay: i as f32 * 0.028,
                from: start,
                to,
                age: 0.0,
            },
        ));
    }
}

/// The desktop ghost card: a card that left the hand flies toward the
/// play side, spinning, fading.
fn ghost_card(
    commands: &mut Commands,
    at: Vec2,
    face: Handle<Image>,
) {
    commands.spawn((
        ImageNode {
            image: face,
            ..default()
        },
        Node {
            position_type: PositionType::Absolute,
            left: Val::Px(at.x - 75.0),
            top: Val::Px(at.y - 105.0),
            width: Val::Px(150.0),
            height: Val::Px(210.0),
            ..default()
        },
        ZIndex(100),
        GhostCard {
            vel: Vec2::new(420.0, -260.0),
            spin: -2.2,
            age: 0.0,
        },
        FadingImage {
            base: Color::WHITE,
            dur: 0.8,
            fade_out: true,
            t: 0.0,
        },
    ));
}

/// A rising "+N Stat" / "−N Stat" float.
fn float_text(
    commands: &mut Commands,
    at: Vec2,
    text: String,
    color: Color,
    theme: &theme::Theme,
) {
    commands.spawn((
        Text2d(text),
        TextLayout {
            linebreak: bevy::text::LineBreak::WordBoundary,
            justify: bevy::text::Justify::Center,
        },
        theme.face(theme::FACE_BODY),
        TextColor(color),
        Transform::from_translation(to_world(at).extend(1.0)),
        bevy::camera::visibility::RenderLayers::layer(2),
        FloatText { age: 0.0 },
    ));
}

/// Turns committed-action effects into particles, floats, ghosts, and
/// shakes (desktop `consumeFX` + web afterSwap wiring).
#[allow(clippy::too_many_arguments)]
pub fn consume_fx(
    mut reader: MessageReader<FxEvent>,
    mut commands: Commands,
    settings: Res<crate::input::Settings>,
    anchors: Res<Anchors>,
    hand_pos: Res<HandScreenPos>,
    view: Res<ViewCache>,
    painted: Res<crate::paint::PaintedCards>,
    mut shake: ResMut<ScreenShake>,
    tex: Res<VignetteTex>,
    theme: Res<theme::Theme>,
) {
    let enabled = crate::input::effects_enabled(&settings);
    for event in reader.read() {
        let e = &event.0;
        match e.kind {
            EffectKind::Stat => {
                if !enabled {
                    continue;
                }
                let (text, color) = if e.delta < 0 {
                    (format!("\u{2212}{} {}", -e.delta, label(&e.stat)), theme::END_DEFEAT)
                } else {
                    (format!("+{} {}", e.delta, label(&e.stat)), theme::GOLD)
                };
                float_text(&mut commands, anchors.stats + Vec2::new(60.0, -10.0), text, color, &theme);
            }
            EffectKind::Threat => {
                if !enabled {
                    continue;
                }
                let color = if e.delta < 0 {
                    theme::GOLD
                } else if view.view.threat >= alexander_core::game::THREAT_AMBUSH {
                    theme::THREAT_CRIT
                } else {
                    theme::THREAT_WARN
                };
                float_text(
                    &mut commands,
                    anchors.threat + Vec2::new(240.0, 0.0),
                    format!("{}{:+} Threat", if e.delta < 0 { "\u{2212}" } else { "" }, e.delta.abs()),
                    color,
                    &theme,
                );
            }
            EffectKind::CardLeft => {
                if !enabled {
                    continue;
                }
                if let Some((_, pos)) = hand_pos.0.iter().find(|(id, _)| *id == e.card_id) {
                    if let Some(face) = painted.fronts.get(&e.card_id) {
                        ghost_card(&mut commands, *pos, face.clone());
                    }
                    dust(&mut commands, *pos, Vec2::new(40.0, 40.0), 7);
                    let to = anchors.scene
                        + Vec2::new(anchors.scene_size.x / 2.0, anchors.scene_size.y * 0.3);
                    trail(&mut commands, *pos, to);
                }
            }
            EffectKind::CardGained => {
                if !enabled {
                    continue;
                }
                dust(&mut commands, Vec2::new(430.0, 280.0), Vec2::new(80.0, 60.0), 7);
            }
            EffectKind::SceneChanged => {
                if !enabled || settings.reduced_motion {
                    continue;
                }
                shake.0 = shake.0.max(0.18);
            }
            EffectKind::Fate => {
                if !enabled {
                    continue;
                }
                shake.0 = shake.0.max(0.3);
                dust(
                    &mut commands,
                    anchors.scene,
                    Vec2::new(anchors.scene_size.x, anchors.scene_size.y),
                    10,
                );
            }
            EffectKind::Ending => {
                if !enabled {
                    continue;
                }
                let dark = e.text == "defeat" || e.text == "death";
                // 24 gold sparks above the scene panel (desktop consumeFX).
                for _ in 0..24 {
                    spawn_particle(
                        &mut commands,
                        Vec2::new(
                            anchors.scene.x + anchors.scene_size.x / 2.0 + rand_spread(180.0),
                            anchors.scene.y + 120.0,
                        ),
                        rand_spread(40.0),
                        -60.0 - rand_pseudo() * 80.0,
                        1.2 + rand_pseudo() * 0.5,
                        2.5,
                        theme::GOLD,
                    );
                }
                // Ambience: 5 waves of goldRise or embers (web).
                commands.spawn(Ambience {
                    t: 0.0,
                    wave: 0,
                    embers: dark,
                });
                vignette(&mut commands, &tex, dark);
            }
            EffectKind::CardGone => {}
        }
    }
}

fn label(stat: &str) -> String {
    alexander_core::game::stat_label(stat)
}

/// Builds the vignette overlay: a radial gradient, dark for grim endings
/// and gold for triumphs, fading in over 1.6s.
fn vignette(commands: &mut Commands, tex: &VignetteTex, dark: bool) {
    let base = if dark {
        Color::srgba(0.039, 0.024, 0.016, 0.55) // rgba(10,6,4,0.55)
    } else {
        Color::srgba(0.788, 0.635, 0.153, 0.12) // rgba(201,162,39,0.12)
    };
    let Some(image) = tex.0.clone() else {
        return;
    };
    let mut color = base;
    color.set_alpha(0.0);
    commands.spawn((
        Node {
            position_type: PositionType::Absolute,
            left: Val::Percent(0.0),
            top: Val::Percent(0.0),
            width: Val::Percent(100.0),
            height: Val::Percent(100.0),
            ..default()
        },
        ImageNode {
            image,
            color,
            ..default()
        },
        ZIndex(1000),
        Vignette { t: 0.0 },
        FadingImage {
            base,
            dur: 1.6,
            fade_out: false,
            t: 0.0,
        },
    ));
}

/// Advances every transient effect.
#[allow(clippy::too_many_arguments, clippy::type_complexity)]
pub fn step_fx(
    mut commands: Commands,
    time: Res<Time>,
    settings: Res<crate::input::Settings>,
    mut particles: Query<
        (Entity, &mut Particle, &mut Transform, &mut Sprite),
        (With<Particle>, Without<FloatText>, Without<TrailMote>),
    >,
    mut floats: Query<
        (Entity, &mut FloatText, &mut Transform, &mut TextColor),
        (With<FloatText>, Without<Particle>, Without<TrailMote>),
    >,
    mut ghosts: Query<
        (Entity, &mut GhostCard, &mut Node, &mut UiTransform),
        (With<GhostCard>, Without<ScenePanelTag>),
    >,
    mut motes: Query<
        (Entity, &mut TrailMote, &mut Transform),
        (With<TrailMote>, Without<Particle>, Without<FloatText>),
    >,
    mut vignettes: Query<(Entity, &mut Vignette)>,
    mut fading: Query<(&mut FadingImage, &mut ImageNode)>,
    mut ambience: Query<(Entity, &mut Ambience)>,
    anchors: Res<Anchors>,
    mut panel: Query<&mut UiTransform, (With<ScenePanelTag>, Without<GhostCard>)>,
    shake: Res<ScreenShake>,
) {
    let dt = time.delta_secs();
    let _ = settings;

    // Particles: drift, shrink, fade.
    for (e, mut p, mut t, mut s) in &mut particles {
        p.age += dt;
        if p.age >= p.ttl {
            commands.entity(e).despawn();
            continue;
        }
        t.translation.x += p.vel.x * dt;
        t.translation.y += p.vel.y * dt;
        let k = 1.0 - p.age / p.ttl;
        s.color.set_alpha(k * 0.75);
        s.custom_size = Some(Vec2::splat(p.size * (0.5 + k * 0.5)));
    }

    // Floats: rise 30px and fade over ~0.95s.
    for (e, mut f, mut t, mut c) in &mut floats {
        f.age += dt;
        if f.age >= 0.95 {
            commands.entity(e).despawn();
            continue;
        }
        t.translation.y += 32.0 * dt;
        c.0.set_alpha((1.0 - f.age / 0.95).min(1.0));
    }

    // Ghost cards: fly up-right (the fade rides on FadingImage).
    for (e, mut g, mut node, mut ui) in &mut ghosts {
        g.age += dt;
        if g.age >= 0.8 {
            commands.entity(e).despawn();
            continue;
        }
        let (lx, ty) = (px_val(node.left), px_val(node.top));
        node.left = Val::Px(lx + g.vel.x * dt);
        node.top = Val::Px(ty + g.vel.y * dt);
        ui.rotation = bevy::math::Rot2::radians(g.spin * g.age);
    }

    for (mut f, mut img) in &mut fading {
        f.t += dt;
        let k = (f.t / f.dur).clamp(0.0, 1.0);
        let mut c = f.base;
        c.set_alpha(f.base.alpha() * if f.fade_out { 1.0 - k } else { k });
        img.color = c;
    }

    // Trail motes: after the delay, sweep toward the scene anchor.
    for (e, mut m, mut t) in &mut motes {
        m.age += dt;
        if m.age < m.delay {
            continue;
        }
        let k = ((m.age - m.delay) / 0.62).min(1.0);
        let pos = m.from.lerp(m.to, k);
        t.translation = to_world(pos).extend(0.0);
        if k >= 1.0 {
            commands.entity(e).despawn();
        }
    }

    // Ambience waves: every 450ms, goldRise or embers across the panel.
    let mut waves: Vec<(Vec2, Vec2, bool)> = Vec::new();
    for (e, mut a) in &mut ambience {
        a.t += dt;
        if a.t >= 0.45 {
            a.t = 0.0;
            a.wave += 1;
            waves.push((anchors.scene, anchors.scene_size, a.embers));
        }
        if a.wave > 5 {
            commands.entity(e).despawn();
        }
    }
    for (at, size, ember) in waves {
        if ember {
            embers(&mut commands, at, size, 6);
        } else {
            gold_rise(&mut commands, at, size, 6);
        }
    }

    // Vignettes linger, then clear.
    for (e, mut v) in &mut vignettes {
        v.t += dt;
        if v.t > 8.0 {
            commands.entity(e).despawn();
        }
    }

    // Scene panel micro-shake (web .shake ±1px).
    if let Ok(mut transform) = panel.single_mut() {
        if shake.0 > 0.0 {
            let dx = (shake.0 * 44.0).sin() * shake.0;
            transform.translation = UiTransform::IDENTITY.translation;
            transform.translation.x = Val::Px(dx);
        } else {
            transform.translation = UiTransform::IDENTITY.translation;
        }
    }
}

/// Tracks the live hand cards' screen positions for FX anchoring.
pub fn track_hand_screen(
    camera: Query<(&Camera, &GlobalTransform), With<crate::TableCamera>>,
    cards: Query<(&crate::hand3d::HandCard, &Transform)>,
    windows: Query<&Window, With<PrimaryWindow>>,
    mut hand_pos: ResMut<HandScreenPos>,
) {
    let Ok((camera, cam_transform)) = camera.single() else {
        return;
    };
    let Ok(window) = windows.single() else {
        return;
    };
    hand_pos.0.clear();
    for (card, transform) in &cards {
        if let Ok(v) = camera.world_to_viewport(cam_transform, transform.translation) {
            hand_pos
                .0
                .push((card.id.clone(), Vec2::new(v.x, window.height() - v.y)));
        }
    }
}

fn px_val(v: Val) -> f32 {
    match v {
        Val::Px(x) => x,
        _ => 0.0,
    }
}
