// Package session opens books of the library for reading and keeps the
// library up to date with where the reader is: the place is saved whenever
// the reader moves and when the book is closed.
package session

import (
	"time"

	"mole/core/internal/library"
	"mole/core/internal/reading"
)

// Book is a book of the library open for reading. Its methods move the
// reader and save the place; Reading gives the layout, the rendering and the
// text, which do not move the reader. It is not safe for concurrent use.
type Book struct {
	store *library.Store
	id    int64
	read  *reading.Session
	now   func() time.Time
}

// Open opens the book with id at the place it was left.
func Open(store *library.Store, id int64) (*Book, error) {
	b, err := store.Book(id)
	if err != nil {
		return nil, err
	}
	p := b.Position
	read, err := reading.Open(b.Path, reading.Location{Chapter: p.Chapter, Fraction: p.ChapterFraction, Offset: p.Offset})
	if err != nil {
		return nil, err
	}
	return &Book{store: store, id: id, read: read, now: time.Now}, nil
}

// ID returns the library ID of the book.
func (b *Book) ID() int64 { return b.id }

// Reading returns the open book. Moving through it directly does not save
// the place; use the methods of Book for that.
func (b *Book) Reading() *reading.Session { return b.read }

// Turn moves by delta screens of pages and reports whether the page changed,
// saving the new place. A failure to save is returned with the change.
func (b *Book) Turn(delta int) (bool, error) {
	return b.saveIf(b.read.Turn(delta))
}

// GoTo shows the screen that holds page and reports whether the page
// changed, saving the new place.
func (b *Book) GoTo(page int) (bool, error) {
	return b.saveIf(b.read.GoTo(page))
}

// GoToAnchor shows the screen that holds a and reports whether the page
// changed, saving the new place.
func (b *Book) GoToAnchor(a reading.Anchor) (bool, error) {
	return b.saveIf(b.read.GoToAnchor(a))
}

func (b *Book) saveIf(changed bool, err error) (bool, error) {
	if err != nil || !changed {
		return changed, err
	}
	return true, b.save()
}

// Close saves the place and releases the book.
func (b *Book) Close() error {
	defer b.read.Close()
	return b.save()
}

// save records the place, unless the book was never laid out and so has no
// place of its own yet.
func (b *Book) save() error {
	if b.read.Pages() == 0 {
		return nil
	}
	at := b.read.Location()
	p := library.Position{Chapter: at.Chapter, Offset: at.Offset, ChapterFraction: at.Fraction, Fraction: b.read.Progress()}
	return b.store.SavePosition(b.id, p, b.now())
}
