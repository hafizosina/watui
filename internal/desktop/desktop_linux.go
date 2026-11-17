package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var soundPlayers = [][]string{{"paplay"}, {"pw-play"}, {"aplay", "-q"}, {"mpv", "--no-video", "--really-quiet"}}

func openCommand(target string) (string, []string) { return "xdg-open", []string{target} }

// CopyText puts text on the clipboard (xclip keeps serving it in the background).
func CopyText(text string) error {
	if !has("xclip") {
		return errors.New("copy needs xclip (sudo apt install xclip)")
	}
	_, err := output(2*time.Second, text, "xclip", "-selection", "clipboard", "-i")
	return err
}

func xclipOut(target string) ([]byte, error) {
	args := []string{"-selection", "clipboard", "-o"}
	if target != "" {
		args = append(args, "-t", target)
	}
	return output(5*time.Second, "", "xclip", args...)
}

// ReadClipboard returns copied files, a copied picture, or text — in that order.
func ReadClipboard() (Clipboard, error) {
	if !has("xclip") {
		return Clipboard{}, errors.New("paste needs xclip (sudo apt install xclip)")
	}
	raw, err := xclipOut("TARGETS")
	if err != nil {
		return Clipboard{}, ErrEmpty
	}
	targets := map[string]bool{}
	for _, t := range strings.Fields(string(raw)) {
		targets[t] = true
	}
	// files copied in Nemo / Nautilus / most file managers
	for _, t := range []string{"x-special/gnome-copied-files", "text/uri-list"} {
		if targets[t] {
			if out, err := xclipOut(t); err == nil {
				if files := FileList(string(out)); len(files) > 0 {
					return Clipboard{Files: files}, nil
				}
			}
		}
	}
	// a copied picture (screenshot tool, browser "Copy image", …)
	for t, ext := range map[string]string{"image/png": ".png", "image/jpeg": ".jpg"} {
		if targets[t] {
			if data, err := xclipOut(t); err == nil && len(data) > 0 {
				return Clipboard{Image: data, ImageExt: ext}, nil
			}
		}
	}
	out, err := xclipOut("")
	if err != nil || len(out) == 0 {
		return Clipboard{}, ErrEmpty
	}
	return Clipboard{Text: string(out)}, nil
}

// CanPickFiles reports whether PickFiles can show a dialog here.
func CanPickFiles() bool {
	return has("zenity") && os.Getenv("DISPLAY")+os.Getenv("WAYLAND_DISPLAY") != ""
}

// PickFiles shows the file dialog; nil, nil means it was cancelled.
func PickFiles(title string) ([]string, error) {
	home, _ := os.UserHomeDir()
	cmd := exec.Command("zenity", "--file-selection", "--multiple", "--separator=\n", "--title="+title, "--filename="+home+"/")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("file picker: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	return lines(string(out)), nil
}

// RaiseTerminal brings the terminal window running watui to the front.
func RaiseTerminal() {
	if !has("wmctrl") {
		return
	}
	if win := os.Getenv("WINDOWID"); win != "" {
		if exec.Command("wmctrl", "-i", "-a", win).Run() == nil {
			return
		}
	}
	_ = exec.Command("wmctrl", "-x", "-a", "kitty").Run()
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
