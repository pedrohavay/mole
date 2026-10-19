package mupdf

import (
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

func TestMemoryLimit(t *testing.T) {
	// About 4 MB of text, which takes far more than 8 MiB once laid out.
	path := testbook.Chapters(t, "<p>Small.</p>", strings.Repeat("<p>"+strings.Repeat("word ", 200)+"</p>", 4000))
	doc, err := open(path, "", 8<<20)
	if err != nil {
		t.Fatalf("open with an 8 MiB limit error: %v", err)
	}
	defer doc.Close()
	_, err = doc.Layout(400, 600, 12)
	if err == nil {
		t.Error("Layout of a book larger than the memory limit: got nil error, want error")
	}
	t.Logf("Layout over the limit: %v", err)

	// The same book lays out under the real limit.
	doc, err = Open(path, "")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer doc.Close()
	if pages, err := doc.Layout(400, 600, 12); err != nil || pages < 100 {
		t.Errorf("Layout = %d pages, %v; want the book", pages, err)
	}
}
