package desktop

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// The pasteboard script must at least run and print JSON on a real Mac.
func TestPasteboardScriptRuns(t *testing.T) {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", pasteboardJS).CombinedOutput()
	if err != nil {
		t.Fatalf("pasteboard script failed: %v\n%s", err, out)
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("pasteboard script printed %q, not JSON: %v", out, err)
	}
}

func TestClipboardTextRoundTrip(t *testing.T) {
	if err := CopyText("watui clipboard test"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadClipboard()
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "watui clipboard test" {
		t.Errorf("read back %+v", got)
	}
}
