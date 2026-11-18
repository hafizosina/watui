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
	SoundMessage SoundKind = iota // direct message
	SoundCall
	SoundGroup   // any other group message
	SoundMention // an @mention of you, or a reply to you, in a group
)

// defaultSound is what "watui" means for each kind.
var defaultSound = map[SoundKind]string{
	SoundMessage: "sword",
	SoundMention: "sword",
	SoundGroup:   "chime",
	SoundCall:    "ring",
}

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

	sounds map[SoundKind]string // "off", "system", or a file to play; "" = the desktop's sound
}

// SetSound picks how notifications sound. base is notify_sound and covers every
// kind: "watui" (each kind its own built-in sound), a built-in name ("chime",
// "sword", "soft", "ping"), "system", "off", or a file path. perKind overrides it for
// some kinds; "" there means follow base. Calls ring unless sound is off,
// system or a file.
func (n *Notifier) SetSound(base string, perKind map[SoundKind]string, cacheDir string) {
	sounds := map[SoundKind]string{}
	for kind, def := range defaultSound {
		v := base
		if o := perKind[kind]; o != "" && kind != SoundCall {
			v = o
		}
		switch {
		case v == "":
			v = "system"
		case v == "watui", kind == SoundCall && Builtin[v] != nil:
			v = def
		}
		switch {
		case v == "off" || v == "system":
		case Builtin[v] != nil:
			path, err := BuiltinFile(cacheDir, v)
			if err != nil {
				path = "" // can't write it: fall back to the desktop's sound
			}
			v = path
		}
		sounds[kind] = v
	}
	n.mu.Lock()
	n.sounds = sounds
	n.mu.Unlock()
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
	sound, set := n.sounds[note.Sound]
	n.mu.Unlock()
	switch {
	case !set:
		sound = "system" // SetSound not called
	case sound != "off" && sound != "system" && sound != "":
		Play(sound)
	}
	n.show(note, id, sound == "system" || sound == "")
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
