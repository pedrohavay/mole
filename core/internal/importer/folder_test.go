package importer

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"mole/core/internal/library"
	"mole/core/internal/testbook"
)

// writeBook writes an EPUB titled title at dir/name, creating dir.
func writeBook(t *testing.T, dir, name, title string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	return testbook.Book{Title: title, Chapters: []string{"<p>" + title + "</p>"}}.WriteTo(t, filepath.Join(dir, name))
}

// collectionsByName returns the titles of the books in each collection.
func collectionsByName(t *testing.T, store *library.Store) map[string][]string {
	t.Helper()
	list, err := store.Collections()
	if err != nil {
		t.Fatalf("Collections error: %v", err)
	}
	books, err := store.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	out := map[string][]string{}
	for _, c := range list {
		out[c.Name] = []string{}
		for _, b := range books {
			if slices.Contains(b.Collections, c.ID) {
				out[c.Name] = append(out[c.Name], b.Title)
			}
		}
		sort.Strings(out[c.Name])
	}
	return out
}

func bookTitles(t *testing.T, store *library.Store) []string {
	t.Helper()
	books, err := store.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	var out []string
	for _, b := range books {
		out = append(out, b.Title)
	}
	sort.Strings(out)
	return out
}

func TestImportFolder(t *testing.T) {
	store := openStore(t)
	root := filepath.Join(tempDir(t), "Shelf")
	writeBook(t, root, "loose.epub", "Loose")
	writeBook(t, filepath.Join(root, "Essays"), "a.epub", "Essay A")
	writeBook(t, filepath.Join(root, "Essays"), "b.EPUB", "Essay B")
	writeBook(t, filepath.Join(root, "Essays", "Short"), "c.epub", "Short C")
	writeBook(t, filepath.Join(root, ".hidden"), "h.epub", "Hidden")
	writeBook(t, filepath.Join(root, "Empty"), "notes.txt", "Not a book")
	if err := os.WriteFile(filepath.Join(root, "broken.epub"), []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)

	r, err := ImportFolder(store, root, FolderOptions{Collections: true}, now)
	if err != nil {
		t.Fatalf("ImportFolder error: %v", err)
	}
	if r.Folder != 0 || r.Found != 5 || len(r.Added) != 4 || len(r.Updated) != 0 || len(r.Failed) != 1 ||
		r.Failed[0].Path != filepath.Join(root, "broken.epub") {
		t.Errorf("ImportFolder = %+v, want 5 found, 4 added and broken.epub failed, unwatched", r)
	}
	want := map[string][]string{
		"Shelf":  {"Loose"},
		"Essays": {"Essay A", "Essay B"},
		"Short":  {"Short C"},
	}
	if got := collectionsByName(t, store); !reflect.DeepEqual(got, want) {
		t.Errorf("collections after ImportFolder = %v, want %v", got, want)
	}
	if folders, _ := store.Folders(); len(folders) != 0 {
		t.Errorf("ImportFolder without Watch watches %v", folders)
	}

	// Importing again adds nothing, and puts books already in the library
	// into the collections of their folders.
	if err := store.DeleteCollection(mustCollection(t, store, "Short").ID); err != nil {
		t.Fatalf("DeleteCollection error: %v", err)
	}
	r, err = ImportFolder(store, root, FolderOptions{Collections: true}, now)
	if err != nil || len(r.Added) != 0 || len(r.Updated) != 0 {
		t.Errorf("ImportFolder again = %+v, %v; want nothing added or updated", r, err)
	}
	if got := collectionsByName(t, store); !reflect.DeepEqual(got, want) {
		t.Errorf("collections after importing again = %v, want %v", got, want)
	}
}

func mustCollection(t *testing.T, store *library.Store, name string) library.Collection {
	t.Helper()
	list, err := store.Collections()
	if err != nil {
		t.Fatalf("Collections error: %v", err)
	}
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no collection named %q", name)
	return library.Collection{}
}

func TestImportFolderRejects(t *testing.T) {
	store := openStore(t)
	file := writeBook(t, t.TempDir(), "a.epub", "A")
	if err := os.MkdirAll(store.BooksDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	for _, dir := range []string{file, store.BooksDir(), filepath.Join(t.TempDir(), "gone")} {
		if _, err := ImportFolder(store, dir, FolderOptions{}, time.Unix(1, 0)); err == nil {
			t.Errorf("ImportFolder(%s): got nil error", dir)
		}
	}
}

func TestImportFolderSkipsTheLibrary(t *testing.T) {
	// A folder that holds the library itself must not import its copies.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Mole"), 0o755); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}
	store, err := library.Open(filepath.Join(root, "Mole", "library.db"))
	if err != nil {
		t.Fatalf("library.Open error: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	writeBook(t, root, "a.epub", "A")
	r, err := ImportFolder(store, root, FolderOptions{Watch: true}, time.Unix(1, 0))
	if err != nil || len(r.Added) != 1 {
		t.Fatalf("ImportFolder = %+v, %v; want one book added", r, err)
	}
	r, err = Sync(store, r.Folder, time.Unix(2, 0))
	if err != nil || r.Found != 1 || len(r.Added) != 0 {
		t.Errorf("Sync = %+v, %v; want the one book found and nothing added", r, err)
	}
}

func TestSync(t *testing.T) {
	store := openStore(t)
	root := filepath.Join(tempDir(t), "Library")
	essays := filepath.Join(root, "Essays")
	a := writeBook(t, essays, "a.epub", "Essay A")
	b := writeBook(t, essays, "b.epub", "Essay B")
	gone := writeBook(t, filepath.Join(root, "Novels"), "n.epub", "Novel")
	now := time.Unix(1_700_000_000, 0)

	first, err := ImportFolder(store, root, FolderOptions{Collections: true, Watch: true}, now)
	if err != nil {
		t.Fatalf("ImportFolder error: %v", err)
	}
	if first.Folder == 0 || len(first.Added) != 3 {
		t.Fatalf("ImportFolder = %+v, want a watched folder and 3 books added", first)
	}
	folder, err := store.Folder(first.Folder)
	if err != nil || folder.Path != root || !folder.Collections || !folder.SyncedAt.Equal(now) {
		t.Errorf("watched folder = %+v, %v; want %s making collections, synced now", folder, err, root)
	}
	ids := map[string]int64{}
	for _, book := range mustBooks(t, store) {
		ids[book.Title] = book.ID
	}

	// A new file, a renamed one, a changed one and a removed one.
	writeBook(t, filepath.Join(root, "Later"), "new.epub", "New")
	renamed := filepath.Join(essays, "a, renamed.epub")
	if err := os.Rename(a, renamed); err != nil {
		t.Fatalf("Rename error: %v", err)
	}
	writeBook(t, essays, "b.epub", "Essay B, second edition")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(b, later, later); err != nil {
		t.Fatalf("Chtimes error: %v", err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	r, err := Sync(store, folder.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if r.Found != 3 || len(r.Added) != 1 || len(r.Failed) != 0 {
		t.Errorf("Sync = %+v, want 3 found and 1 added", r)
	}
	if want := []int64{ids["Essay A"], ids["Essay B"]}; !sameIDs(r.Updated, want) {
		t.Errorf("Sync updated %v, want the renamed and the changed book %v", r.Updated, want)
	}
	if !reflect.DeepEqual(r.Missing, []int64{ids["Novel"]}) {
		t.Errorf("Sync missing = %v, want [%d]", r.Missing, ids["Novel"])
	}
	wantTitles := []string{"Essay A", "Essay B, second edition", "New", "Novel"}
	if got := bookTitles(t, store); !reflect.DeepEqual(got, wantTitles) {
		t.Errorf("books after Sync = %v, want %v (no duplicates, nothing removed)", got, wantTitles)
	}
	if got, _ := store.Book(ids["Essay A"]); got.OriginalPath != renamed {
		t.Errorf("renamed book comes from %q, want %q", got.OriginalPath, renamed)
	}
	if got, _ := store.Book(ids["Novel"]); !got.Missing {
		t.Error("book of a removed file is not marked missing")
	}
	if names := booksIn(t, store.BooksDir()); len(names) != 4 {
		t.Errorf("Books directory holds %v, want one copy per book", names)
	}
	want := map[string][]string{
		"Essays": {"Essay A", "Essay B, second edition"},
		"Novels": {"Novel"},
		"Later":  {"New"},
	}
	if got := collectionsByName(t, store); !reflect.DeepEqual(got, want) {
		t.Errorf("collections after Sync = %v, want %v", got, want)
	}

	// The removed file comes back; a book the user took out of its
	// collection stays out while its file is unchanged.
	writeBook(t, filepath.Join(root, "Novels"), "n.epub", "Novel")
	if err := store.RemoveFromCollection(mustCollection(t, store, "Essays").ID, ids["Essay A"]); err != nil {
		t.Fatalf("RemoveFromCollection error: %v", err)
	}
	r, err = Sync(store, folder.ID, now.Add(2*time.Minute))
	if err != nil || len(r.Missing) != 0 || len(r.Added) != 0 {
		t.Errorf("Sync after the file came back = %+v, %v; want nothing missing or added", r, err)
	}
	if got, _ := store.Book(ids["Novel"]); got.Missing {
		t.Error("book whose file came back is still marked missing")
	}
	if got := collectionsByName(t, store)["Essays"]; !reflect.DeepEqual(got, []string{"Essay B, second edition"}) {
		t.Errorf("Essays after Sync = %v, want the book the user took out to stay out", got)
	}
}

func TestSyncMovesBetweenCollections(t *testing.T) {
	store := openStore(t)
	root := t.TempDir()
	from := writeBook(t, filepath.Join(root, "To Read"), "a.epub", "A")
	r, err := ImportFolder(store, root, FolderOptions{Collections: true, Watch: true}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("ImportFolder error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Done"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.Rename(from, filepath.Join(root, "Done", "a.epub")); err != nil {
		t.Fatalf("Rename error: %v", err)
	}
	if _, err := Sync(store, r.Folder, time.Unix(2, 0)); err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	want := map[string][]string{"To Read": {}, "Done": {"A"}}
	if got := collectionsByName(t, store); !reflect.DeepEqual(got, want) {
		t.Errorf("collections after a move = %v, want %v", got, want)
	}
}

func TestSyncOfAFolderThatIsGone(t *testing.T) {
	store := openStore(t)
	root := filepath.Join(t.TempDir(), "Disk")
	writeBook(t, root, "a.epub", "A")
	r, err := ImportFolder(store, root, FolderOptions{Watch: true}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("ImportFolder error: %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}
	if _, err := Sync(store, r.Folder, time.Unix(2, 0)); err == nil {
		t.Error("Sync of a folder that is gone: got nil error")
	}
	if got, _ := store.Book(r.Added[0]); got.Missing {
		t.Error("Sync of a folder that is gone marked its books missing")
	}
}

func mustBooks(t *testing.T, store *library.Store) []library.Book {
	t.Helper()
	books, err := store.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	return books
}

func sameIDs(got, want []int64) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

func TestImportFolderAfterImportThroughALink(t *testing.T) {
	// On macOS the temporary directory is reached through a symbolic link
	// (/var is /private/var); a file imported by one name and found again
	// by the other is still one book.
	store := openStore(t)
	dir := t.TempDir()
	if _, err := Import(store, writeBook(t, dir, "a.epub", "A"), time.Unix(1, 0)); err != nil {
		t.Fatalf("Import error: %v", err)
	}
	r, err := ImportFolder(store, dir, FolderOptions{}, time.Unix(2, 0))
	if err != nil || r.Found != 1 || len(r.Added) != 0 {
		t.Errorf("ImportFolder = %+v, %v; want the book found and nothing added", r, err)
	}
}
