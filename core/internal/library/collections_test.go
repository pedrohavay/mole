package library

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func collectionNames(list []Collection) []string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = c.Name
	}
	return out
}

func TestCollections(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	essays, err := s.CreateCollection("  Essays ", now)
	if err != nil {
		t.Fatalf("CreateCollection error: %v", err)
	}
	if essays.Name != "Essays" || essays.ID == 0 || !essays.CreatedAt.Equal(now) {
		t.Errorf("CreateCollection = %+v, want Essays, trimmed, with an ID and the time", essays)
	}
	art, err := s.CreateCollection("art", now)
	if err != nil {
		t.Fatalf("CreateCollection error: %v", err)
	}
	if _, err := s.CreateCollection("ESSAYS", now); !errors.Is(err, ErrCollectionExists) {
		t.Errorf("CreateCollection of a taken name: error %v, want ErrCollectionExists", err)
	}
	if _, err := s.CreateCollection("  ", now); err == nil {
		t.Error("CreateCollection of a blank name: got nil error")
	}
	if list, err := s.Collections(); err != nil || !reflect.DeepEqual(collectionNames(list), []string{"art", "Essays"}) {
		t.Errorf("Collections = %v, %v; want [art Essays] by name, ignoring case", collectionNames(list), err)
	}

	if err := s.RenameCollection(art.ID, "Art"); err != nil {
		t.Errorf("RenameCollection to a new case of its own name error: %v", err)
	}
	if err := s.RenameCollection(art.ID, "essays"); !errors.Is(err, ErrCollectionExists) {
		t.Errorf("RenameCollection to a taken name: error %v, want ErrCollectionExists", err)
	}
	if err := s.RenameCollection(999, "Other"); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("RenameCollection of a missing collection: error %v, want ErrCollectionNotFound", err)
	}
	if c, err := s.Collection(art.ID); err != nil || c.Name != "Art" {
		t.Errorf("Collection after renaming = %+v, %v; want Art", c, err)
	}

	if err := s.DeleteCollection(art.ID); err != nil {
		t.Fatalf("DeleteCollection error: %v", err)
	}
	if err := s.DeleteCollection(art.ID); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("DeleteCollection twice: error %v, want ErrCollectionNotFound", err)
	}
}

func TestCollectionMembers(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	a, _ := s.Add(Book{Path: "/a.epub", Title: "A"}, now)
	b, _ := s.Add(Book{Path: "/b.epub", Title: "B"}, now)
	essays, _ := s.CreateCollection("Essays", now)
	art, _ := s.CreateCollection("Art", now)

	for _, m := range []struct{ collection, book int64 }{
		{essays.ID, a.ID}, {art.ID, a.ID}, {essays.ID, b.ID}, {essays.ID, a.ID},
	} {
		if err := s.AddToCollection(m.collection, m.book, now); err != nil {
			t.Fatalf("AddToCollection(%d, %d) error: %v", m.collection, m.book, err)
		}
	}
	if err := s.AddToCollection(999, a.ID, now); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("AddToCollection of a missing collection: error %v, want ErrCollectionNotFound", err)
	}
	if err := s.AddToCollection(essays.ID, 999, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddToCollection of a missing book: error %v, want ErrNotFound", err)
	}
	wantA := []int64{essays.ID, art.ID}
	if essays.ID > art.ID {
		wantA = []int64{art.ID, essays.ID}
	}
	if got, err := s.Book(a.ID); err != nil || !reflect.DeepEqual(got.Collections, wantA) {
		t.Errorf("Book(a).Collections = %v, %v; want %v", got.Collections, err, wantA)
	}
	books, err := s.Books()
	if err != nil {
		t.Fatalf("Books error: %v", err)
	}
	for _, book := range books {
		want := map[int64][]int64{a.ID: wantA, b.ID: {essays.ID}}[book.ID]
		if !reflect.DeepEqual(book.Collections, want) {
			t.Errorf("Books: %s in %v, want %v", book.Title, book.Collections, want)
		}
	}

	if err := s.RemoveFromCollection(essays.ID, a.ID); err != nil {
		t.Fatalf("RemoveFromCollection error: %v", err)
	}
	if err := s.RemoveFromCollection(essays.ID, a.ID); err != nil {
		t.Errorf("RemoveFromCollection of a book not there: error %v, want nil", err)
	}
	if got, _ := s.Book(a.ID); !reflect.DeepEqual(got.Collections, []int64{art.ID}) {
		t.Errorf("Book(a).Collections after removing it from Essays = %v, want [%d]", got.Collections, art.ID)
	}

	// Deleting a collection keeps its books; removing a book empties its
	// places in collections.
	if err := s.DeleteCollection(art.ID); err != nil {
		t.Fatalf("DeleteCollection error: %v", err)
	}
	if got, err := s.Book(a.ID); err != nil || got.Collections != nil {
		t.Errorf("Book(a) after deleting its collection = %v, %v; want the book in no collection", got.Collections, err)
	}
	if err := s.Remove(b.ID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if held, err := s.memberships(); err != nil || len(held) != 0 {
		t.Errorf("memberships after removing the last member = %v, %v; want none", held, err)
	}
}

func TestMakeCollectionOfFolder(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	essays, err := s.MakeCollectionOfFolder("/books/Essays", now)
	if err != nil {
		t.Fatalf("MakeCollectionOfFolder error: %v", err)
	}
	if essays.Name != "Essays" || essays.Folder != "/books/Essays" {
		t.Errorf("MakeCollectionOfFolder = %+v, want Essays from /books/Essays", essays)
	}
	// A renamed collection is still the folder's.
	if err := s.RenameCollection(essays.ID, "Thinking"); err != nil {
		t.Fatalf("RenameCollection error: %v", err)
	}
	if again, err := s.MakeCollectionOfFolder("/books/Essays", now); err != nil || again.ID != essays.ID {
		t.Errorf("MakeCollectionOfFolder after a rename = %+v, %v; want collection %d", again, err, essays.ID)
	}
	// A collection the user made with the folder's name is taken over.
	mine, _ := s.CreateCollection("Poems", now)
	if got, err := s.MakeCollectionOfFolder("/books/poems", now); err != nil || got.ID != mine.ID || got.Folder != "/books/poems" {
		t.Errorf("MakeCollectionOfFolder of a name in use = %+v, %v; want collection %d linked to the folder", got, err, mine.ID)
	}
	if got, err := s.CollectionOfFolder("/books/poems"); err != nil || got.ID != mine.ID {
		t.Errorf("CollectionOfFolder = %+v, %v; want collection %d", got, err, mine.ID)
	}
	if got, err := s.MakeCollectionOfFolder("/books/  ", now); err != nil || got.Name != "/books/" {
		t.Errorf("MakeCollectionOfFolder of a blank name = %+v, %v; want the collection named after the path", got, err)
	}
	if _, err := s.CollectionOfFolder("/elsewhere"); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("CollectionOfFolder of an unknown folder: error %v, want ErrCollectionNotFound", err)
	}
}
