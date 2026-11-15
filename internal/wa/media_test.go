package wa

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func need(t *testing.T, tools ...string) {
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
}

func TestAnimatedWebP(t *testing.T) {
	need(t, "ffmpeg", "convert", "identify")
	path := filepath.Join(t.TempDir(), "s.webp")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x160:rate=5:duration=1",
		"-c:v", "libwebp_anim", "-loop", "0", path).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make animated webp here: %v %s", err, out)
	}
	frames, err := animatedWebP(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 5 || frames[0].Delay != 200*time.Millisecond || frames[0].Img.Bounds().Dx() != 160 {
		t.Fatalf("want 5 frames of 160px at 200ms, got %d (delay %v)", len(frames), frames[0].Delay)
	}
}

func TestGIFFrames(t *testing.T) {
	need(t, "ffmpeg")
	path := filepath.Join(t.TempDir(), "g.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=3",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", path).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	frames, err := videoFrames(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 30 || frames[0].Img.Bounds().Dx() != 480 {
		t.Fatalf("3s at 10fps scaled to 480px should give 30 frames, got %d at %dpx", len(frames), frames[0].Img.Bounds().Dx())
	}
}
