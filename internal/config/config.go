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
}

func Default() Config {
	return Config{
		Images:      ImagesAlways,
		ImageWidth:  40,
		ImageHeight: 15,
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
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}
