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

	Notify      NotifyMode
	NotifySound string
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

		Notify:      NotifyOn,
		NotifySound: "watui",
	}
}

const template = `# watui settings. Restart watui after editing, or change them live with :set key=value

# How pictures show in chats (kitty terminal):
#   always     download the full picture and show it inline
#   thumbnail  show the small preview WhatsApp sends with the message (no download, blurry)
#   off        text only, e.g. "[image] caption"
images = always

# Biggest picture size, in terminal cells.
image_width = 40
image_height = 15

# Long messages show only this many lines until you expand them (Space → Expand). 0 = never fold.
collapse_lines = 15

# Desktop notifications for new messages:
#   on       sender + message text
#   private  sender only ("New message"), no text on screen
#   off      none
notify = on

# Notification sound: watui (built-in chime; calls ring), system (desktop default), off,
# or a path to any .ogg/.wav file.
notify_sound = watui
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
	case "notify_sound":
		if value == "" {
			return fmt.Errorf("notify_sound needs a value: watui, system, off or a file path")
		}
		if value != "watui" && value != "system" && value != "off" {
			if _, err := os.Stat(value); err != nil {
				return fmt.Errorf("notify_sound file %q: %w", value, err)
			}
		}
		c.NotifySound = value
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
