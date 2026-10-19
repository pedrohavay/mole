package library

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ErrCollectionExists is returned when a collection would take the name of
// another. Names are compared without regard to ASCII case.
var ErrCollectionExists = errors.New("library: a collection with that name already exists")

// Collection is a named group of books. A book may be in any number of
// collections.
type Collection struct {
	ID        int64
	Name      string
	Folder    string // the folder the collection was made from, or ""
	CreatedAt time.Time
}

// Collections returns every collection, by name.
func (s *Store) Collections() ([]Collection, error) {
	rows, err := s.db.Query(`SELECT id, name, folder, created_at FROM collections ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	return out, nil
}

// Collection returns the collection with id.
func (s *Store) Collection(id int64) (Collection, error) {
	c, err := scanCollection(s.db.QueryRow(`SELECT id, name, folder, created_at FROM collections WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, ErrCollectionNotFound
	}
	return c, err
}

// CreateCollection stores a new, empty collection named name, with the
// spaces around it trimmed.
func (s *Store) CreateCollection(name string, now time.Time) (Collection, error) {
	return s.createCollection(name, "", now)
}

func (s *Store) createCollection(name, folder string, now time.Time) (Collection, error) {
	name, err := collectionName(name)
	if err != nil {
		return Collection{}, err
	}
	res, err := s.db.Exec(`INSERT INTO collections (name, folder, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		name, folder, now.Unix())
	if err != nil {
		return Collection{}, fmt.Errorf("create collection %q: %w", name, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Collection{}, ErrCollectionExists
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Collection{}, fmt.Errorf("create collection %q: %w", name, err)
	}
	return Collection{ID: id, Name: name, Folder: folder, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// RenameCollection gives a collection a new name, with the spaces around it
// trimmed.
func (s *Store) RenameCollection(id int64, name string) error {
	name, err := collectionName(name)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE OR IGNORE collections SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("rename collection %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		// Either the collection is gone or the name is taken.
		if _, err := s.Collection(id); err != nil {
			return err
		}
		return ErrCollectionExists
	}
	return nil
}

// DeleteCollection deletes a collection. Its books stay in the library.
func (s *Store) DeleteCollection(id int64) error {
	res, err := s.db.Exec(`DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete collection %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCollectionNotFound
	}
	return nil
}

// AddToCollection puts a book in a collection. Adding a book the collection
// already holds does nothing.
func (s *Store) AddToCollection(collectionID, bookID int64, now time.Time) error {
	var hasCollection, hasBook bool
	err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM collections WHERE id = ?), EXISTS (SELECT 1 FROM books WHERE id = ?)`,
		collectionID, bookID).Scan(&hasCollection, &hasBook)
	if err != nil {
		return fmt.Errorf("add book %d to collection %d: %w", bookID, collectionID, err)
	}
	if !hasCollection {
		return ErrCollectionNotFound
	}
	if !hasBook {
		return ErrNotFound
	}
	_, err = s.db.Exec(`INSERT INTO collection_books (collection_id, book_id, added_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		collectionID, bookID, now.Unix())
	if err != nil {
		return fmt.Errorf("add book %d to collection %d: %w", bookID, collectionID, err)
	}
	return nil
}

// RemoveFromCollection takes a book out of a collection. Removing a book the
// collection does not hold does nothing.
func (s *Store) RemoveFromCollection(collectionID, bookID int64) error {
	_, err := s.db.Exec(`DELETE FROM collection_books WHERE collection_id = ? AND book_id = ?`, collectionID, bookID)
	if err != nil {
		return fmt.Errorf("remove book %d from collection %d: %w", bookID, collectionID, err)
	}
	return nil
}

// CollectionOfFolder returns the collection made from dir, or
// ErrCollectionNotFound.
func (s *Store) CollectionOfFolder(dir string) (Collection, error) {
	c, err := scanCollection(s.db.QueryRow(`SELECT id, name, folder, created_at FROM collections WHERE folder = ?`, dir))
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, ErrCollectionNotFound
	}
	return c, err
}

// MakeCollectionOfFolder returns the collection made from dir, making one
// named after dir if there is none. A collection the user made with that
// name is taken over rather than duplicated. The link to dir survives a
// rename, so books added to dir later find the collection again.
func (s *Store) MakeCollectionOfFolder(dir string, now time.Time) (Collection, error) {
	c, err := s.CollectionOfFolder(dir)
	if !errors.Is(err, ErrCollectionNotFound) {
		return c, err
	}
	name, err := collectionName(filepath.Base(dir))
	if err != nil {
		return Collection{}, err
	}
	c, err = scanCollection(s.db.QueryRow(`SELECT id, name, folder, created_at FROM collections WHERE name = ?`, name))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		c, err = s.createCollection(name, dir, now)
		if errors.Is(err, ErrCollectionExists) {
			// Another handle made it in the meantime.
			return s.CollectionOfFolder(dir)
		}
		return c, err
	case err != nil:
		return Collection{}, err
	case c.Folder == "":
		if _, err := s.db.Exec(`UPDATE collections SET folder = ? WHERE id = ?`, dir, c.ID); err != nil {
			return Collection{}, fmt.Errorf("link collection %d to %s: %w", c.ID, dir, err)
		}
		c.Folder = dir
	}
	return c, nil
}

// memberships returns the collections of every book that is in one.
func (s *Store) memberships() (map[int64][]int64, error) {
	rows, err := s.db.Query(`SELECT book_id, collection_id FROM collection_books ORDER BY book_id, collection_id`)
	if err != nil {
		return nil, fmt.Errorf("list collection members: %w", err)
	}
	defer rows.Close()
	held := map[int64][]int64{}
	for rows.Next() {
		var book, collection int64
		if err := rows.Scan(&book, &collection); err != nil {
			return nil, fmt.Errorf("list collection members: %w", err)
		}
		held[book] = append(held[book], collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list collection members: %w", err)
	}
	return held, nil
}

// collectionsOf returns the collections that hold a book.
func (s *Store) collectionsOf(bookID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT collection_id FROM collection_books WHERE book_id = ? ORDER BY collection_id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list collections of book %d: %w", bookID, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list collections of book %d: %w", bookID, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list collections of book %d: %w", bookID, err)
	}
	return ids, nil
}

// collectionName returns name without the spaces around it, or an error if
// nothing is left.
func collectionName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("library: a collection needs a name")
	}
	return name, nil
}

func scanCollection(row scanner) (Collection, error) {
	var c Collection
	var created int64
	if err := row.Scan(&c.ID, &c.Name, &c.Folder, &created); err != nil {
		return Collection{}, err
	}
	c.CreatedAt = time.Unix(created, 0)
	return c, nil
}
