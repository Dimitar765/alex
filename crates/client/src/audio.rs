//! The seven-cue synth table, parameter-identical to the Go desktop
//! audio.go / web game.js oscillators: exponential gain decay,
//! exponential frequency slides, and an RBJ bandpass over white noise.
//! Cues are rendered to f32 PCM at 44.1 kHz, wrapped in a hand-rolled
//! WAV header (no audio crates), and played through `bevy_audio`.

use std::f32::consts::PI;
use std::sync::Arc;

use bevy::audio::Volume;
use bevy::prelude::*;

pub const SAMPLE_RATE: f32 = 44100.0;
/// Cue names, mirroring the Go client's `Play` table.
pub const CUES: [&str; 7] = ["card", "choice", "shuffle", "scout", "error", "victory", "defeat"];

/// A request to play a named cue.
#[derive(Message)]
pub struct Sfx(pub &'static str);

/// One synthesized cue, pre-rendered at startup.
#[derive(Resource, Default)]
pub struct CueBank {
    wavs: Vec<(&'static str, AudioSource)>,
}

impl CueBank {
    pub fn build() -> CueBank {
        let mut bank = CueBank::default();
        for name in CUES {
            let bytes: Arc<[u8]> = Arc::from(synth_cue(name));
            bank.wavs.push((name, AudioSource { bytes }));
        }
        bank
    }

    /// Spawns a one-shot player for the cue.
    pub fn play(
        &self,
        name: &str,
        muted: bool,
        commands: &mut Commands,
        sources: &mut Assets<AudioSource>,
    ) {
        if muted {
            return;
        }
        let Some((_, source)) = self.wavs.iter().find(|(n, _)| *n == name) else {
            return;
        };
        let handle = sources.add(source.clone());
        commands.spawn((
            AudioPlayer::new(handle),
            PlaybackSettings::ONCE.with_volume(Volume::Linear(1.0)),
            DespawnTimer(4.0),
        ));
    }
}

/// Despawns finished one-shot audio players.
#[derive(Component)]
pub struct DespawnTimer(pub f32);

pub fn despawn_finished(
    time: Res<Time>,
    mut commands: Commands,
    mut players: Query<(Entity, &mut DespawnTimer)>,
) {
    for (entity, mut t) in &mut players {
        t.0 -= time.delta_secs();
        if t.0 <= 0.0 {
            commands.entity(entity).despawn();
        }
    }
}

/// Plays queued cues through the bank.
pub fn play_cues(
    mut reader: MessageReader<Sfx>,
    bank: Res<CueBank>,
    settings: Res<crate::input::Settings>,
    mut commands: Commands,
    mut sources: ResMut<Assets<AudioSource>>,
) {
    for cue in reader.read() {
        bank.play(cue.0, settings.muted, &mut commands, &mut sources);
    }
}

/// Renders one cue's mix to WAV bytes.
fn synth_cue(name: &str) -> Vec<u8> {
    let mut m = Mixer::default();
    match name {
        "card" => {
            m.add(0.0, noise(0.09, 1600.0));
            m.add(0.0, tone(196.0, 0.08, Wave::Triangle, 0.05, 0.0));
        }
        "choice" => {
            m.add(0.0, tone(392.0, 0.06, Wave::Triangle, 0.06, 0.0));
            m.add(0.055, tone(523.0, 0.09, Wave::Triangle, 0.05, 0.0));
        }
        "shuffle" => {
            m.add(0.0, noise(0.12, 900.0));
            m.add(0.09, noise(0.12, 1200.0));
        }
        "scout" => {
            m.add(0.0, tone(587.0, 0.05, Wave::Sine, 0.06, 0.0));
            m.add(0.07, tone(784.0, 0.08, Wave::Sine, 0.05, 0.0));
        }
        "error" => {
            m.add(0.0, tone(147.0, 0.22, Wave::Sawtooth, 0.05, 98.0));
        }
        "victory" => {
            for (i, f) in [523.0, 659.0, 784.0].into_iter().enumerate() {
                m.add(i as f32 * 0.11, tone(f, 0.4, Wave::Sine, 0.07, 0.0));
            }
        }
        "defeat" => {
            m.add(0.0, tone(196.0, 0.7, Wave::Triangle, 0.07, 110.0));
        }
        _ => {}
    }
    wav(m.encode())
}

enum Wave {
    Sine,
    Triangle,
    Sawtooth,
}

/// One oscillator note: exponential gain decay over `dur`, with an
/// optional exponential frequency slide to `slide_to` Hz.
fn tone(freq: f32, dur: f32, kind: Wave, gain: f32, slide_to: f32) -> Vec<f32> {
    let n = (dur * SAMPLE_RATE) as usize;
    let mut out = Vec::with_capacity(n);
    let mut phase = 0.0f32;
    for i in 0..n {
        let k = i as f32 / n as f32;
        let f = if slide_to > 0.0 {
            freq * (slide_to / freq).powf(k)
        } else {
            freq
        };
        phase += 2.0 * PI * f / SAMPLE_RATE;
        let v = match kind {
            Wave::Triangle => 2.0 / PI * phase.sin().asin(),
            Wave::Sawtooth => 2.0 * (phase / (2.0 * PI) - (0.5 + phase / (2.0 * PI)).floor()),
            Wave::Sine => phase.sin(),
        };
        // Exponential ramp down to near silence, like the WebAudio gain.
        let g = gain * (-5.0 * k).exp();
        out.push(v * g);
    }
    out
}

/// Decayed white noise through a biquad bandpass at `freq` — the
/// riffle/rustle voice. RBJ constants, constant peak gain, Q = 1.
fn noise(dur: f32, freq: f32) -> Vec<f32> {
    use std::cell::Cell;
    thread_local!(static RNG: Cell<u64> = const { Cell::new(0x9E3779B97F4A7C15) });
    let n = (dur * SAMPLE_RATE) as usize;
    let mut raw = Vec::with_capacity(n);
    for i in 0..n {
        // xorshift64 — deterministic-enough white noise for a rustle.
        let s = RNG.with(|r| {
            let mut x = r.get();
            x ^= x << 13;
            x ^= x >> 7;
            x ^= x << 17;
            r.set(x);
            x
        });
        let white = ((s as f64 / u64::MAX as f64) * 2.0 - 1.0) as f32;
        raw.push(white * (1.0 - i as f32 / n as f32));
    }
    let w0 = 2.0 * PI * freq / SAMPLE_RATE;
    let alpha = w0.sin() / 2.0;
    let (b0, b1, b2) = (alpha, 0.0, -alpha);
    let (a0, a1, a2) = (1.0 + alpha, -2.0 * w0.cos(), 1.0 - alpha);
    let mut out = Vec::with_capacity(n);
    let (mut x1, mut x2, mut y1, mut y2) = (0.0f32, 0.0f32, 0.0f32, 0.0f32);
    for &r in &raw {
        let x0 = r;
        let y = b0 / a0 * x0 + b1 / a0 * x1 + b2 / a0 * x2 - a1 / a0 * y1 - a2 / a0 * y2;
        x2 = x1;
        x1 = x0;
        y2 = y1;
        y1 = y;
        out.push(y * 4.0); // bandpass attenuation compensation
    }
    out
}

/// Accumulates float32 mono samples at cue offsets.
#[derive(Default)]
struct Mixer {
    data: Vec<f32>,
}

impl Mixer {
    fn add(&mut self, offset: f32, samples: Vec<f32>) {
        let start = (offset * SAMPLE_RATE) as usize;
        let end = start + samples.len();
        if end > self.data.len() {
            self.data.resize(end, 0.0);
        }
        for (i, s) in samples.into_iter().enumerate() {
            self.data[start + i] += s;
        }
    }

    /// Flattens the mix to 16-bit little-endian PCM, clipping peaks.
    fn encode(&self) -> Vec<u8> {
        let mut out = Vec::with_capacity(self.data.len() * 2);
        for s in &self.data {
            let s = s.clamp(-1.0, 1.0);
            let v = (s * 32767.0 * 0.9) as i16;
            out.extend_from_slice(&v.to_le_bytes());
        }
        out
    }
}

/// Wraps 16-bit mono PCM in a minimal WAV (RIFF) header.
fn wav(pcm: Vec<u8>) -> Vec<u8> {
    let mut b = Vec::with_capacity(44 + pcm.len());
    b.extend_from_slice(b"RIFF");
    let chunk = (36 + pcm.len()) as u32;
    b.extend_from_slice(&chunk.to_le_bytes());
    b.extend_from_slice(b"WAVE");
    b.extend_from_slice(b"fmt ");
    b.extend_from_slice(&16u32.to_le_bytes()); // fmt chunk size
    b.extend_from_slice(&1u16.to_le_bytes()); // PCM
    b.extend_from_slice(&1u16.to_le_bytes()); // mono
    b.extend_from_slice(&(SAMPLE_RATE as u32).to_le_bytes());
    let byte_rate = SAMPLE_RATE as u32 * 2;
    b.extend_from_slice(&byte_rate.to_le_bytes());
    b.extend_from_slice(&2u16.to_le_bytes()); // block align
    b.extend_from_slice(&16u16.to_le_bytes()); // bits per sample
    b.extend_from_slice(b"data");
    b.extend_from_slice(&(pcm.len() as u32).to_le_bytes());
    b.extend_from_slice(&pcm);
    b
}
