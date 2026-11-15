package media

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"golang.org/x/image/draw"
	"golang.org/x/sys/unix"
)

type Quality int

const (
	Full Quality = iota
	Thumbnail
)

type Loader func(ctx context.Context, chatJID, msgID string, q Quality) (image.Image, error)

type state int

const (
	stateLoading state = iota
	stateReady
	stateFailed
)

type entry struct {
	state state
	id    uint32
	cols  int
	rows  int
	err   error
}

type Manager struct {
	load     Loader
	notify   func()
	cacheDir string
	tty      *os.File
	inTmux   bool
	sem      chan struct{}

	mu      sync.Mutex
	entries map[string]*entry
	nextID  uint32
}

func NewManager(load Loader, notify func()) (*Manager, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "watui", "images")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	return &Manager{
		load:     load,
		notify:   notify,
		cacheDir: dir,
		tty:      tty,
		inTmux:   os.Getenv("TMUX") != "",
		sem:      make(chan struct{}, 3),
		entries:  map[string]*entry{},
		nextID:   uint32(time.Now().UnixNano()%100000) + 1000,
	}, nil
}

func key(msgID string, q Quality) string {
	return fmt.Sprintf("%s/%d", msgID, q)
}

type Box struct {
	MaxCols, MaxRows int
}

func (m *Manager) Request(chatJID, msgID string, q Quality, box Box) {
	k := key(msgID, q)
	m.mu.Lock()
	if _, ok := m.entries[k]; ok {
		m.mu.Unlock()
		return
	}
	e := &entry{state: stateLoading}
	m.entries[k] = e
	m.mu.Unlock()

	go func() {
		m.sem <- struct{}{}
		defer func() { <-m.sem }()
		id, cols, rows, err := m.prepare(chatJID, msgID, q, box)
		m.mu.Lock()
		if err != nil {
			e.state, e.err = stateFailed, err
		} else {
			e.state, e.id, e.cols, e.rows = stateReady, id, cols, rows
		}
		m.mu.Unlock()
		m.notify()
	}()
}

func (m *Manager) prepare(chatJID, msgID string, q Quality, box Box) (uint32, int, int, error) {
	path := filepath.Join(m.cacheDir, safeName(msgID)+fmt.Sprintf("-%d.png", q))
	w, h, err := pngSize(path)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		img, err := m.load(ctx, chatJID, msgID, q)
		if err != nil {
			return 0, 0, 0, err
		}
		img = shrink(img, 900)
		if err := writePNG(path, img); err != nil {
			return 0, 0, 0, err
		}
		w, h = img.Bounds().Dx(), img.Bounds().Dy()
	}
	cols, rows := fit(w, h, box, cellRatio())
	m.mu.Lock()
	m.nextID++
	id := m.nextID
	m.mu.Unlock()
	seq := ansi.KittyGraphics([]byte(base64.StdEncoding.EncodeToString([]byte(path))),
		"a=T", "U=1", "f=100", "t=f", "q=2",
		fmt.Sprintf("i=%d", id), fmt.Sprintf("c=%d", cols), fmt.Sprintf("r=%d", rows))
	if err := m.write(seq); err != nil {
		return 0, 0, 0, err
	}
	return id, cols, rows, nil
}

func (m *Manager) write(seq string) error {
	if m.inTmux {
		seq = ansi.TmuxPassthrough(seq)
	}
	_, err := m.tty.WriteString(seq)
	return err
}

func (m *Manager) Lines(msgID string, q Quality) ([]string, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key(msgID, q)]
	if !ok {
		return nil, "", false
	}
	switch e.state {
	case stateLoading:
		return nil, "loading…", true
	case stateFailed:
		return nil, "could not load: " + shortErr(e.err), true
	}
	color := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", (e.id>>16)&0xff, (e.id>>8)&0xff, e.id&0xff)
	lines := make([]string, e.rows)
	for r := 0; r < e.rows; r++ {
		var b strings.Builder
		b.WriteString(color)
		for c := 0; c < e.cols; c++ {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[39m")
		lines[r] = b.String()
	}
	return lines, "", true
}

func (m *Manager) Path(msgID string) string {
	for _, q := range []Quality{Full, Thumbnail} {
		path := filepath.Join(m.cacheDir, safeName(msgID)+fmt.Sprintf("-%d.png", q))
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (m *Manager) Reset() {
	m.mu.Lock()
	ids := m.ids()
	m.entries = map[string]*entry{}
	m.mu.Unlock()
	m.deleteIDs(ids)
}

func (m *Manager) Close() {
	m.mu.Lock()
	ids := m.ids()
	m.mu.Unlock()
	m.deleteIDs(ids)
	_ = m.tty.Close()
}

func (m *Manager) ids() []uint32 {
	var ids []uint32
	for _, e := range m.entries {
		if e.state == stateReady {
			ids = append(ids, e.id)
		}
	}
	return ids
}

func (m *Manager) deleteIDs(ids []uint32) {
	for _, id := range ids {
		_ = m.write(ansi.KittyGraphics(nil, "a=d", "d=I", "q=2", fmt.Sprintf("i=%d", id)))
	}
}

func fit(w, h int, box Box, ratio float64) (int, int) {
	if w <= 0 || h <= 0 {
		return 1, 1
	}
	cols := box.MaxCols
	rows := int(math.Round(float64(cols) * float64(h) / float64(w) / ratio))
	if rows > box.MaxRows {
		rows = box.MaxRows
		cols = int(math.Round(float64(rows) * ratio * float64(w) / float64(h)))
	}
	return max(1, min(cols, box.MaxCols)), max(1, rows)
}

func cellRatio() float64 {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
		return 2.0
	}
	cellW := float64(ws.Xpixel) / float64(ws.Col)
	cellH := float64(ws.Ypixel) / float64(ws.Row)
	return cellH / cellW
}

func shrink(img image.Image, maxW int) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxW {
		return img
	}
	h := b.Dy() * maxW / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, maxW, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func writePNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func pngSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' {
			return '_'
		}
		return r
	}, s)
}

func shortErr(err error) string {
	if err == nil {
		return "unknown error"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "404"), strings.Contains(s, "410"):
		return "expired on WhatsApp's server"
	case len(s) > 60:
		return s[:60] + "…"
	}
	return s
}
