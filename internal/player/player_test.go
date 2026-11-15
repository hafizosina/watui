package player

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayAudio(t *testing.T) {
	if !HasMPV() {
		t.Skip("mpv not installed")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "tone.ogg")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=4", "-c:a", "libopus", path).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	audioArgs = []string{"--ao=null"}
	changes := make(chan struct{}, 10)
	p := New(func() { changes <- struct{}{} })
	defer p.Stop()

	if err := p.PlayAudio("MSG", path, 4*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	st, ok := p.State()
	if !ok || st.MsgID != "MSG" || st.Position <= 0 {
		t.Fatalf("should be playing MSG, got %+v ok=%v", st, ok)
	}
	if err := p.TogglePause(); err != nil {
		t.Fatalf("pause over mpv IPC: %v", err)
	}
	st, _ = p.State()
	pos := st.Position
	time.Sleep(300 * time.Millisecond)
	if st, _ = p.State(); !st.Paused || st.Position != pos {
		t.Fatalf("paused position should not move: %v -> %v", pos, st.Position)
	}
	if err := p.TogglePause(); err != nil {
		t.Fatal(err)
	}
	if speed, err := p.CycleSpeed(); err != nil || speed != 1.5 {
		t.Fatalf("speed: %v %v", speed, err)
	}
	p.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := p.State(); !ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("stop should end playback")
}
