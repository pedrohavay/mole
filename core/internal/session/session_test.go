package session

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mole/core/internal/library"
	"mole/core/internal/reading"
	"mole/core/internal/testbook"
)

func TestSavesWhereTheReaderIs(t *testing.T) {
	store, err := library.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("library.Open error: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	chapters := make([]string, 3)
	for c := range chapters {
		chapters[c] = strings.Repeat(fmt.Sprintf("<p>Chapter %d has a paragraph that runs over a few lines.</p>", c), 60)
	}
	added, err := store.Add(library.Book{Path: testbook.Chapters(t, chapters...), Title: "T"}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	clock := time.Unix(1_700_000_000, 0)
	open := func() *Book {
		t.Helper()
		b, err := Open(store, added.ID)
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		b.now = func() time.Time { return clock }
		if err := b.Reading().Layout(reading.NewGeometry(500, 600, 0, 1, 14, reading.SpreadSingle, reading.MarginsNormal), 14); err != nil {
			t.Fatalf("Layout error: %v", err)
		}
		return b
	}
	position := func() library.Position {
		t.Helper()
		got, err := store.Book(added.ID)
		if err != nil {
			t.Fatalf("Book error: %v", err)
		}
		return got.Position
	}

	b := open()
	// Opening and laying out do not save; nothing moved.
	if p := position(); p.Offset != -1 {
		t.Errorf("position after opening = %+v, want it untouched", p)
	}
	if changed, err := b.Turn(3); err != nil || !changed {
		t.Fatalf("Turn(3) = %v, %v", changed, err)
	}
	at := b.Reading().Location()
	if p := position(); p.Chapter != at.Chapter || p.Offset != at.Offset || p.Fraction != b.Reading().Progress() {
		t.Errorf("position after Turn = %+v, want the reader's place %+v", p, at)
	}
	if changed, err := b.Turn(-100); err != nil || !changed || position().Offset != 0 {
		t.Errorf("Turn back to the start = %v, %v at %+v; want offset 0 saved", changed, err, position())
	}
	if changed, _ := b.Turn(-1); changed {
		t.Error("Turn(-1) on the first page reported a change")
	}

	target := reading.Anchor{Chapter: 2, Offset: 300}
	if changed, err := b.GoToAnchor(target); err != nil || !changed {
		t.Fatalf("GoToAnchor = %v, %v", changed, err)
	}
	if p := position(); p.Chapter != 2 || p.Offset != 300 {
		t.Errorf("position after GoToAnchor = %+v, want chapter 2 offset 300", p)
	}
	page := b.Reading().Page()
	clock = clock.Add(time.Hour)
	if err := b.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if got, _ := store.Book(added.ID); !got.OpenedAt.Equal(clock) {
		t.Errorf("OpenedAt after Close = %v, want %v", got.OpenedAt, clock)
	}

	// Reopening shows the same page.
	b = open()
	defer b.Close()
	if b.Reading().Page() != page {
		t.Errorf("reopened on page %d, want %d", b.Reading().Page(), page)
	}
	if changed, err := b.GoTo(page); err != nil || changed {
		t.Errorf("GoTo the current page = %v, %v; want no change", changed, err)
	}
}
