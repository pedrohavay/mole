package mupdf

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

// The fonts below come with every macOS 14 install.

func TestFindFont(t *testing.T) {
	tests := []struct {
		family   string
		style    FontStyle
		wantFile string // base name of the file
	}{
		{family: "Georgia", wantFile: "Georgia.ttf"},
		{family: "georgia", style: FontStyle{Bold: true}, wantFile: "Georgia Bold.ttf"},
		{family: "Georgia", style: FontStyle{Bold: true, Italic: true}, wantFile: "Georgia Bold Italic.ttf"},
		{family: "Palatino", wantFile: "Palatino.ttc"},
		{family: "Helvetica Neue", style: FontStyle{Bold: true}, wantFile: "HelveticaNeue.ttc"},
		// New York is a hidden system family, ".New York", and a variable font.
		{family: "New York", wantFile: "NewYork.ttf"},
		{family: "New York", style: FontStyle{Italic: true}, wantFile: "NewYorkItalic.ttf"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%+v", tt.family, tt.style), func(t *testing.T) {
			path, index, ok := FindFont(tt.family, tt.style)
			if !ok || filepath.Base(path) != tt.wantFile {
				t.Fatalf("FindFont(%q, %+v) = %q, %d, %v; want %s", tt.family, tt.style, path, index, ok, tt.wantFile)
			}
			if !strings.HasSuffix(path, ".ttc") && index != 0 {
				t.Errorf("FindFont(%q, %+v) index = %d, want 0 in a file of one font", tt.family, tt.style, index)
			}
		})
	}

	// The faces of a collection are told apart.
	faces := map[int]FontStyle{}
	for _, style := range []FontStyle{{}, {Bold: true}, {Italic: true}, {Bold: true, Italic: true}} {
		_, index, ok := FindFont("Palatino", style)
		if !ok {
			t.Fatalf("FindFont(Palatino, %+v) failed", style)
		}
		if other, seen := faces[index]; seen {
			t.Errorf("FindFont(Palatino, %+v) = face %d, the face of %+v too", style, index, other)
		}
		faces[index] = style
	}

	for _, family := range []string{"serif", "Sans-Serif", "monospace", "", "No Such Family Anywhere"} {
		if path, index, ok := FindFont(family, FontStyle{}); ok {
			t.Errorf("FindFont(%q) = %q, %d, true; want false", family, path, index)
		}
	}
}

// TestSystemFontLayout checks that a book naming installed fonts is laid out
// with them: each paragraph sets the same word in another font, and where its
// letters fall tells which font MuPDF used.
func TestSystemFontLayout(t *testing.T) {
	const word = "Hamburgefonstiv"
	fonts := []string{
		"font-family: 'No Such Family Anywhere'", // MuPDF's own serif
		"font-family: Georgia",
		"font-family: Palatino",
		"font-family: Palatino; font-weight: bold",
		"font-family: Palatino; font-style: italic",
		"font-family: 'Avenir Next'",
		"font-family: 'New York'",
	}
	var body strings.Builder
	for _, f := range fonts {
		fmt.Fprintf(&body, `<p style="%s">%s</p>`, f, word)
	}
	doc, err := Open(testbook.Chapters(t, body.String()), "")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(doc.Close)
	if _, err := doc.Layout(1200, 1600, 20); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	chars, err := doc.PageChars(0)
	if err != nil {
		t.Fatalf("PageChars error: %v", err)
	}
	// The right edge of each letter of the word, by paragraph.
	edges := map[int][]float64{}
	for _, c := range chars {
		edges[c.Block] = append(edges[c.Block], c.Box.X1)
	}
	if len(edges) != len(fonts) {
		t.Fatalf("PageChars found %d paragraphs, want %d", len(edges), len(fonts))
	}
	for i := range fonts {
		for j := range i {
			if !differ(edges[i], edges[j]) {
				t.Errorf("letters of %q at %.1f in %q and at %.1f in %q; want different fonts", word, edges[i], fonts[i], edges[j], fonts[j])
			}
		}
	}
}

// differ reports whether two lists of positions differ by half a point
// anywhere.
func differ(a, b []float64) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if d := a[i] - b[i]; d < -0.5 || d > 0.5 {
			return true
		}
	}
	return false
}
