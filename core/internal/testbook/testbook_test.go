package testbook

import (
	"archive/zip"
	"os"
	"testing"
)

// TestColors checks that the color book is a readable archive. With
// MOLE_COLOR_EPUB set to a path, it also writes the book there, to open it
// in the app.
func TestColors(t *testing.T) {
	path := Colors(t).Write(t)
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer r.Close()
	want := map[string]bool{"OEBPS/photo.jpg": false, "OEBPS/diagram.png": false, "OEBPS/lines.png": false, "OEBPS/drawing.svg": false}
	for _, f := range r.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("Colors book has no %s", name)
		}
	}
	if out := os.Getenv("MOLE_COLOR_EPUB"); out != "" {
		Colors(t).WriteTo(t, out)
		t.Logf("wrote %s", out)
	}
}
