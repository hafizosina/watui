// Package desktop wraps what watui needs from the desktop around it: opening
// files and links, the clipboard, a file dialog, raising the terminal window
// and playing a sound. Each OS has its own file (desktop_linux.go,
// desktop_darwin.go); the rest of watui only calls this package.
package desktop

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Clipboard is what a paste found. At most one of Files, Image or Text is set.
type Clipboard struct {
	Files    []string // copied files (file manager) — absolute paths
	Image    []byte   // a copied picture
	ImageExt string   // ".png" or ".jpg"
	Text     string
}

// ErrEmpty means there was nothing on the clipboard to paste.
var ErrEmpty = errors.New("clipboard is empty")

// Open opens a file or URL with the default app and reaps the process when it
// exits, so no zombies pile up.
func Open(target string) error {
	name, args := openCommand(target)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Play plays a sound file in the background with the first player found.
func Play(path string) {
	if path == "" {
		return
	}
	for _, player := range soundPlayers {
		if p, err := exec.LookPath(player[0]); err == nil {
			cmd := exec.Command(p, append(player[1:], path)...)
			if cmd.Start() == nil {
				go func() { _ = cmd.Wait() }()
			}
			return
		}
	}
}

// FileList reads file:// URIs or absolute paths, one per line (as file
// managers and terminals copy them). Any line that isn't an existing file
// makes the whole thing plain text, and nil is returned.
func FileList(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "copy" || line == "cut" {
			continue
		}
		path := line
		if strings.HasPrefix(line, "file://") {
			u, err := url.Parse(line)
			if err != nil {
				return nil
			}
			path = u.Path
		} else if strings.HasPrefix(line, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, line[2:])
			}
		}
		if !filepath.IsAbs(path) {
			return nil
		}
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			return nil
		}
		out = append(out, path)
	}
	return out
}

func command(ctx context.Context, stdin string, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	// a tool may leave a background child holding our pipes (xclip does, to
	// serve the clipboard): don't wait for it past the timeout
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

// output runs a tool and returns what it printed.
func output(timeout time.Duration, stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return command(ctx, stdin, name, args...).Output()
}

// run runs a tool without reading its output. Use it for tools that fork a
// background child (xclip -i): reading stdout would wait for that child,
// which lives until something else is copied — that froze Compose on dd.
func run(timeout time.Duration, stdin string, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return command(ctx, stdin, name, args...).Run()
}

func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
