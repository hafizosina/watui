package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ImageMode string

const (
	ImagesAlways    ImageMode = "always"
	ImagesThumbnail ImageMode = "thumbnail"
	ImagesOff       ImageMode = "off"
)

type Config struct {
	Images      ImageMode
	ImageWidth  int
	ImageHeight int

	CollapseLines int

	// ShowDeleted shows what a message said after its sender deleted it for everyone.
	ShowDeleted bool

	Notify      NotifyMode
	NotifySound string
	// Per kind of message; "" follows NotifySound.
	NotifySoundPersonal string // direct messages
	NotifySoundMention  string // @mentions of you and replies to you in groups
	NotifySoundGroup    string // other group messages

	Theme string
}

type NotifyMode string

const (
	NotifyOn      NotifyMode = "on"
	NotifyPrivate NotifyMode = "private"
	NotifyOff     NotifyMode = "off"
)

func Default() Config {
	return Config{
		Images:      ImagesAlways,
		ImageWidth:  40,
		ImageHeight: 15,

		CollapseLines: 15,
		ShowDeleted:   true,

		Notify:      NotifyOn,
		NotifySound: "watui",

		Theme: "tokyonight",
	}
}

const template = `# watui settings. Restart watui after editing, or change them live with :set key=value

# How pictures show in chats (kitty terminal):
#   always     download the full picture and show it inline
#   thumbnail  show the small preview WhatsApp sends with the message (no download, blurry)
#   off        text only, e.g. "[image] caption"
images = always

# Colors: tokyonight, tokyonight-day, catppuccin-mocha, catppuccin-latte, gruvbox-dark, nord, dracula,
# or your own file in ~/.config/watui/themes/<name>.conf. Try them live with :theme
theme = tokyonight

# Biggest picture size, in terminal cells.
image_width = 40
image_height = 15

# Long messages show only this many lines until you expand them (Space → Expand). 0 = never fold.
collapse_lines = 15

# Messages deleted for everyone: on = keep showing what they said (marked "deleted"),
# off = show only "message deleted". The content is kept either way.
show_deleted = on

# Desktop notifications for new messages:
#   on       sender + message text
#   private  sender only ("New message"), no text on screen
#   off      none
notify = on

# Notification sound: watui (built-in sounds: sword for direct messages and mentions,
# chime for other group messages, a ring for calls), system (desktop default), off,
# one built-in sound for everything (sword, chime, soft, ping), or a path to any .ogg/.wav file.
notify_sound = watui

# A different sound per kind of message (empty = follow notify_sound):
# sword, chime, soft, ping, system, off or a file path.
notify_sound_personal =
notify_sound_mention =
notify_sound_group =
`

func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "watui", "config"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return cfg, err
		}
		return cfg, os.WriteFile(path, []byte(template), 0o644)
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := cfg.Set(text); err != nil {
			return cfg, fmt.Errorf("%s line %d: %w", path, line, err)
		}
	}
	return cfg, scanner.Err()
}

func (c *Config) Set(assignment string) error {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok {
		return fmt.Errorf("expected key = value, got %q", assignment)
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	switch key {
	case "images":
		mode := ImageMode(value)
		if mode != ImagesAlways && mode != ImagesThumbnail && mode != ImagesOff {
			return fmt.Errorf("images must be always, thumbnail or off, got %q", value)
		}
		c.Images = mode
	case "image_width":
		n, err := strconv.Atoi(value)
		if err != nil || n < 4 {
			return fmt.Errorf("image_width must be a number >= 4, got %q", value)
		}
		c.ImageWidth = n
	case "image_height":
		n, err := strconv.Atoi(value)
		if err != nil || n < 2 {
			return fmt.Errorf("image_height must be a number >= 2, got %q", value)
		}
		c.ImageHeight = n
	case "notify":
		mode := NotifyMode(value)
		if mode != NotifyOn && mode != NotifyPrivate && mode != NotifyOff {
			return fmt.Errorf("notify must be on, private or off, got %q", value)
		}
		c.Notify = mode
	case "theme":
		if value == "" {
			return fmt.Errorf("theme needs a name")
		}
		c.Theme = value
	case "notify_sound":
		if value == "" {
			return fmt.Errorf("notify_sound needs a value: watui, %s, system, off or a file path", strings.Join(SoundNames, ", "))
		}
		if err := checkSound(key, value); err != nil {
			return err
		}
		c.NotifySound = value
	case "notify_sound_personal", "notify_sound_mention", "notify_sound_group":
		if err := checkSound(key, value); err != nil {
			return err
		}
		switch key {
		case "notify_sound_personal":
			c.NotifySoundPersonal = value
		case "notify_sound_mention":
			c.NotifySoundMention = value
		default:
			c.NotifySoundGroup = value
		}
	case "show_deleted":
		switch value {
		case "on", "true", "yes":
			c.ShowDeleted = true
		case "off", "false", "no":
			c.ShowDeleted = false
		default:
			return fmt.Errorf("show_deleted must be on or off, got %q", value)
		}
	case "collapse_lines":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("collapse_lines must be 0 or more, got %q", value)
		}
		c.CollapseLines = n
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

// SoundNames are watui's built-in notification sounds.
var SoundNames = []string{"sword", "chime", "soft", "ping"}

// checkSound accepts "" (follow notify_sound), a built-in name, watui, system,
// off, or an existing file.
func checkSound(key, value string) error {
	switch value {
	case "", "watui", "system", "off":
		return nil
	}
	for _, n := range SoundNames {
		if value == n {
			return nil
		}
	}
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("%s: %q is not %s, system, off or a file: %w", key, value, strings.Join(SoundNames, ", "), err)
	}
	return nil
}
