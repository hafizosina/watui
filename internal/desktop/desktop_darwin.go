package desktop

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var soundPlayers = [][]string{{"afplay"}, {"mpv", "--no-video", "--really-quiet"}}

func openCommand(target string) (string, []string) { return "open", []string{target} }

// CopyText puts text on the clipboard.
func CopyText(text string) error {
	_, err := output(2*time.Second, text, "pbcopy")
	return err
}

// pasteboardJS reads copied Finder files or a copied picture from the
// general pasteboard (JavaScript for Automation, built into macOS).
const pasteboardJS = `
ObjC.import('AppKit');
function run() {
  var pb = $.NSPasteboard.generalPasteboard;
  var files = [];
  var urls = pb.readObjectsForClassesOptions($([$.NSURL]), $({}));
  if (urls && !urls.isNil()) {
    for (var i = 0; i < urls.count; i++) {
      var u = urls.objectAtIndex(i);
      if (u.isFileURL) files.push(u.path.js);
    }
  }
  if (files.length) return JSON.stringify({files: files});
  var kinds = [['public.png', '.png'], ['public.jpeg', '.jpg'], ['public.tiff', 'tiff']];
  for (var k = 0; k < kinds.length; k++) {
    var d = pb.dataForType(kinds[k][0]);
    if (d.isNil() || d.length == 0) continue;
    var ext = kinds[k][1];
    if (ext == 'tiff') {
      var rep = $.NSBitmapImageRep.imageRepWithData(d);
      if (rep.isNil()) continue;
      d = rep.representationUsingTypeProperties(4, $({})); // 4 = PNG
      ext = '.png';
    }
    return JSON.stringify({image: d.base64EncodedStringWithOptions(0).js, ext: ext});
  }
  return '{}';
}`

// ReadClipboard returns copied files, a copied picture, or text — in that order.
func ReadClipboard() (Clipboard, error) {
	if out, err := output(10*time.Second, "", "osascript", "-l", "JavaScript", "-e", pasteboardJS); err == nil {
		var got struct {
			Files []string `json:"files"`
			Image string   `json:"image"`
			Ext   string   `json:"ext"`
		}
		if json.Unmarshal(out, &got) == nil {
			if len(got.Files) > 0 {
				return Clipboard{Files: got.Files}, nil
			}
			if got.Image != "" {
				if data, err := base64.StdEncoding.DecodeString(got.Image); err == nil && len(data) > 0 {
					return Clipboard{Image: data, ImageExt: got.Ext}, nil
				}
			}
		}
	}
	out, err := output(5*time.Second, "", "pbpaste")
	if err != nil || len(out) == 0 {
		return Clipboard{}, ErrEmpty
	}
	return Clipboard{Text: string(out)}, nil
}

// CanPickFiles reports whether PickFiles can show a dialog here.
func CanPickFiles() bool { return has("osascript") }

// PickFiles shows the macOS file dialog; nil, nil means it was cancelled.
func PickFiles(title string) ([]string, error) {
	cmd := exec.Command("osascript",
		"-e", "on run argv",
		"-e", "set picked to choose file with prompt (item 1 of argv) with multiple selections allowed",
		"-e", `set out to ""`,
		"-e", "repeat with f in picked",
		"-e", "set out to out & POSIX path of f & linefeed",
		"-e", "end repeat",
		"-e", "return out",
		"-e", "end run",
		"--", title)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "-128") { // User canceled.
			return nil, nil
		}
		return nil, fmt.Errorf("file picker: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// RaiseTerminal brings the terminal app running watui to the front.
func RaiseTerminal() {
	app := map[string]string{"Apple_Terminal": "Terminal", "iTerm.app": "iTerm", "WezTerm": "WezTerm"}[os.Getenv("TERM_PROGRAM")]
	if os.Getenv("KITTY_PID") != "" || strings.Contains(os.Getenv("TERM"), "kitty") {
		app = "kitty"
	}
	if app != "" {
		_ = exec.Command("open", "-a", app).Run()
	}
}
