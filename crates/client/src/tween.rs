//! Hand-rolled tween engine: a port of hand3d.js's per-property tweens
//! with cubic ease-out, delays, and per-target kill. Reduced motion
//! collapses durations to a single frame.

use bevy::prelude::*;

/// `1 - (1 - t)^3`.
pub fn ease_out(t: f32) -> f32 {
    1.0 - (1.0 - t).powi(3)
}

#[derive(Clone, Copy, PartialEq)]
pub enum Prop {
    X,
    Y,
    Z,
    RotX,
    RotY,
    RotZ,
    #[allow(dead_code)]
    Scale,
    Opacity,
}

struct Tween {
    target: Entity,
    prop: Prop,
    from: f32,
    to: f32,
    t: f32,
    delay: f32,
    dur: f32,
}

/// The active tween bank. Updated once per frame in `Update`.
#[derive(Resource, Default)]
pub struct Tweens {
    tweens: Vec<Tween>,
    /// Reduced motion: every duration collapses to one frame.
    pub reduced_motion: bool,
}

impl Tweens {
    /// Starts a tween; an existing tween on the same target+prop is
    /// replaced (per-prop kill).
    pub fn to(&mut self, target: Entity, prop: Prop, to: f32, dur: f32, delay: f32) {
        let dur = if self.reduced_motion { f32::EPSILON } else { dur.max(f32::EPSILON) };
        for tw in &mut self.tweens {
            if tw.target == target && tw.prop == prop {
                tw.to = to;
                tw.t = 0.0;
                tw.delay = delay;
                tw.dur = dur;
                return;
            }
        }
        self.tweens.push(Tween {
            target,
            prop,
            from: f32::NAN, // captured on first step
            to,
            t: 0.0,
            delay,
            dur,
        });
    }

    /// A fresh bank; reduced motion collapses all durations to a frame.
    pub fn new(reduced_motion: bool) -> Tweens {
        Tweens {
            tweens: Vec::new(),
            reduced_motion,
        }
    }

    /// Kills every tween touching the target.
    pub fn kill(&mut self, target: Entity) {
        self.tweens.retain(|tw| tw.target != target);
    }

    /// Advances every tween by `dt`, writing values through `transforms`
    /// and `fades`. Finished tweens are dropped.
    pub fn step(
        &mut self,
        dt: f32,
        transforms: &mut Query<(&mut Transform, &mut Fade)>,
        mut on_done: impl FnMut(Entity, Prop),
    ) {
        let mut finished: Vec<(Entity, Prop)> = Vec::new();
        for tw in &mut self.tweens {
            tw.t += dt;
            if tw.t <= tw.delay {
                continue;
            }
            let k = ease_out(((tw.t - tw.delay) / tw.dur).clamp(0.0, 1.0));
            let Ok((mut transform, mut fade)) = transforms.get_mut(tw.target) else {
                continue;
            };
            if tw.from.is_nan() {
                tw.from = current(&transform, &fade, tw.prop);
            }
            let v = tw.from + (tw.to - tw.from) * k;
            apply(&mut transform, &mut fade, tw.prop, v);
            if tw.t - tw.delay >= tw.dur {
                finished.push((tw.target, tw.prop));
            }
        }
        self.tweens
            .retain(|tw| !finished.contains(&(tw.target, tw.prop)));
        for (target, prop) in finished {
            on_done(target, prop);
        }
    }
}

/// Per-entity opacity for tweened fades (multiplied into material color
/// by the fading systems).
#[derive(Component, Default)]
pub struct Fade {
    pub opacity: f32,
}

impl Fade {
    pub fn full() -> Fade {
        Fade { opacity: 1.0 }
    }
}

fn current(t: &Transform, f: &Fade, prop: Prop) -> f32 {
    match prop {
        Prop::X => t.translation.x,
        Prop::Y => t.translation.y,
        Prop::Z => t.translation.z,
        Prop::RotX => t.rotation.to_euler(EulerRot::XYZ).0,
        Prop::RotY => t.rotation.to_euler(EulerRot::XYZ).1,
        Prop::RotZ => t.rotation.to_euler(EulerRot::XYZ).2,
        Prop::Scale => t.scale.x,
        Prop::Opacity => f.opacity,
    }
}

fn apply(t: &mut Transform, f: &mut Fade, prop: Prop, v: f32) {
    match prop {
        Prop::X => t.translation.x = v,
        Prop::Y => t.translation.y = v,
        Prop::Z => t.translation.z = v,
        Prop::RotX => {
            let (_, ry, rz) = t.rotation.to_euler(EulerRot::XYZ);
            t.rotation = Quat::from_euler(EulerRot::XYZ, v, ry, rz);
        }
        Prop::RotY => {
            let (rx, _, rz) = t.rotation.to_euler(EulerRot::XYZ);
            t.rotation = Quat::from_euler(EulerRot::XYZ, rx, v, rz);
        }
        Prop::RotZ => {
            let (rx, ry, _) = t.rotation.to_euler(EulerRot::XYZ);
            t.rotation = Quat::from_euler(EulerRot::XYZ, rx, ry, v);
        }
        Prop::Scale => t.scale = Vec3::splat(v),
        Prop::Opacity => f.opacity = v,
    }
}

/// Steps every tween once per frame.
pub fn step_tweens_system(
    time: Res<Time>,
    mut tweens: ResMut<Tweens>,
    mut query: Query<(&mut Transform, &mut Fade)>,
) {
    let dt = time.delta_secs();
    tweens.step(dt, &mut query, |_, _| {});
}
