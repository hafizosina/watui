package notify

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/hafizosina/watui/internal/desktop"
)

const sampleRate = 44100

type tone struct {
	freq, start, dur, gain float64
}

// chime renders a short two-note bell with harmonics and an exponential decay.
func chime(tones []tone, length float64) []int16 {
	n := int(length * sampleRate)
	pcm := make([]float64, n)
	for _, t := range tones {
		from := int(t.start * sampleRate)
		for i := 0; i < int(t.dur*sampleRate) && from+i < n; i++ {
			x := float64(i) / sampleRate
			env := math.Exp(-x*7) * math.Min(1, x*400)
			v := math.Sin(2*math.Pi*t.freq*x) + 0.35*math.Sin(2*math.Pi*t.freq*2*x) + 0.12*math.Sin(2*math.Pi*t.freq*3.01*x)
			pcm[from+i] += t.gain * env * v
		}
	}
	out := make([]int16, n)
	for i, v := range pcm {
		out[i] = int16(math.Max(-1, math.Min(1, v*0.45)) * 32767)
	}
	return out
}

func wav(samples []int16) []byte {
	var b bytes.Buffer
	data := len(samples) * 2
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+data))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&b, binary.LittleEndian, uint32(sampleRate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(data))
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

func messageChime() []int16 {
	return chime([]tone{
		{freq: 1318.5, start: 0, dur: 0.35, gain: 0.8},
		{freq: 1760, start: 0.11, dur: 0.4, gain: 0.9},
	}, 0.55)
}

// groupChime is a lower, softer, falling chime: an option for quiet group chatter.
func groupChime() []int16 {
	return chime([]tone{
		{freq: 784, start: 0, dur: 0.25, gain: 0.55},
		{freq: 587.3, start: 0.09, dur: 0.3, gain: 0.5},
	}, 0.42)
}

// mentionPing is a quick bright double tap with a high finish, for @mentions if
// you want them to stand out from direct messages.
func mentionPing() []int16 {
	return chime([]tone{
		{freq: 1568, start: 0, dur: 0.16, gain: 0.7},
		{freq: 1568, start: 0.11, dur: 0.16, gain: 0.7},
		{freq: 2093, start: 0.22, dur: 0.35, gain: 0.85},
	}, 0.6)
}

// swordShing is a blade drawn from its sheath: a rising swoosh of filtered noise,
// then a bright metallic ring with slightly detuned partials that shimmer as they fade.
func swordShing() []int16 {
	const length = 1.1
	n := int(length * sampleRate)
	pcm := make([]float64, n)
	rng := rand.New(rand.NewSource(7)) // same sound every time

	// swoosh: noise through a band-pass sweeping 900 Hz → 6 kHz
	swoosh := int(0.24 * sampleRate)
	var low, band float64
	for i := 0; i < swoosh; i++ {
		t := float64(i) / float64(swoosh)
		fc := 900 * math.Pow(6.5, t)
		f := 2 * math.Sin(math.Pi*fc/sampleRate)
		high := (rng.Float64()*2 - 1) - low - 0.2*band
		band += f * high
		low += f * band
		env := 6.75 * t * t * (1 - t) // swells, peaks at 2/3 as the ring starts, fades to 0
		pcm[i] += 0.5 * env * band
	}

	// ring: inharmonic partials like struck steel, a short scrape at the start
	ringAt := int(0.15 * sampleRate)
	partials := []struct{ freq, gain, decay float64 }{
		{2637, 0.55, 3.2}, {2643, 0.35, 3.0}, // a beating pair: the shimmer
		{3951, 0.35, 4.5}, {5274, 0.25, 6}, {6912, 0.15, 8}, {8370, 0.08, 11},
	}
	for i := 0; ringAt+i < n; i++ {
		x := float64(i) / sampleRate
		attack := math.Min(1, x*600)
		v := 0.0
		for _, p := range partials {
			v += p.gain * math.Exp(-x*p.decay) * math.Sin(2*math.Pi*p.freq*x)
		}
		if x < 0.03 {
			v += 0.4 * (1 - x/0.03) * (rng.Float64()*2 - 1)
		}
		pcm[ringAt+i] += attack * v
	}

	peak := 0.0
	for _, v := range pcm {
		peak = math.Max(peak, math.Abs(v))
	}
	out := make([]int16, n)
	for i, v := range pcm {
		out[i] = int16(v / peak * 0.5 * 32767)
	}
	return out
}

func callRing() []int16 {
	var tones []tone
	for i := 0; i < 3; i++ {
		s := float64(i) * 0.9
		tones = append(tones,
			tone{freq: 987.8, start: s, dur: 0.3, gain: 0.8},
			tone{freq: 1318.5, start: s + 0.15, dur: 0.3, gain: 0.8},
			tone{freq: 1760, start: s + 0.3, dur: 0.45, gain: 0.9},
		)
	}
	return chime(tones, 2.9)
}

// Builtin are watui's own sounds, by the name used in the config.
var Builtin = map[string]func() []int16{
	"sword": swordShing,   // direct messages and mentions by default
	"chime": messageChime, // bright two-note bell: group messages by default
	"soft":  groupChime,   // low, falling
	"ping":  mentionPing,  // double tap
	"ring":  callRing,     // calls
}

// BuiltinNames lists the built-in message sounds, for settings and completion.
var BuiltinNames = []string{"sword", "chime", "soft", "ping"}

// BuiltinFile writes the built-in sound name into dir (once) and returns its path.
func BuiltinFile(dir, name string) (string, error) {
	pcm, ok := Builtin[name]
	if !ok {
		return "", fmt.Errorf("no built-in sound %q", name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "watui-"+name+".wav")
	data := wav(pcm())
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return path, nil
	}
	// rewritten when watui's version of the sound changed
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// Play plays a sound file in the background with the first available player
// (paplay / pw-play / aplay on Linux, afplay on macOS, else mpv).
func Play(path string) { desktop.Play(path) }
