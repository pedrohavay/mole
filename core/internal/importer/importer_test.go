package importer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mole/core/internal/library"
	"mole/core/internal/testbook"
)

func openStore(t *testing.T) *library.Store {
	t.Helper()
	s, err := library.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("library.Open error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func booksIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir error: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestImport(t *testing.T) {
	store := openStore(t)
	downloads := t.TempDir()
	original := testbook.Book{Title: "Imported", Chapters: []string{"<p>Hello.</p>"}}.
		WriteTo(t, filepath.Join(downloads, strings.Repeat("long name ", 20)+".epub"))
	now := time.Unix(1_700_000_000, 0)

	b, err := Import(store, original, now)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if b.Title != "Imported" || b.OriginalPath != original || filepath.Dir(b.Path) != store.BooksDir() {
		t.Errorf("Import = %q from %q at %q, want the title, the original path and a copy in %q", b.Title, b.OriginalPath, b.Path, store.BooksDir())
	}
	if len(filepath.Base(b.Path)) > maxNameLength+20 || filepath.Ext(b.Path) != ".epub" {
		t.Errorf("copy name %q, want a shortened .epub name", filepath.Base(b.Path))
	}
	want, _ := os.ReadFile(original)
	if got, err := os.ReadFile(b.Path); err != nil || !bytes.Equal(got, want) {
		t.Errorf("the copy differs from the original (%v)", err)
	}
	if full, err := store.Book(b.ID); err != nil || !bytes.HasPrefix(full.Cover, []byte("\x89PNG")) {
		t.Errorf("cover of a book without one = %d bytes, %v; want the first page as PNG", len(full.Cover), err)
	}

	// Importing the same file again returns the book without reading the
	// file, and makes no second copy, even after the original is gone.
	if err := os.Remove(original); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	again, err := Import(store, original, now.Add(time.Hour))
	if err != nil || again.ID != b.ID {
		t.Errorf("Import of a known path = %d, %v; want book %d", again.ID, err, b.ID)
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 1 {
		t.Errorf("Books directory holds %v, want one copy", names)
	}

	if err := store.Remove(b.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 0 {
		t.Errorf("Books directory after removing the book holds %v, want nothing", names)
	}
}

func TestImportRejectsNonEPUB(t *testing.T) {
	store := openStore(t)
	path := filepath.Join(t.TempDir(), "notes.epub")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if _, err := Import(store, path, time.Unix(1, 0)); err == nil {
		t.Error("Import of a text file: got nil error, want error")
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 0 {
		t.Errorf("a failed import left %v in the Books directory", names)
	}
	if books, _ := store.Books(); len(books) != 0 {
		t.Errorf("a failed import added %d books", len(books))
	}
}

func TestCopyName(t *testing.T) {
	long := strings.Repeat("ç", 80) // 160 bytes
	tests := []struct{ give, want string }{
		{give: "/d/Book.epub", want: "Book-*.epub"},
		{give: "/d/a*b.epub", want: "ab-*.epub"},
		{give: "/d/.epub", want: "book-*.epub"},
		{give: "/d/" + long + ".epub", want: strings.Repeat("ç", maxNameLength/2) + "-*.epub"},
	}
	for _, tt := range tests {
		if got := copyName(tt.give); got != tt.want {
			t.Errorf("copyName(%q) = %q, want %q", tt.give, got, tt.want)
		}
	}
}
