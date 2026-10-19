package library

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddListRemove(t *testing.T) {
	s := openTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	first, err := s.Add(Book{Path: "/books/a.epub", Title: "A", Cover: []byte{1, 2}}, base)
	if err != nil {
		t.Fatalf("Add(a) error: %v", err)
	}
	second, err := s.Add(Book{Path: "/books/b.epub", Title: "B"}, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("Add(b) error: %v", err)
	}
	again, err := s.Add(Book{Path: "/books/a.epub", Title: "renamed"}, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("Add(a) again error: %v", err)
	}
	if again.ID != first.ID || again.Title != "A" {
		t.Errorf("Add of an existing path = {%d %q}, want the existing {%d %q}", again.ID, again.Title, first.ID, "A")
	}

	books, err := s.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	if len(books) != 2 || books[0].ID != second.ID || books[1].ID != first.ID {
		t.Fatalf("Books = %v, want [b a] (most recently added first)", titles(books))
	}
	if books[1].Cover != nil || !books[1].AddedAt.Equal(base) || books[1].OriginalPath != "/books/a.epub" {
		t.Errorf("listed book a = cover %v added %v from %q, want no cover, added %v from its path", books[1].Cover, books[1].AddedAt, books[1].OriginalPath, base)
	}
	if b, err := s.Book(first.ID); err != nil || string(b.Cover) != "\x01\x02" {
		t.Errorf("Book(a) cover = %v, %v; want [1 2]", b.Cover, err)
	}

	if err := s.Remove(first.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if err := s.Remove(first.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Remove of a removed book: error %v, want ErrNotFound", err)
	}
	if _, err := s.Book(first.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Book of a removed book: error %v, want ErrNotFound", err)
	}
}

func TestSavePositionOrdersByLastOpened(t *testing.T) {
	s := openTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	a, _ := s.Add(Book{Path: "/a.epub", Title: "A"}, base)
	b, _ := s.Add(Book{Path: "/b.epub", Title: "B"}, base.Add(time.Minute))

	if books, _ := s.Books(); books[0].Position.Offset != -1 {
		t.Errorf("new book position offset = %d, want -1 (unknown)", books[0].Position.Offset)
	}
	want := Position{Chapter: 9, Offset: 1234, ChapterFraction: 0.75, Fraction: 0.08}
	if err := s.SavePosition(a.ID, want, base.Add(time.Hour)); err != nil {
		t.Fatalf("SavePosition error: %v", err)
	}
	books, err := s.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	if books[0].ID != a.ID || books[1].ID != b.ID {
		t.Fatalf("Books = %v, want [A B] (last opened first)", titles(books))
	}
	if books[0].Position != want || !books[0].OpenedAt.Equal(base.Add(time.Hour)) {
		t.Errorf("Books()[0] position %+v opened %v, want %+v opened %v",
			books[0].Position, books[0].OpenedAt, want, base.Add(time.Hour))
	}
	if !books[1].OpenedAt.IsZero() {
		t.Errorf("never opened book has OpenedAt %v, want zero", books[1].OpenedAt)
	}
	if err := s.SavePosition(999, want, base); !errors.Is(err, ErrNotFound) {
		t.Errorf("SavePosition of a missing book: error %v, want ErrNotFound", err)
	}
}

func TestSettings(t *testing.T) {
	s := openTestStore(t)
	got, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings error: %v", err)
	}
	if got != DefaultSettings() {
		t.Errorf("Settings before saving = %+v, want defaults %+v", got, DefaultSettings())
	}
	want := Settings{Level: 0.31, EDR: false, Tint: 2, Gamma: 1.5, FontSize: 24, Spread: 2, Accent: "#3D7BFF", Images: 1}
	for range 2 { // the second save updates the existing row
		if err := s.SaveSettings(want); err != nil {
			t.Fatalf("SaveSettings error: %v", err)
		}
	}
	if got, _ := s.Settings(); got != want {
		t.Errorf("Settings after saving = %+v, want %+v", got, want)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	if _, err := s.Add(Book{Path: "/a.epub", Title: "A"}, time.Unix(1, 0)); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	s.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error: %v", err)
	}
	defer reopened.Close()
	books, err := reopened.Books()
	if err != nil || len(books) != 1 {
		t.Errorf("Books after reopening = %v, %v; want one book", titles(books), err)
	}
}

func titles(books []Book) []string {
	out := make([]string, len(books))
	for i, b := range books {
		out[i] = b.Title
	}
	return out
}

func TestAnnotations(t *testing.T) {
	s := openTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	book, _ := s.Add(Book{Path: "/a.epub", Title: "A"}, base)
	other, _ := s.Add(Book{Path: "/b.epub", Title: "B"}, base)

	add := func(a Annotation) Annotation {
		t.Helper()
		got, err := s.AddAnnotation(a, base)
		if err != nil {
			t.Fatalf("AddAnnotation(%+v) error: %v", a, err)
		}
		return got
	}
	late := add(Annotation{BookID: book.ID, Kind: KindHighlight, Start: Anchor{3, 10}, End: Anchor{4, 2}, Color: 2, Text: "late"})
	mark := add(Annotation{BookID: book.ID, Kind: KindBookmark, Start: Anchor{1, 500}, End: Anchor{1, 500}, Text: "mark"})
	early := add(Annotation{BookID: book.ID, Kind: KindHighlight, Start: Anchor{1, 20}, End: Anchor{1, 40}, Color: 1, Note: "why", Text: "early"})
	add(Annotation{BookID: other.ID, Kind: KindBookmark, Text: "other book"})

	got, err := s.Annotations(book.ID)
	if err != nil {
		t.Fatalf("Annotations error: %v", err)
	}
	want := []Annotation{early, mark, late}
	if len(got) != len(want) {
		t.Fatalf("Annotations = %d items, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Annotations()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if !early.CreatedAt.Equal(base) || !early.UpdatedAt.Equal(base) || early.ID == 0 {
		t.Errorf("added annotation = %+v, want an ID and times %v", early, base)
	}

	if err := s.UpdateAnnotation(early.ID, 4, "", base.Add(time.Hour)); err != nil {
		t.Fatalf("UpdateAnnotation error: %v", err)
	}
	got, _ = s.Annotations(book.ID)
	if got[0].Color != 4 || got[0].Note != "" || !got[0].UpdatedAt.Equal(base.Add(time.Hour)) || !got[0].CreatedAt.Equal(base) {
		t.Errorf("after UpdateAnnotation: %+v, want color 4, no note, updated an hour later", got[0])
	}
	if err := s.UpdateAnnotation(999, 1, "x", base); !errors.Is(err, ErrAnnotationNotFound) {
		t.Errorf("UpdateAnnotation of a missing annotation: error %v, want ErrAnnotationNotFound", err)
	}

	if err := s.RemoveAnnotation(mark.ID); err != nil {
		t.Fatalf("RemoveAnnotation error: %v", err)
	}
	if err := s.RemoveAnnotation(mark.ID); !errors.Is(err, ErrAnnotationNotFound) {
		t.Errorf("RemoveAnnotation twice: error %v, want ErrAnnotationNotFound", err)
	}
	if got, _ := s.Annotations(book.ID); len(got) != 2 {
		t.Errorf("Annotations after removing one = %d items, want 2", len(got))
	}

	// Removing a book removes its annotations and no others.
	if err := s.Remove(book.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if got, err := s.Annotations(book.ID); err != nil || len(got) != 0 {
		t.Errorf("Annotations of a removed book = %v, %v; want none", got, err)
	}
	if got, _ := s.Annotations(other.ID); len(got) != 1 {
		t.Errorf("Annotations of the other book = %d items, want 1", len(got))
	}
}

func TestAddAnnotationRejects(t *testing.T) {
	s := openTestStore(t)
	book, _ := s.Add(Book{Path: "/a.epub", Title: "A"}, time.Unix(1, 0))
	tests := []struct {
		name string
		give Annotation
	}{
		{name: "UnknownKind", give: Annotation{BookID: book.ID, Kind: "note"}},
		{name: "Reversed", give: Annotation{BookID: book.ID, Kind: KindHighlight, Start: Anchor{2, 5}, End: Anchor{2, 4}}},
		{name: "ReversedChapters", give: Annotation{BookID: book.ID, Kind: KindHighlight, Start: Anchor{3, 0}, End: Anchor{2, 9}}},
		{name: "NegativeOffset", give: Annotation{BookID: book.ID, Kind: KindHighlight, Start: Anchor{0, -1}, End: Anchor{0, 2}}},
		{name: "BookmarkWithRange", give: Annotation{BookID: book.ID, Kind: KindBookmark, Start: Anchor{0, 1}, End: Anchor{0, 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.AddAnnotation(tt.give, time.Unix(1, 0)); err == nil {
				t.Errorf("AddAnnotation(%+v): got nil error", tt.give)
			}
		})
	}
	if _, err := s.AddAnnotation(Annotation{BookID: 999, Kind: KindBookmark}, time.Unix(1, 0)); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddAnnotation to a missing book: error %v, want ErrNotFound", err)
	}
}

// TestMigrateKeepsData opens a database made by the schema before
// annotations (user_version 2) and checks that its data survives.
func TestMigrateKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("sql.Open error: %v", err)
	}
	for _, m := range migrations[:2] {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("old migration error: %v", err)
		}
	}
	_, err = db.Exec(`
		PRAGMA user_version = 2;
		INSERT INTO books (path, title, author, added_at, opened_at, chapter, chapter_fraction, fraction)
		VALUES ('/old.epub', 'Old', 'Someone', 100, 200, 7, 0.5, 0.25);
		INSERT INTO reader_settings (id, level, edr, tint, gamma, font_size, spread, accent)
		VALUES (1, 0.7, 0, 3, 1.8, 19, 1, '#112233');`)
	if err != nil {
		t.Fatalf("seed old database error: %v", err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open of an old database error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	books, err := s.Books()
	if err != nil || len(books) != 1 {
		t.Fatalf("Books after migrating = %v, %v; want the old book", titles(books), err)
	}
	b := books[0]
	wantPos := Position{Chapter: 7, Offset: -1, ChapterFraction: 0.5, Fraction: 0.25}
	if b.Title != "Old" || b.Author != "Someone" || b.Position != wantPos || b.OpenedAt.Unix() != 200 {
		t.Errorf("migrated book = %+v, want Old by Someone at %+v", b, wantPos)
	}
	want := Settings{Level: 0.7, EDR: false, Tint: 3, Gamma: 1.8, FontSize: 19, Spread: 1, Accent: "#112233"}
	if got, err := s.Settings(); err != nil || got != want {
		t.Errorf("Settings after migrating = %+v, %v; want %+v", got, err, want)
	}
	if _, err := s.AddAnnotation(Annotation{BookID: b.ID, Kind: KindBookmark}, time.Unix(300, 0)); err != nil {
		t.Errorf("AddAnnotation after migrating error: %v", err)
	}
	if b.OriginalPath != "/old.epub" || b.Position.Offset != -1 {
		t.Errorf("migrated book from %q at offset %d, want from its own path at offset -1", b.OriginalPath, b.Position.Offset)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations) {
		t.Errorf("user_version after migrating = %d, %v; want %d", version, err, len(migrations))
	}
}

func TestCopiesAndOriginalPaths(t *testing.T) {
	s := openTestStore(t)
	if err := os.MkdirAll(s.BooksDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	copyPath := filepath.Join(s.BooksDir(), "a-1.epub")
	outside := filepath.Join(t.TempDir(), "outside.epub")
	for _, p := range []string{copyPath, outside} {
		if err := os.WriteFile(p, []byte("epub"), 0o644); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}
	}
	base := time.Unix(1_700_000_000, 0)
	a, err := s.Add(Book{Path: copyPath, OriginalPath: "/downloads/a.epub", Title: "A"}, base)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if got, err := s.BookByOriginalPath("/downloads/a.epub"); err != nil || got.ID != a.ID || got.Path != copyPath {
		t.Errorf("BookByOriginalPath = %+v, %v; want book %d at %s", got, err, a.ID, copyPath)
	}
	if _, err := s.BookByOriginalPath("/downloads/other.epub"); !errors.Is(err, ErrNotFound) {
		t.Errorf("BookByOriginalPath of an unknown file: error %v, want ErrNotFound", err)
	}
	again, err := s.Add(Book{Path: filepath.Join(s.BooksDir(), "a-2.epub"), OriginalPath: "/downloads/a.epub", Title: "A2"}, base)
	if err != nil || again.ID != a.ID {
		t.Errorf("Add of the same original path = %d, %v; want the existing book %d", again.ID, err, a.ID)
	}
	old, err := s.Add(Book{Path: outside, Title: "Old"}, base)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}

	// Removing deletes the library's copy, never a file elsewhere.
	if err := s.Remove(a.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if _, err := os.Stat(copyPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the copy of a removed book still exists: %v", err)
	}
	if err := s.Remove(old.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("removing a book deleted a file outside the library: %v", err)
	}
}
