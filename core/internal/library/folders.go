package library

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Folder is a folder the library keeps in sync: books that appear in it or
// change in it are imported, and books whose file leaves it are marked
// missing.
type Folder struct {
	ID          int64
	Path        string
	Collections bool // each folder in it that holds books is a collection
	AddedAt     time.Time
	SyncedAt    time.Time // zero before the first sync
}

// Folders returns the watched folders, by path.
func (s *Store) Folders() ([]Folder, error) {
	rows, err := s.db.Query(`SELECT id, path, collections, added_at, synced_at FROM watched_folders ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return out, nil
}

// Folder returns the watched folder with id.
func (s *Store) Folder(id int64) (Folder, error) {
	f, err := scanFolder(s.db.QueryRow(`SELECT id, path, collections, added_at, synced_at FROM watched_folders WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Folder{}, ErrFolderNotFound
	}
	return f, err
}

// AddFolder starts watching the folder at path, an absolute path, and
// returns it. Adding a folder already watched updates its collections
// setting.
func (s *Store) AddFolder(path string, collections bool, now time.Time) (Folder, error) {
	_, err := s.db.Exec(`
		INSERT INTO watched_folders (path, collections, added_at) VALUES (?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET collections = excluded.collections`, path, collections, now.Unix())
	if err != nil {
		return Folder{}, fmt.Errorf("watch %s: %w", path, err)
	}
	f, err := scanFolder(s.db.QueryRow(`SELECT id, path, collections, added_at, synced_at FROM watched_folders WHERE path = ?`, path))
	if err != nil {
		return Folder{}, fmt.Errorf("watch %s: %w", path, err)
	}
	return f, nil
}

// RemoveFolder stops watching a folder. Its books stay in the library; the
// ones marked missing lose the mark, unless another watched folder holds
// their file.
func (s *Store) RemoveFolder(id int64) error {
	f, err := s.Folder(id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM watched_folders WHERE id = ?`, id); err != nil {
		return fmt.Errorf("unwatch %s: %w", f.Path, err)
	}
	books, err := s.BooksUnder(f.Path)
	if err != nil {
		return err
	}
	others, err := s.Folders()
	if err != nil {
		return err
	}
	for _, b := range books {
		if b.Missing && !anyHolds(others, b.OriginalPath) {
			if err := s.SetMissing(b.ID, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// FolderSynced records when a folder was last synced.
func (s *Store) FolderSynced(id int64, now time.Time) error {
	res, err := s.db.Exec(`UPDATE watched_folders SET synced_at = ? WHERE id = ?`, now.Unix(), id)
	if err != nil {
		return fmt.Errorf("record sync of folder %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrFolderNotFound
	}
	return nil
}

// BooksUnder returns the books whose original file is inside dir, at any
// depth, without their covers or collections.
func (s *Store) BooksUnder(dir string) ([]Book, error) {
	// The range holds every path that starts with prefix: '0' follows '/'.
	prefix := dirPrefix(dir)
	books, err := s.queryBooks(`SELECT `+bookColumns+` FROM books WHERE original_path >= ? AND original_path < ? ORDER BY original_path`,
		prefix, prefix[:len(prefix)-1]+"0")
	if err != nil {
		return nil, fmt.Errorf("list books in %s: %w", dir, err)
	}
	return books, nil
}

// BooksBySource returns the books whose original file had the stamp st when
// it was last imported, without their covers or collections.
func (s *Store) BooksBySource(st Stamp) ([]Book, error) {
	if !st.Known() {
		return nil, nil
	}
	size, modTime := st.columns()
	books, err := s.queryBooks(`SELECT `+bookColumns+` FROM books WHERE source_size = ? AND source_mtime = ? ORDER BY id`,
		size, modTime)
	if err != nil {
		return nil, fmt.Errorf("find books by source: %w", err)
	}
	return books, nil
}

// SetSource records that the original file of a book is at path with the
// stamp st, as when it was renamed or moved, and clears its missing mark.
func (s *Store) SetSource(id int64, path string, st Stamp) error {
	size, modTime := st.columns()
	res, err := s.db.Exec(`UPDATE books SET original_path = ?, source_size = ?, source_mtime = ?, missing = 0 WHERE id = ?`,
		path, size, modTime, id)
	if err != nil {
		return fmt.Errorf("record the source of book %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMissing marks a book whose original file left a watched folder, or
// clears the mark.
func (s *Store) SetMissing(id int64, missing bool) error {
	res, err := s.db.Exec(`UPDATE books SET missing = ? WHERE id = ?`, missing, id)
	if err != nil {
		return fmt.Errorf("mark book %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Replace gives a book a new version of its file: the library's copy at
// b.Path, the original at b.OriginalPath with the stamp b.Source, and the
// title, author and cover read from it. The book keeps its ID, place,
// annotations and collections, loses its missing mark, and the library's
// previous copy is deleted.
func (s *Store) Replace(id int64, b Book) (Book, error) {
	var old string
	err := s.db.QueryRow(`SELECT path FROM books WHERE id = ?`, id).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, fmt.Errorf("replace book %d: %w", id, err)
	}
	size, modTime := b.Source.columns()
	_, err = s.db.Exec(`
		UPDATE books SET path = ?, original_path = ?, title = ?, author = ?, cover = ?,
			source_size = ?, source_mtime = ?, missing = 0
		WHERE id = ?`, b.Path, b.OriginalPath, b.Title, b.Author, b.Cover, size, modTime, id)
	if err != nil {
		return Book{}, fmt.Errorf("replace book %d: %w", id, err)
	}
	if old != b.Path && s.owns(old) {
		if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Book{}, fmt.Errorf("remove the old copy of book %d: %w", id, err)
		}
	}
	return s.Book(id)
}

// Within reports whether path is inside the folder dir, at any depth.
func Within(path, dir string) bool {
	return strings.HasPrefix(path, dirPrefix(dir))
}

// dirPrefix returns dir with one trailing slash, the prefix of every path
// inside it.
func dirPrefix(dir string) string {
	if strings.HasSuffix(dir, "/") {
		return dir
	}
	return dir + "/"
}

func anyHolds(folders []Folder, path string) bool {
	for _, f := range folders {
		if Within(path, f.Path) {
			return true
		}
	}
	return false
}

func scanFolder(row scanner) (Folder, error) {
	var f Folder
	var added, synced int64
	if err := row.Scan(&f.ID, &f.Path, &f.Collections, &added, &synced); err != nil {
		return Folder{}, err
	}
	f.AddedAt = time.Unix(added, 0)
	if synced != 0 {
		f.SyncedAt = time.Unix(synced, 0)
	}
	return f, nil
}
