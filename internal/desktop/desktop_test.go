package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileList(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a b.pdf"), filepath.Join(dir, "c.png")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	nemo := "copy\nfile://" + strings.ReplaceAll(a, " ", "%20") + "\r\nfile://" + b
	if got := FileList(nemo); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("file manager copy: %q", got)
	}
	for _, text := range []string{"hello", a + "\nnot a path", dir, "relative.txt"} {
		if got := FileList(text); got != nil {
			t.Errorf("%q should be plain text, got %q", text, got)
		}
	}
}

// xclip -i forks a child that holds the clipboard (and our pipes) until
// something else is copied; waiting for it froze Compose on dd.
func TestRunDoesNotWaitForForkedChild(t *testing.T) {
	start := time.Now()
	if err := run(2*time.Second, "hello", "sh", "-c", "cat >/dev/null; sleep 5 & exit 0"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("run waited %v for a background child", d)
	}
	start = time.Now()
	_, _ = output(2*time.Second, "", "sh", "-c", "sleep 5 & echo hi")
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("output waited %v for a background child", d)
	}
}
