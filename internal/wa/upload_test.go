package wa

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSendKind(t *testing.T) {
	dir := t.TempDir()
	var pic bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	img.Set(1, 1, color.White)
	if err := png.Encode(&pic, img); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"shot.png":   pic.Bytes(),
		"noext":      pic.Bytes(),
		"song.mp3":   []byte("ID3"),
		"clip.mp4":   []byte("x"),
		"report.pdf": []byte("%PDF-1.4"),
		"anim.gif":   []byte("GIF89a"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		"shot.png": MediaImage, "noext": MediaImage, "song.mp3": MediaAudio,
		"clip.mp4": MediaVideo, "report.pdf": MediaDoc, "anim.gif": MediaDoc,
	}
	for name, kind := range want {
		got, _, err := SendKind(filepath.Join(dir, name), false)
		if err != nil || got != kind {
			t.Errorf("%s: got %q %v, want %q", name, got, err, kind)
		}
	}
	if got, _, _ := SendKind(filepath.Join(dir, "shot.png"), true); got != MediaDoc {
		t.Errorf("asDocument should force a document, got %q", got)
	}
	if _, _, err := SendKind(dir, false); err == nil {
		t.Error("a folder should be refused")
	}
	if th := thumbnail(img); len(th) == 0 {
		t.Error("thumbnail should encode")
	}
}
