package notify

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"

	"github.com/zhenzhu/watui/internal/desktop"
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

// SoundFiles writes watui's built-in sounds into dir (once) and returns their paths.
func SoundFiles(dir string) (message, call string, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	message = filepath.Join(dir, "watui-message.wav")
	call = filepath.Join(dir, "watui-call.wav")
	for path, pcm := range map[string]func() []int16{message: messageChime, call: callRing} {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, wav(pcm()), 0o600); err != nil {
			return "", "", err
		}
	}
	return message, call, nil
}

// Play plays a sound file in the background with the first available player
// (paplay / pw-play / aplay on Linux, afplay on macOS, else mpv).
func Play(path string) { desktop.Play(path) }
