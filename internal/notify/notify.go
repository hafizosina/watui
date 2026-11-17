package notify

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/hafizosina/watui/internal/desktop"
)

type Action struct {
	Name  string
	Label string
}

type SoundKind int

const (
	SoundMessage SoundKind = iota
	SoundCall
)

type Note struct {
	Key     string
	Title   string
	Body    string
	Actions []Action
	Sound   SoundKind
}

type Notifier struct {
	onOpen func(key, action string)

	mu  sync.Mutex
	ids map[string]uint32

	soundMode    string
	messageSound string
	callSound    string
}

// SetSound picks how notifications sound: "watui" (built-in chimes), "system", "off", or a file path.
func (n *Notifier) SetSound(mode, cacheDir string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.soundMode = mode
	n.messageSound, n.callSound = "", ""
	switch mode {
	case "system", "off", "":
	case "watui":
		if msg, call, err := SoundFiles(cacheDir); err == nil {
			n.messageSound, n.callSound = msg, call
		}
	default:
		n.messageSound, n.callSound = mode, mode
	}
}

// SoundDir is where SetSound keeps the generated "watui" sounds.
func SoundDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "watui", "sounds")
}

func New(onOpen func(key, action string)) *Notifier {
	return &Notifier{onOpen: onOpen, ids: map[string]uint32{}}
}

// Send shows (or updates, per Key) a desktop notification; clicking it calls onOpen(Key).
func (n *Notifier) Send(note Note) {
	if !Available() {
		return
	}
	n.mu.Lock()
	id := n.ids[note.Key]
	n.mu.Unlock()

	n.mu.Lock()
	mode := n.soundMode
	sound := n.messageSound
	if note.Sound == SoundCall {
		sound = n.callSound
	}
	n.mu.Unlock()
	if mode != "off" && mode != "system" {
		Play(sound)
	}
	n.show(note, id, mode == "system" || (mode != "off" && sound == ""))
}

// Forget drops the replace id so the next note for this key pops up fresh.
func (n *Notifier) Forget(key string) {
	n.mu.Lock()
	delete(n.ids, key)
	n.mu.Unlock()
}

// FocusTerminal brings watui's tmux pane and terminal window to the front.
func FocusTerminal() {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		_ = exec.Command("tmux", "select-window", "-t", pane).Run()
		_ = exec.Command("tmux", "select-pane", "-t", pane).Run()
	}
	desktop.RaiseTerminal()
}
