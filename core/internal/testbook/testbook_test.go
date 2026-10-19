package testbook

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

// TestSamplePDF checks that the sample PDF is one. With MOLE_PDF_DIR set to
// a folder, it also writes the PDFs to try in the app there: sample.pdf,
// locked.pdf, which needs the password "mole", and long.pdf, of 400 pages.
func TestSamplePDF(t *testing.T) {
	data := SamplePDF().Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) || !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Errorf("SamplePDF().Bytes() starts with %q and ends with %q, want a PDF", data[:8], data[len(data)-6:])
	}
	dir := os.Getenv("MOLE_PDF_DIR")
	if dir == "" {
		return
	}
	locked := SamplePDF()
	locked.Title, locked.UserPassword = "Locked", "mole"
	long := PDF{Title: "A Long PDF", Author: "Ada Burrow"}
	for page := range 400 {
		lines := []string{fmt.Sprintf("Page %d", page+1), ""}
		for line := range 40 {
			lines = append(lines, fmt.Sprintf("Line %d of page %d of a long book of fixed pages.", line+1, page+1))
		}
		long.Pages = append(long.Pages, PDFPage{Lines: lines})
		if page%20 == 0 {
			long.Outline = append(long.Outline, PDFEntry{Title: fmt.Sprintf("Part %d", page/20+1), Page: page})
		}
	}
	for name, p := range map[string]PDF{"sample.pdf": SamplePDF(), "locked.pdf": locked, "long.pdf": long} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, p.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}
