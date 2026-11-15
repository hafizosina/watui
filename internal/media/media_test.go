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
		load: func(ctx context.Context, chat, id string, q Quality) (image.Image, error) {
			img := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
			img.Set(10, 10, color.RGBA{R: 255, A: 255})
			return img, nil
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
