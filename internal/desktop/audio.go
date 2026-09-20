package desktop

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// sampleRate is the audio context rate. Voices are synthesized in memory
// — the same cue table as the web build's WebAudio synth, no files.
const sampleRate = 44100

// audioSys owns the audio context and the mute toggle.
type audioSys struct {
	ctx   *audio.Context
	muted bool
}

func newAudio() *audioSys {
	return &audioSys{ctx: audio.NewContext(sampleRate)}
}

// Play renders and starts a named cue: card, choice, shuffle, scout,
// error, victory, defeat.
func (a *audioSys) Play(name string) {
	if a == nil || a.muted {
		return
	}
	m := &mixer{}
	switch name {
	case "card":
		m.add(0, a.noise(0.09, 1600))
		m.add(0, a.tone(196, 0.08, "triangle", 0.05, 0))
	case "choice":
		m.add(0, a.tone(392, 0.06, "triangle", 0.06, 0))
		m.add(0.055, a.tone(523, 0.09, "triangle", 0.05, 0))
	case "shuffle":
		m.add(0, a.noise(0.12, 900))
		m.add(0.09, a.noise(0.12, 1200))
	case "scout":
		m.add(0, a.tone(587, 0.05, "sine", 0.06, 0))
		m.add(0.07, a.tone(784, 0.08, "sine", 0.05, 0))
	case "error":
		m.add(0, a.tone(147, 0.22, "sawtooth", 0.05, 98))
	case "victory":
		for i, f := range []float64{523, 659, 784} {
			m.add(float64(i)*0.11, a.tone(f, 0.4, "sine", 0.07, 0))
		}
	case "defeat":
		m.add(0, a.tone(196, 0.7, "triangle", 0.07, 110))
	default:
		return
	}
	p, err := a.ctx.NewPlayer(bytes.NewReader(m.encode()))
	if err != nil {
		return
	}
	p.Play()
}

// mixer accumulates float32 mono samples at cue offsets.
type mixer struct {
	data []float32
}

// add splices samples starting at offset seconds.
func (m *mixer) add(offset float64, samples []float32) {
	start := int(offset * sampleRate)
	if end := start + len(samples); end > len(m.data) {
		grown := make([]float32, end)
		copy(grown, m.data)
		m.data = grown
	}
	for i, s := range samples {
		m.data[start+i] += s
	}
}

// encode flattens the mix to 16-bit little-endian PCM, clipping peaks.
func (m *mixer) encode() []byte {
	buf := &bytes.Buffer{}
	for _, s := range m.data {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		v := int16(s * 32767 * 0.9)
		binary.Write(buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}

// tone synthesizes one oscillator note: an exponential gain decay over
// dur, with an optional exponential frequency slide to slideTo Hz.
func (a *audioSys) tone(freq, dur float64, kind string, gain, slideTo float64) []float32 {
	n := int(dur * sampleRate)
	out := make([]float32, 0, n)
	phase := 0.0
	for i := 0; i < n; i++ {
		k := float64(i) / float64(n)
		f := freq
		if slideTo > 0 {
			f = freq * math.Pow(slideTo/freq, k)
		}
		phase += 2 * math.Pi * f / sampleRate
		var v float64
		switch kind {
		case "triangle":
			v = 2 / math.Pi * math.Asin(math.Sin(phase))
		case "sawtooth":
			v = 2 * (phase/(2*math.Pi) - math.Floor(0.5+phase/(2*math.Pi)))
		default:
			v = math.Sin(phase)
		}
		// Exponential ramp down to near silence, like the WebAudio gain.
		g := gain * math.Exp(-5*k)
		out = append(out, float32(v*g))
	}
	return out
}

// noise synthesizes decayed white noise through a biquad bandpass at
// freq — the riffle/rustle voice.
func (a *audioSys) noise(dur, freq float64) []float32 {
	n := int(dur * sampleRate)
	raw := make([]float32, n)
	for i := range raw {
		raw[i] = float32((rand.Float64()*2 - 1) * (1 - float64(i)/float64(n)))
	}
	// RBJ bandpass, constant peak gain, Q = 1.
	w0 := 2 * math.Pi * freq / sampleRate
	alpha := math.Sin(w0) / 2
	b0, b1, b2 := alpha, 0.0, -alpha
	a0, a1, a2 := 1+alpha, -2*math.Cos(w0), 1-alpha
	out := make([]float32, n)
	x1, x2, y1, y2 := 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		x0 := float64(raw[i])
		y := b0/a0*x0 + b1/a0*x1 + b2/a0*x2 - a1/a0*y1 - a2/a0*y2
		x2, x1, y2, y1 = x1, x0, y1, y
		out[i] = float32(y * 4) // bandpass attenuation compensation
	}
	return out
}
