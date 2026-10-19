// Package library stores the user's books, where each one was left, their
// bookmarks and highlights, and the reader settings in a SQLite database.
package library

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Errors returned when a record does not exist.
var (
	ErrNotFound           = errors.New("library: book not found")
	ErrAnnotationNotFound = errors.New("library: annotation not found")
)

// Book is a book in the library.
type Book struct {
	ID           int64
	Path         string // the file read, normally a copy in the library's Books directory
	OriginalPath string // the file the book was imported from
	Title        string
	Author       string
	Cover        []byte // encoded image, or nil; Books leaves it out
	AddedAt      time.Time
	OpenedAt     time.Time // zero if the book was never opened
	Position     Position
}

// Position is where a book was left. Chapter and Offset are the anchor of
// the first character shown (see reading.Anchor); positions saved before
// anchors existed have an Offset of -1, and ChapterFraction, how far into
// the chapter the page was, locates them instead. Fraction is the share of
// the whole book read.
type Position struct {
	Chapter         int
	Offset          int
	ChapterFraction float64
	Fraction        float64
}

// Settings are the reader settings shared by every book.
type Settings struct {
	Level    float64 // text brightness as a fraction of the EDR headroom
	EDR      bool
	Tint     int
	Gamma    float64
	FontSize float64
	Spread   int    // pages side by side: 0 automatic, 1 single, 2 double
	Accent   string // interface accent as "#RRGGBB"; empty means the default
	Images   int    // how bright the image layer is: 0 dim, 1 off, 2 full
}

// DefaultSettings returns the settings used before the user changes any.
func DefaultSettings() Settings {
	return Settings{Level: 0.5, EDR: true, Gamma: 2.2, FontSize: 21}
}

// Anchor is a place in the text of a book that does not depend on the layout:
// a chapter and an offset into it (see reading.Anchor).
type Anchor struct {
	Chapter int
	Offset  int
}

// AnnotationKind says what an annotation is.
type AnnotationKind string

// Annotation kinds. A note is a highlight with a note.
const (
	KindBookmark  AnnotationKind = "bookmark"  // a place: Start == End
	KindHighlight AnnotationKind = "highlight" // a passage of text
)

// Annotation is a bookmark or a highlight in a book.
type Annotation struct {
	ID         int64
	BookID     int64
	Kind       AnnotationKind
	Start, End Anchor // End is exclusive
	Color      int    // index into the reader's palette; 0 for bookmarks
	Note       string
	Text       string // the passage, or the text at a bookmark, when it was made
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// migrations upgrade the schema; PRAGMA user_version records how many ran.
var migrations = []string{
	`CREATE TABLE books (
		id               INTEGER PRIMARY KEY,
		path             TEXT    NOT NULL UNIQUE,
		title            TEXT    NOT NULL,
		author           TEXT    NOT NULL DEFAULT '',
		cover            BLOB,
		added_at         INTEGER NOT NULL,
		opened_at        INTEGER NOT NULL DEFAULT 0,
		chapter          INTEGER NOT NULL DEFAULT 0,
		chapter_fraction REAL    NOT NULL DEFAULT 0,
		fraction         REAL    NOT NULL DEFAULT 0
	);
	CREATE TABLE reader_settings (
		id        INTEGER PRIMARY KEY CHECK (id = 1),
		level     REAL    NOT NULL,
		edr       INTEGER NOT NULL,
		tint      INTEGER NOT NULL,
		gamma     REAL    NOT NULL,
		font_size REAL    NOT NULL
	);`,
	`ALTER TABLE reader_settings ADD COLUMN spread INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE reader_settings ADD COLUMN accent TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE annotations (
		id            INTEGER PRIMARY KEY,
		book_id       INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
		kind          TEXT    NOT NULL CHECK (kind IN ('bookmark', 'highlight')),
		start_chapter INTEGER NOT NULL,
		start_offset  INTEGER NOT NULL,
		end_chapter   INTEGER NOT NULL,
		end_offset    INTEGER NOT NULL,
		color         INTEGER NOT NULL DEFAULT 0,
		note          TEXT    NOT NULL DEFAULT '',
		text          TEXT    NOT NULL DEFAULT '',
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	);
	CREATE INDEX annotations_by_position ON annotations (book_id, start_chapter, start_offset);`,
	`ALTER TABLE books ADD COLUMN original_path TEXT NOT NULL DEFAULT '';
	ALTER TABLE books ADD COLUMN chapter_offset INTEGER NOT NULL DEFAULT -1;
	UPDATE books SET original_path = path;
	CREATE UNIQUE INDEX books_by_original_path ON books (original_path);`,
	`ALTER TABLE reader_settings ADD COLUMN images INTEGER NOT NULL DEFAULT 0;`,
}

// Store is a library backed by a SQLite database. It is safe for concurrent use.
type Store struct {
	db       *sql.DB
	booksDir string
}

// Open opens or creates the library database at path and brings its schema
// up to date. Copies of imported books go to a Books directory next to it.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open library: %w", err)
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open library: %w", err)
	}
	s := &Store{db: db, booksDir: filepath.Join(dir, "Books")}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// BooksDir returns the directory that holds the library's copies of its
// books. It may not exist yet.
func (s *Store) BooksDir() string {
	return s.booksDir
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("migrate to %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to %d: %w", i+1, err)
		}
		// PRAGMA does not accept parameters; i+1 is an integer we control.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate to %d: %w", i+1, err)
		}
	}
	return nil
}

// bookColumns are the columns scanBook reads, in order.
const bookColumns = `id, path, original_path, title, author, added_at, opened_at,
	chapter, chapter_offset, chapter_fraction, fraction`

// Books returns every book, most recently opened first, then most recently
// added, without their covers.
func (s *Store) Books() ([]Book, error) {
	rows, err := s.db.Query(`SELECT ` + bookColumns + ` FROM books ORDER BY opened_at DESC, added_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	return books, nil
}

// Book returns the book with id, with its cover.
func (s *Store) Book(id int64) (Book, error) {
	var cover []byte
	row := s.db.QueryRow(`SELECT `+bookColumns+`, cover FROM books WHERE id = ?`, id)
	b, err := scanBook(row, &cover)
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	b.Cover = cover
	return b, err
}

// BookByOriginalPath returns the book imported from path, with its cover,
// or ErrNotFound.
func (s *Store) BookByOriginalPath(path string) (Book, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM books WHERE original_path = ?`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, fmt.Errorf("find %s: %w", path, err)
	}
	return s.Book(id)
}

// Add stores a new book and returns it with its ID. OriginalPath defaults to
// Path. Adding a book with the path or the original path of one already in
// the library returns the existing book unchanged.
func (s *Store) Add(b Book, now time.Time) (Book, error) {
	if b.OriginalPath == "" {
		b.OriginalPath = b.Path
	}
	res, err := s.db.Exec(`
		INSERT INTO books (path, original_path, title, author, cover, added_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, b.Path, b.OriginalPath, b.Title, b.Author, b.Cover, now.Unix())
	if err != nil {
		return Book{}, fmt.Errorf("add %s: %w", b.Path, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 1 {
		id, err := res.LastInsertId()
		if err != nil {
			return Book{}, fmt.Errorf("add %s: %w", b.Path, err)
		}
		return s.Book(id)
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM books WHERE path = ? OR original_path = ?`, b.Path, b.OriginalPath).Scan(&id)
	if err != nil {
		return Book{}, fmt.Errorf("find %s: %w", b.Path, err)
	}
	return s.Book(id)
}

// Remove deletes a book and its annotations from the library, and the
// library's copy of the book. A file outside the Books directory, such as
// one imported before books were copied, is not touched.
func (s *Store) Remove(id int64) error {
	var path string
	err := s.db.QueryRow(`SELECT path FROM books WHERE id = ?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove book %d: %w", id, err)
	}
	if _, err := s.db.Exec(`DELETE FROM books WHERE id = ?`, id); err != nil {
		return fmt.Errorf("remove book %d: %w", id, err)
	}
	if !s.owns(path) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the copy of book %d: %w", id, err)
	}
	return nil
}

// owns reports whether path is a file inside the Books directory.
func (s *Store) owns(path string) bool {
	rel, err := filepath.Rel(s.booksDir, path)
	return err == nil && filepath.IsAbs(path) && rel != "." && !strings.HasPrefix(rel, "..")
}

// SavePosition records where a book was left and when it was read.
func (s *Store) SavePosition(id int64, p Position, openedAt time.Time) error {
	res, err := s.db.Exec(`
		UPDATE books SET chapter = ?, chapter_offset = ?, chapter_fraction = ?, fraction = ?, opened_at = ?
		WHERE id = ?`,
		p.Chapter, p.Offset, p.ChapterFraction, p.Fraction, openedAt.Unix(), id)
	if err != nil {
		return fmt.Errorf("save position of book %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Settings returns the reader settings, or the defaults if none were saved.
func (s *Store) Settings() (Settings, error) {
	var st Settings
	err := s.db.QueryRow(`
		SELECT level, edr, tint, gamma, font_size, spread, accent, images FROM reader_settings WHERE id = 1`).
		Scan(&st.Level, &st.EDR, &st.Tint, &st.Gamma, &st.FontSize, &st.Spread, &st.Accent, &st.Images)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	return st, nil
}

// SaveSettings replaces the reader settings.
func (s *Store) SaveSettings(st Settings) error {
	_, err := s.db.Exec(`
		INSERT INTO reader_settings (id, level, edr, tint, gamma, font_size, spread, accent, images)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET level = excluded.level, edr = excluded.edr, tint = excluded.tint,
			gamma = excluded.gamma, font_size = excluded.font_size, spread = excluded.spread,
			accent = excluded.accent, images = excluded.images`,
		st.Level, st.EDR, st.Tint, st.Gamma, st.FontSize, st.Spread, st.Accent, st.Images)
	if err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

// Annotations returns the annotations of a book in reading order.
func (s *Store) Annotations(bookID int64) ([]Annotation, error) {
	rows, err := s.db.Query(`
		SELECT id, book_id, kind, start_chapter, start_offset, end_chapter, end_offset, color, note, text,
			created_at, updated_at
		FROM annotations WHERE book_id = ?
		ORDER BY start_chapter, start_offset, end_chapter, end_offset, id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list annotations of book %d: %w", bookID, err)
	}
	defer rows.Close()
	var out []Annotation
	for rows.Next() {
		a, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list annotations of book %d: %w", bookID, err)
	}
	return out, nil
}

// AddAnnotation stores a new annotation of a.BookID and returns it with its
// ID and times. A bookmark's range must be empty (Start == End). It returns
// ErrNotFound when the book does not exist.
func (s *Store) AddAnnotation(a Annotation, now time.Time) (Annotation, error) {
	if a.Kind != KindBookmark && a.Kind != KindHighlight {
		return Annotation{}, fmt.Errorf("library: unknown annotation kind %q", a.Kind)
	}
	if a.Start.Offset < 0 || a.Start.Chapter < 0 || a.End.Chapter < a.Start.Chapter ||
		a.End.Chapter == a.Start.Chapter && a.End.Offset < a.Start.Offset ||
		a.Kind == KindBookmark && a.Start != a.End {
		return Annotation{}, fmt.Errorf("library: invalid %s range %+v to %+v", a.Kind, a.Start, a.End)
	}
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM books WHERE id = ?)`, a.BookID).Scan(&exists)
	if err != nil {
		return Annotation{}, fmt.Errorf("find book %d: %w", a.BookID, err)
	}
	if !exists {
		return Annotation{}, ErrNotFound
	}
	res, err := s.db.Exec(`
		INSERT INTO annotations (book_id, kind, start_chapter, start_offset, end_chapter, end_offset,
			color, note, text, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.BookID, a.Kind, a.Start.Chapter, a.Start.Offset, a.End.Chapter, a.End.Offset,
		a.Color, a.Note, a.Text, now.Unix(), now.Unix())
	if err != nil {
		return Annotation{}, fmt.Errorf("add annotation to book %d: %w", a.BookID, err)
	}
	if a.ID, err = res.LastInsertId(); err != nil {
		return Annotation{}, fmt.Errorf("add annotation to book %d: %w", a.BookID, err)
	}
	a.CreatedAt, a.UpdatedAt = time.Unix(now.Unix(), 0), time.Unix(now.Unix(), 0)
	return a, nil
}

// UpdateAnnotation replaces the color and the note of an annotation.
func (s *Store) UpdateAnnotation(id int64, color int, note string, now time.Time) error {
	res, err := s.db.Exec(`UPDATE annotations SET color = ?, note = ?, updated_at = ? WHERE id = ?`,
		color, note, now.Unix(), id)
	if err != nil {
		return fmt.Errorf("update annotation %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrAnnotationNotFound
	}
	return nil
}

// RemoveAnnotation deletes an annotation.
func (s *Store) RemoveAnnotation(id int64) error {
	res, err := s.db.Exec(`DELETE FROM annotations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove annotation %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrAnnotationNotFound
	}
	return nil
}

func scanAnnotation(row scanner) (Annotation, error) {
	var a Annotation
	var created, updated int64
	err := row.Scan(&a.ID, &a.BookID, &a.Kind, &a.Start.Chapter, &a.Start.Offset, &a.End.Chapter, &a.End.Offset,
		&a.Color, &a.Note, &a.Text, &created, &updated)
	if err != nil {
		return Annotation{}, fmt.Errorf("read annotation: %w", err)
	}
	a.CreatedAt, a.UpdatedAt = time.Unix(created, 0), time.Unix(updated, 0)
	return a, nil
}

type scanner interface {
	Scan(dest ...any) error
}

// scanBook reads the bookColumns of row, followed by extra columns into
// extra.
func scanBook(row scanner, extra ...any) (Book, error) {
	var b Book
	var added, opened int64
	dest := append([]any{&b.ID, &b.Path, &b.OriginalPath, &b.Title, &b.Author, &added, &opened,
		&b.Position.Chapter, &b.Position.Offset, &b.Position.ChapterFraction, &b.Position.Fraction}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Book{}, err
	}
	b.AddedAt = time.Unix(added, 0)
	if opened != 0 {
		b.OpenedAt = time.Unix(opened, 0)
	}
	return b, nil
}
