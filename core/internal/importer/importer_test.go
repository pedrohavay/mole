package importer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mole/core/internal/bookfile"
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

// tempDir returns a new temporary directory without symbolic links in its
// path, the way Import names the files in it.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks error: %v", err)
	}
	return dir
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
	downloads := tempDir(t)
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

func TestImportFormats(t *testing.T) {
	store := openStore(t)
	mobi := testbook.MOBI{Title: "Mobi Book", Author: "Ana", HTML: "<p>Hello.</p>"}.Write(t)
	prc := filepath.Join(filepath.Dir(mobi), "Old Book.prc")
	if err := os.Rename(mobi, prc); err != nil {
		t.Fatalf("Rename error: %v", err)
	}
	tests := []struct {
		name, path, title, author, ext string
	}{
		{name: "PRC", path: prc, title: "Mobi Book", author: "Ana", ext: ".prc"},
		{name: "FB2", path: testbook.FB2{Title: "Fb Book", Author: [2]string{"Rui", "Dias"}, Sections: []string{"<p>Hi.</p>"}}.Write(t), title: "Fb Book", author: "Rui Dias", ext: ".fb2"},
		{name: "Text", path: testbook.File(t, "Plain Words.txt", []byte("Hello.\n")), title: "Plain Words", ext: ".txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Import(store, tt.path, time.Unix(1, 0))
			if err != nil {
				t.Fatalf("Import error: %v", err)
			}
			if b.Title != tt.title || b.Author != tt.author || filepath.Ext(b.Path) != tt.ext {
				t.Errorf("Import = %q by %q at %s, want %q by %q in a %s copy", b.Title, b.Author, filepath.Base(b.Path), tt.title, tt.author, tt.ext)
			}
			if full, err := store.Book(b.ID); err != nil || len(full.Cover) == 0 {
				t.Errorf("cover = %d bytes, %v; want the first page", len(full.Cover), err)
			}
		})
	}
}

func TestImportRejectsDRM(t *testing.T) {
	store := openStore(t)
	path := testbook.MOBI{Title: "Locked", HTML: "<p>Secret.</p>", Encrypted: true}.Write(t)
	if _, err := Import(store, path, time.Unix(1, 0)); !errors.Is(err, bookfile.ErrDRM) {
		t.Errorf("Import of a protected MOBI = %v, want bookfile.ErrDRM", err)
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 0 {
		t.Errorf("a refused import left %v in the Books directory", names)
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
	tests := []struct{ give, ext, want string }{
		{give: "/d/Book.epub", ext: ".epub", want: "Book-*.epub"},
		{give: "/d/a*b.epub", ext: ".epub", want: "ab-*.epub"},
		{give: "/d/.epub", ext: ".epub", want: "book-*.epub"},
		{give: "/d/" + long + ".epub", ext: ".epub", want: strings.Repeat("ç", maxNameLength/2) + "-*.epub"},
		{give: "/d/Notes.TXT", ext: ".txt", want: "Notes-*.txt"},
	}
	for _, tt := range tests {
		if got := copyName(tt.give, tt.ext); got != tt.want {
			t.Errorf("copyName(%q, %q) = %q, want %q", tt.give, tt.ext, got, tt.want)
		}
	}
}

func TestImportIdentity(t *testing.T) {
	store := openStore(t)
	dir := tempDir(t)
	original := writeBook(t, dir, "a.epub", "First")
	now := time.Unix(1_700_000_000, 0)
	b, err := Import(store, original, now)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if !b.Source.Known() {
		t.Errorf("Import recorded source %+v, want the file's size and time", b.Source)
	}

	// A changed file replaces the book's copy and metadata.
	writeBook(t, dir, "a.epub", "First, revised")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(original, later, later); err != nil {
		t.Fatalf("Chtimes error: %v", err)
	}
	changed, err := Import(store, original, now)
	if err != nil || changed.ID != b.ID || changed.Title != "First, revised" || changed.Path == b.Path {
		t.Errorf("Import of a changed file = {%d %q at %s}, %v; want book %d revised in a new copy", changed.ID, changed.Title, changed.Path, err, b.ID)
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 1 {
		t.Errorf("Books directory holds %v after a change, want one copy", names)
	}

	// A renamed file is the same book.
	renamed := filepath.Join(dir, "b.epub")
	if err := os.Rename(original, renamed); err != nil {
		t.Fatalf("Rename error: %v", err)
	}
	moved, err := Import(store, renamed, now)
	if err != nil || moved.ID != b.ID || moved.OriginalPath != renamed {
		t.Errorf("Import of a renamed file = {%d from %s}, %v; want book %d from %s", moved.ID, moved.OriginalPath, err, b.ID, renamed)
	}

	// A copy next to the original is another book.
	twin := filepath.Join(dir, "twin.epub")
	data, _ := os.ReadFile(renamed)
	if err := os.WriteFile(twin, data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if err := os.Chtimes(twin, later, later); err != nil {
		t.Fatalf("Chtimes error: %v", err)
	}
	other, err := Import(store, twin, now)
	if err != nil || other.ID == b.ID {
		t.Errorf("Import of a copy whose original still exists = %d, %v; want a new book", other.ID, err)
	}
	if books, _ := store.Books(); len(books) != 2 {
		t.Errorf("library holds %d books, want 2", len(books))
	}
	if _, err := Import(store, dir, now); err == nil {
		t.Error("Import of a folder: got nil error")
	}
}

func TestImportPDF(t *testing.T) {
	store := openStore(t)
	b, err := Import(store, testbook.SamplePDF().Write(t), time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if b.Title != "The Mole in the PDF" || b.Author != "Ada Burrow" || filepath.Ext(b.Path) != ".pdf" {
		t.Errorf("Import = %q by %q at %s; want the PDF's metadata and a .pdf copy", b.Title, b.Author, b.Path)
	}
	if !bytes.HasPrefix(b.Cover, []byte("\x89PNG")) {
		t.Error("the PDF has no cover drawn from its first page")
	}

	untitled := testbook.File(t, "Field Notes.pdf", testbook.PDF{Pages: []testbook.PDFPage{{Lines: []string{"Notes."}}}}.Bytes())
	if b, err := Import(store, untitled, time.Unix(1_700_000_000, 0)); err != nil || b.Title != "Field Notes" {
		t.Errorf("Import(untitled) = %q, %v; want the file name as title", b.Title, err)
	}

	locked := testbook.SamplePDF()
	locked.UserPassword = "secret"
	if _, err := Import(store, locked.Write(t), time.Unix(1_700_000_000, 0)); !errors.Is(err, bookfile.ErrPassword) {
		t.Errorf("Import(locked PDF) error = %v, want ErrPassword", err)
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 2 {
		t.Errorf("Books holds %v, want the two PDFs that opened", names)
	}
}
