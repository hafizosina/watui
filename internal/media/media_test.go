package media

import (
	"context"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestPipeline(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	m := &Manager{
		load: func(ctx context.Context, chat, id string, q Quality) ([]Frame, error) {
			img := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
			img.Set(10, 10, color.RGBA{R: 255, A: 255})
			return []Frame{{Img: img}}, nil
		},
		notify:   func() { done <- struct{}{} },
		cacheDir: t.TempDir(),
		tty:      out,
		inTmux:   true,
		sem:      make(chan struct{}, 1),
		entries:  map[string]*entry{},
		nextID:   41,
	}

	m.Request("chat", "MSG1", Full, Box{MaxCols: 40, MaxRows: 15})
	if _, status, _ := m.Lines("MSG1", Full); status != "loading…" {
		t.Fatalf("want loading status, got %q", status)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("image never loaded")
	}

	lines, status, ok := m.Lines("MSG1", Full)
	if !ok || status != "" {
		t.Fatalf("want ready, got ok=%v status=%q", ok, status)
	}
	if len(lines) != 15 {
		t.Fatalf("4:3 picture in a 40x15 box with 2:1 cells should be 15 rows, got %d", len(lines))
	}
	if w := ansi.StringWidth(lines[0]); w != 40 {
		t.Fatalf("row should be 40 cells wide, got %d", w)
	}
	if !strings.HasPrefix(lines[0], "\x1b[38;2;0;0;42m") {
		t.Fatalf("image id 42 should be in the fg color, got %q", lines[0][:20])
	}

	written, _ := os.ReadFile(out.Name())
	seq := string(written)
	for _, want := range []string{"\x1bPtmux;", "a=T", "U=1", "f=100", "t=f", "i=42", "c=40", "r=15"} {
		if !strings.Contains(seq, want) {
			t.Errorf("kitty command missing %q: %q", want, seq)
		}
	}
}

func TestInvalidateResends(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	m := &Manager{
		load: func(ctx context.Context, chat, id string, q Quality) ([]Frame, error) {
			return []Frame{{Img: image.NewRGBA(image.Rect(0, 0, 40, 30))}}, nil
		},
		notify:   func() { done <- struct{}{} },
		cacheDir: t.TempDir(),
		tty:      out,
		sem:      make(chan struct{}, 1),
		entries:  map[string]*entry{},
		nextID:   6,
	}
	box := Box{MaxCols: 20, MaxRows: 10}
	sends := func() int {
		written, _ := os.ReadFile(out.Name())
		return strings.Count(string(written), "a=T")
	}

	m.Request("chat", "MSG1", Full, box)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("image never loaded")
	}
	m.Request("chat", "MSG1", Full, box)
	time.Sleep(50 * time.Millisecond)
	if n := sends(); n != 1 {
		t.Fatalf("a ready picture should not be sent again, got %d sends", n)
	}

	m.Invalidate() // pane regained focus
	m.Request("chat", "MSG1", Full, box)
	m.Request("chat", "MSG1", Full, box)
	deadline := time.Now().Add(5 * time.Second)
	for sends() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := sends(); n != 2 {
		t.Fatalf("after Invalidate the picture should be sent exactly once more, got %d sends", n)
	}
	written, _ := os.ReadFile(out.Name())
	if strings.Count(string(written), "i=7") != 2 {
		t.Fatalf("resend should reuse image id 7: %q", written)
	}
}

func TestFit(t *testing.T) {
	cases := []struct {
		w, h, cols, rows int
	}{
		{1600, 1200, 40, 15},
		{1000, 200, 40, 4},
		{400, 1600, 8, 15},
	}
	for _, c := range cases {
		cols, rows := fit(c.w, c.h, Box{MaxCols: 40, MaxRows: 15}, 2.0)
		if cols != c.cols || rows != c.rows {
			t.Errorf("fit(%d,%d) = %dx%d, want %dx%d", c.w, c.h, cols, rows, c.cols, c.rows)
		}
	}
}

func TestAnimation(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	m := &Manager{
		load: func(ctx context.Context, chat, id string, q Quality) ([]Frame, error) {
			var frames []Frame
			for i := 0; i < 3; i++ {
				img := image.NewRGBA(image.Rect(0, 0, 200, 200))
				img.Set(i, i, color.RGBA{G: 255, A: 255})
				frames = append(frames, Frame{Img: img, Delay: 150 * time.Millisecond})
			}
			return frames, nil
		},
		notify:   func() { done <- struct{}{} },
		cacheDir: t.TempDir(),
		tty:      out,
		sem:      make(chan struct{}, 1),
		entries:  map[string]*entry{},
		nextID:   99,
	}
	m.Request("chat", "ANIM", Full, Box{MaxCols: 20, MaxRows: 10})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("animation never loaded")
	}
	if lines, status, _ := m.Lines("ANIM", Full); len(lines) == 0 {
		t.Fatalf("animation should render placeholders, status %q", status)
	}
	written, _ := os.ReadFile(out.Name())
	seq := string(written)
	if strings.Count(seq, "a=f") != 2 {
		t.Errorf("2 extra frames should be sent with a=f, got %d", strings.Count(seq, "a=f"))
	}
	for _, want := range []string{"a=T", "z=150", "s=3", "v=1"} {
		if !strings.Contains(seq, want) {
			t.Errorf("kitty commands missing %q", want)
		}
	}
	if c, ok := m.readCache("ANIM", Full); !ok || len(c.paths) != 3 {
		t.Fatal("frames should be cached for next time")
	}
}
