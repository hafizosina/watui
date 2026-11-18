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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// Frame is one picture of an image; a still image has one frame.
type Frame struct {
	Img   image.Image
	Delay time.Duration
}

type Loader func(ctx context.Context, chatJID, msgID string, q Quality) ([]Frame, error)

const (
	maxFrames   = 60
	animMaxW    = 480
	stillMaxW   = 900
	minFrameGap = 20 * time.Millisecond
)

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
	pic   cached
	gen   uint64 // Manager.gen when the picture was last sent to the terminal
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
	gen     uint64

	wmu sync.Mutex // one escape sequence at a time on the tty
}

var imageIDs atomic.Uint32

func init() {
	imageIDs.Store(uint32(time.Now().UnixNano()%100000) + 1000)
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
	if e, ok := m.entries[k]; ok {
		resend := e.state == stateReady && e.gen != m.gen
		if resend {
			e.gen = m.gen
		}
		id, pic, cols, rows := e.id, e.pic, e.cols, e.rows
		m.mu.Unlock()
		if resend {
			go func() { _ = m.transmit(id, pic, cols, rows) }()
		}
		return
	}
	e := &entry{state: stateLoading}
	m.entries[k] = e
	gen := m.gen
	m.mu.Unlock()

	go func() {
		m.sem <- struct{}{}
		defer func() { <-m.sem }()
		id, cols, rows, pic, err := m.prepare(chatJID, msgID, q, box)
		m.mu.Lock()
		if err != nil {
			e.state, e.err = stateFailed, err
		} else {
			e.state, e.id, e.cols, e.rows, e.pic, e.gen = stateReady, id, cols, rows, pic, gen
		}
		m.mu.Unlock()
		m.notify()
	}()
}

// Invalidate marks every picture as possibly missing from the terminal, so the
// next Request for it sends it again. tmux drops passthrough while the pane is
// hidden (allow-passthrough on), and a second tmux client never saw what the
// first one was sent; call this when the pane regains focus.
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.gen++
	m.mu.Unlock()
}

type cached struct {
	paths  []string
	delays []time.Duration
	w, h   int
}

func (m *Manager) framesDir(msgID string, q Quality) string {
	return filepath.Join(m.cacheDir, safeName(msgID)+fmt.Sprintf("-%d.anim", q))
}

func (m *Manager) readCache(msgID string, q Quality) (cached, bool) {
	still := filepath.Join(m.cacheDir, safeName(msgID)+fmt.Sprintf("-%d.png", q))
	if w, h, err := pngSize(still); err == nil {
		return cached{paths: []string{still}, delays: []time.Duration{0}, w: w, h: h}, true
	}
	dir := m.framesDir(msgID, q)
	data, err := os.ReadFile(filepath.Join(dir, "delays"))
	if err != nil {
		return cached{}, false
	}
	var c cached
	for i, field := range strings.Fields(string(data)) {
		ms, err := strconv.Atoi(field)
		if err != nil {
			return cached{}, false
		}
		c.paths = append(c.paths, filepath.Join(dir, fmt.Sprintf("%03d.png", i)))
		c.delays = append(c.delays, time.Duration(ms)*time.Millisecond)
	}
	if len(c.paths) == 0 {
		return cached{}, false
	}
	c.w, c.h, err = pngSize(c.paths[0])
	return c, err == nil
}

func (m *Manager) writeCache(msgID string, q Quality, frames []Frame) (cached, error) {
	if len(frames) == 1 {
		img := shrink(frames[0].Img, stillMaxW)
		path := filepath.Join(m.cacheDir, safeName(msgID)+fmt.Sprintf("-%d.png", q))
		if err := writePNG(path, img); err != nil {
			return cached{}, err
		}
		return cached{paths: []string{path}, delays: []time.Duration{0}, w: img.Bounds().Dx(), h: img.Bounds().Dy()}, nil
	}
	if len(frames) > maxFrames {
		frames = frames[:maxFrames]
	}
	dir := m.framesDir(msgID, q)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return cached{}, err
	}
	var c cached
	var delays []string
	for i, f := range frames {
		img := shrink(f.Img, animMaxW)
		path := filepath.Join(dir, fmt.Sprintf("%03d.png", i))
		if err := writePNG(path, img); err != nil {
			return cached{}, err
		}
		if i == 0 {
			c.w, c.h = img.Bounds().Dx(), img.Bounds().Dy()
		}
		d := max(f.Delay, minFrameGap)
		c.paths = append(c.paths, path)
		c.delays = append(c.delays, d)
		delays = append(delays, strconv.Itoa(int(d/time.Millisecond)))
	}
	return c, os.WriteFile(filepath.Join(dir, "delays"), []byte(strings.Join(delays, " ")+"\n"), 0o600)
}

func pathPayload(path string) []byte {
	return []byte(base64.StdEncoding.EncodeToString([]byte(path)))
}

func (m *Manager) prepare(chatJID, msgID string, q Quality, box Box) (uint32, int, int, cached, error) {
	c, ok := m.readCache(msgID, q)
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		frames, err := m.load(ctx, chatJID, msgID, q)
		if err != nil {
			return 0, 0, 0, cached{}, err
		}
		if len(frames) == 0 {
			return 0, 0, 0, cached{}, fmt.Errorf("no picture")
		}
		if c, err = m.writeCache(msgID, q, frames); err != nil {
			return 0, 0, 0, cached{}, err
		}
	}
	cols, rows := fit(c.w, c.h, box, cellRatio())
	id := imageIDs.Add(1)
	if m.nextID != 0 {
		m.mu.Lock()
		m.nextID++
		id = m.nextID
		m.mu.Unlock()
	}
	if err := m.transmit(id, c, cols, rows); err != nil {
		return 0, 0, 0, cached{}, err
	}
	return id, cols, rows, c, nil
}

// transmit sends the picture to kitty as image id with a virtual placement of
// cols×rows cells. Sending the same id again replaces the old copy.
func (m *Manager) transmit(id uint32, c cached, cols, rows int) error {
	seq := ansi.KittyGraphics(pathPayload(c.paths[0]),
		"a=T", "U=1", "f=100", "t=f", "q=2",
		fmt.Sprintf("i=%d", id), fmt.Sprintf("c=%d", cols), fmt.Sprintf("r=%d", rows))
	if err := m.write(seq); err != nil {
		return err
	}
	if len(c.paths) > 1 {
		return m.animate(id, c)
	}
	return nil
}

func (m *Manager) animate(id uint32, c cached) error {
	idOpt := fmt.Sprintf("i=%d", id)
	for i := 1; i < len(c.paths); i++ {
		seq := ansi.KittyGraphics(pathPayload(c.paths[i]), "a=f", idOpt, "f=100", "t=f", "q=2",
			fmt.Sprintf("z=%d", c.delays[i]/time.Millisecond))
		if err := m.write(seq); err != nil {
			return err
		}
	}
	if err := m.write(ansi.KittyGraphics(nil, "a=a", idOpt, "r=1", fmt.Sprintf("z=%d", c.delays[0]/time.Millisecond), "q=2")); err != nil {
		return err
	}
	return m.write(ansi.KittyGraphics(nil, "a=a", idOpt, "s=3", "v=1", "q=2"))
}

func (m *Manager) write(seq string) error {
	if m.inTmux {
		seq = ansi.TmuxPassthrough(seq)
	}
	m.wmu.Lock()
	defer m.wmu.Unlock()
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
		if c, ok := m.readCache(msgID, q); ok {
			return c.paths[0]
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
