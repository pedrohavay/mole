package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func bookIDs(books []Book) []int64 {
	out := make([]int64, len(books))
	for i, b := range books {
		out[i] = b.ID
	}
	return out
}

func TestFolders(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	f, err := s.AddFolder("/books", false, now)
	if err != nil {
		t.Fatalf("AddFolder error: %v", err)
	}
	if f.ID == 0 || f.Path != "/books" || f.Collections || !f.AddedAt.Equal(now) || !f.SyncedAt.IsZero() {
		t.Errorf("AddFolder = %+v, want /books without collections, added now, never synced", f)
	}
	again, err := s.AddFolder("/books", true, now.Add(time.Hour))
	if err != nil || again.ID != f.ID || !again.Collections || !again.AddedAt.Equal(now) {
		t.Errorf("AddFolder of a watched folder = %+v, %v; want folder %d, now making collections", again, err, f.ID)
	}
	if _, err := s.AddFolder("/articles", false, now); err != nil {
		t.Fatalf("AddFolder error: %v", err)
	}
	if err := s.FolderSynced(f.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("FolderSynced error: %v", err)
	}
	if err := s.FolderSynced(999, now); !errors.Is(err, ErrFolderNotFound) {
		t.Errorf("FolderSynced of a missing folder: error %v, want ErrFolderNotFound", err)
	}
	list, err := s.Folders()
	if err != nil || len(list) != 2 || list[0].Path != "/articles" || list[1].Path != "/books" {
		t.Fatalf("Folders = %+v, %v; want /articles and /books", list, err)
	}
	if !list[1].SyncedAt.Equal(now.Add(time.Minute)) {
		t.Errorf("SyncedAt = %v, want %v", list[1].SyncedAt, now.Add(time.Minute))
	}
	if err := s.RemoveFolder(f.ID); err != nil {
		t.Fatalf("RemoveFolder error: %v", err)
	}
	if _, err := s.Folder(f.ID); !errors.Is(err, ErrFolderNotFound) {
		t.Errorf("Folder after RemoveFolder: error %v, want ErrFolderNotFound", err)
	}
	if err := s.RemoveFolder(f.ID); !errors.Is(err, ErrFolderNotFound) {
		t.Errorf("RemoveFolder twice: error %v, want ErrFolderNotFound", err)
	}
}

func TestBooksUnderAndMissing(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	in, _ := s.Add(Book{Path: "/lib/1.epub", OriginalPath: "/books/a.epub", Title: "In"}, now)
	deep, _ := s.Add(Book{Path: "/lib/2.epub", OriginalPath: "/books/x/y/b.epub", Title: "Deep"}, now)
	s.Add(Book{Path: "/lib/3.epub", OriginalPath: "/books2/c.epub", Title: "Next door"}, now)
	s.Add(Book{Path: "/lib/4.epub", OriginalPath: "/books.epub", Title: "Same prefix"}, now)

	got, err := s.BooksUnder("/books")
	if err != nil || !reflect.DeepEqual(bookIDs(got), []int64{in.ID, deep.ID}) {
		t.Errorf("BooksUnder(/books) = %v, %v; want [%d %d]", titles(got), err, in.ID, deep.ID)
	}
	if got, _ := s.BooksUnder("/"); len(got) != 4 {
		t.Errorf("BooksUnder(/) = %v, want all four", titles(got))
	}

	// Removing a watched folder clears the marks of its books, but not of
	// books that another watched folder still holds.
	outer, _ := s.AddFolder("/books", false, now)
	s.AddFolder("/books/x", false, now)
	for _, id := range []int64{in.ID, deep.ID} {
		if err := s.SetMissing(id, true); err != nil {
			t.Fatalf("SetMissing error: %v", err)
		}
	}
	if err := s.SetMissing(999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetMissing of a missing book: error %v, want ErrNotFound", err)
	}
	if err := s.RemoveFolder(outer.ID); err != nil {
		t.Fatalf("RemoveFolder error: %v", err)
	}
	if b, _ := s.Book(in.ID); b.Missing {
		t.Errorf("book from the removed folder is still marked missing")
	}
	if b, _ := s.Book(deep.ID); !b.Missing {
		t.Errorf("book still in a watched folder lost its missing mark")
	}
}

func TestSourceStamps(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	st := Stamp{Size: 1234, ModTime: time.Unix(1_600_000_000, 123_456_789)}
	a, err := s.Add(Book{Path: "/lib/a.epub", OriginalPath: "/books/a.epub", Title: "A", Source: st}, now)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if !a.Source.Equal(st) {
		t.Errorf("Add stored source %+v, want %+v to the nanosecond", a.Source, st)
	}
	old, _ := s.Add(Book{Path: "/lib/old.epub", Title: "Old"}, now)
	if old.Source.Known() {
		t.Errorf("a book added without a stamp has source %+v, want unknown", old.Source)
	}

	if got, err := s.BooksBySource(st); err != nil || !reflect.DeepEqual(bookIDs(got), []int64{a.ID}) {
		t.Errorf("BooksBySource = %v, %v; want [%d]", titles(got), err, a.ID)
	}
	if got, err := s.BooksBySource(Stamp{}); err != nil || len(got) != 0 {
		t.Errorf("BooksBySource(unknown) = %v, %v; want none", titles(got), err)
	}

	s.SetMissing(a.ID, true)
	moved := Stamp{Size: 1234, ModTime: st.ModTime}
	if err := s.SetSource(a.ID, "/books/renamed.epub", moved); err != nil {
		t.Fatalf("SetSource error: %v", err)
	}
	got, _ := s.Book(a.ID)
	if got.OriginalPath != "/books/renamed.epub" || got.Missing || !got.Source.Equal(moved) {
		t.Errorf("after SetSource: from %q, missing %v, source %+v; want the new path, unmarked", got.OriginalPath, got.Missing, got.Source)
	}
	if err := s.SetSource(999, "/x", moved); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetSource of a missing book: error %v, want ErrNotFound", err)
	}
}

func TestReplace(t *testing.T) {
	s := openTestStore(t)
	if err := os.MkdirAll(s.BooksDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	first := filepath.Join(s.BooksDir(), "a-1.epub")
	second := filepath.Join(s.BooksDir(), "a-2.epub")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("epub"), 0o644); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}
	}
	now := time.Unix(1_700_000_000, 0)
	a, _ := s.Add(Book{Path: first, OriginalPath: "/books/a.epub", Title: "A", Cover: []byte{1}}, now)
	c, _ := s.CreateCollection("Kept", now)
	s.AddToCollection(c.ID, a.ID, now)
	place := Position{Chapter: 2, Offset: 30, ChapterFraction: 0.5, Fraction: 0.2}
	s.SavePosition(a.ID, place, now)
	s.SetMissing(a.ID, true)

	st := Stamp{Size: 99, ModTime: time.Unix(1_700_000_100, 0)}
	got, err := s.Replace(a.ID, Book{Path: second, OriginalPath: "/books/a.epub", Title: "A, revised", Author: "Someone", Cover: []byte{2}, Source: st})
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if got.ID != a.ID || got.Path != second || got.Title != "A, revised" || got.Author != "Someone" ||
		string(got.Cover) != "\x02" || !got.Source.Equal(st) || got.Missing {
		t.Errorf("Replace = %+v, want the new copy, metadata and stamp, unmarked", got)
	}
	if got.Position != place || !reflect.DeepEqual(got.Collections, []int64{c.ID}) {
		t.Errorf("Replace lost the place %+v or the collections %v", got.Position, got.Collections)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the previous copy still exists after Replace: %v", err)
	}
	if _, err := s.Replace(999, Book{Path: "/x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Replace of a missing book: error %v, want ErrNotFound", err)
	}

	// A previous copy that cannot be deleted does not undo the replacement,
	// and a failed replacement deletes the new copy, never the current one.
	stuck := filepath.Join(s.BooksDir(), "stuck")
	if err := os.MkdirAll(filepath.Join(stuck, "inside"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	b, _ := s.Add(Book{Path: stuck, OriginalPath: "/books/b.epub", Title: "B"}, now)
	third := filepath.Join(s.BooksDir(), "b-3.epub")
	os.WriteFile(third, []byte("epub"), 0o644)
	if got, err := s.Replace(b.ID, Book{Path: third, OriginalPath: "/books/b.epub", Title: "B", Source: st}); err != nil || got.Path != third {
		t.Errorf("Replace with an old copy that cannot be deleted = %q, %v; want the new copy", got.Path, err)
	}
	orphan := filepath.Join(s.BooksDir(), "b-4.epub")
	os.WriteFile(orphan, []byte("epub"), 0o644)
	if _, err := s.Replace(999, Book{Path: orphan}); err == nil {
		t.Error("Replace of a missing book: got nil error")
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a failed Replace left its new copy: %v", err)
	}
	if _, err := os.Stat(third); err != nil {
		t.Errorf("a failed Replace of another book touched this book's copy: %v", err)
	}
}

func TestSetSourceMovesOldBooks(t *testing.T) {
	// A book imported before copies were kept reads its original file.
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	old, _ := s.Add(Book{Path: "/books/old.epub", Title: "Old"}, now)
	copied, _ := s.Add(Book{Path: "/lib/Books/c.epub", OriginalPath: "/books/c.epub", Title: "Copied"}, now)
	st := Stamp{Size: 1, ModTime: now}
	for _, id := range []int64{old.ID, copied.ID} {
		b, _ := s.Book(id)
		if err := s.SetSource(id, "/moved/"+filepath.Base(b.OriginalPath), st); err != nil {
			t.Fatalf("SetSource error: %v", err)
		}
	}
	if b, _ := s.Book(old.ID); b.Path != "/moved/old.epub" || b.OriginalPath != "/moved/old.epub" {
		t.Errorf("old book after a move reads %q from %q, want both /moved/old.epub", b.Path, b.OriginalPath)
	}
	if b, _ := s.Book(copied.ID); b.Path != "/lib/Books/c.epub" || b.OriginalPath != "/moved/c.epub" {
		t.Errorf("copied book after a move reads %q from %q, want its copy from /moved/c.epub", b.Path, b.OriginalPath)
	}
}

func TestWithin(t *testing.T) {
	tests := []struct {
		path, dir string
		want      bool
	}{
		{path: "/books/a.epub", dir: "/books", want: true},
		{path: "/books/x/a.epub", dir: "/books/", want: true},
		{path: "/books2/a.epub", dir: "/books", want: false},
		{path: "/books", dir: "/books", want: false},
		{path: "/a.epub", dir: "/", want: true},
	}
	for _, tt := range tests {
		if got := Within(tt.path, tt.dir); got != tt.want {
			t.Errorf("Within(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.want)
		}
	}
}
