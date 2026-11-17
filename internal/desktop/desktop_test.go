package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
