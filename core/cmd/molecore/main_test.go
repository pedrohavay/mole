package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"time"

	"mole/core/internal/bookfile"
	"mole/core/internal/importer"
	"mole/core/internal/library"
)

func TestLastErrorKind(t *testing.T) {
	// The last error belongs to the OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tests := []struct {
		give error
		want int
	}{
		{give: fmt.Errorf("import /b.mobi: %w", bookfile.ErrDRM), want: errorKindDRM},
		{give: errors.New("cannot read"), want: errorKindOther},
	}
	for _, tt := range tests {
		fail(tt.give)
		if got := int(mole_last_error_kind()); got != tt.want {
			t.Errorf("mole_last_error_kind after fail(%v) = %d, want %d", tt.give, got, tt.want)
		}
	}
}

func TestUTF16Len(t *testing.T) {
	for _, tt := range []struct {
		give string
		want int
	}{
		{give: "", want: 0},
		{give: "ação", want: 4},
		{give: "\U0001D11Ex", want: 3}, // a clef outside the BMP takes a surrogate pair
	} {
		if got := utf16Len(tt.give); got != tt.want {
			t.Errorf("utf16Len(%q) = %d, want %d", tt.give, got, tt.want)
		}
	}
}

// TestJSONShapes pins the field names of the JSON the app decodes.
func TestJSONShapes(t *testing.T) {
	r := rangeJSON{Start: anchorJSON{Chapter: 1, Offset: 2}, End: anchorJSON{Chapter: 1, Offset: 9}}
	tests := []struct {
		name string
		give any
		want string
	}{
		{
			name: "Search",
			give: searchJSON{Done: true, Hits: []hitJSON{{Range: r, Excerpt: "ab", MatchStart: 1, MatchLength: 1}}},
			want: `{"done":true,"truncated":false,"hits":[{"range":{"start":{"chapter":1,"offset":2},"end":{"chapter":1,"offset":9}},"excerpt":"ab","matchStart":1,"matchLength":1}]}`,
		},
		{
			name: "Annotation",
			give: annotationJSON{ID: 3, Kind: "highlight", Range: r, Color: 2, Note: "n", Text: "t", CreatedAt: 10, UpdatedAt: 11},
			want: `{"id":3,"kind":"highlight","range":{"start":{"chapter":1,"offset":2},"end":{"chapter":1,"offset":9}},"color":2,"note":"n","text":"t","createdAt":10,"updatedAt":11}`,
		},
		{
			name: "Book",
			give: toBookJSON(library.Book{ID: 1, Path: "/lib/Books/a-1.epub", OriginalPath: "/dl/a.epub", Title: "A",
				Position: library.Position{Fraction: 0.5}, OpenedAt: time.Unix(3, 0), AddedAt: time.Unix(2, 0)}),
			want: `{"id":1,"path":"/lib/Books/a-1.epub","originalPath":"/dl/a.epub","title":"A","author":"","fraction":0.5,"openedAt":3,"addedAt":2,"collections":[],"missing":false}`,
		},
		{
			name: "BookInCollections",
			give: toBookJSON(library.Book{ID: 2, Path: "/b", Title: "B", AddedAt: time.Unix(2, 0), Collections: []int64{4, 7}, Missing: true}),
			want: `{"id":2,"path":"/b","originalPath":"","title":"B","author":"","fraction":0,"openedAt":0,"addedAt":2,"collections":[4,7],"missing":true}`,
		},
		{
			name: "Collection",
			give: collectionJSON{ID: 4, Name: "Essays", Folder: "/books/Essays"},
			want: `{"id":4,"name":"Essays","folder":"/books/Essays"}`,
		},
		{
			name: "Folder",
			give: folderJSON{ID: 1, Path: "/books", Collections: true, AddedAt: 5, SyncedAt: 6},
			want: `{"id":1,"path":"/books","collections":true,"addedAt":5,"syncedAt":6}`,
		},
		{
			name: "FolderResult",
			give: toFolderResultJSON(importer.FolderResult{Folder: 1, Found: 3, Added: []int64{9},
				Failed: []importer.Failure{
					{Path: "/books/x.epub", Err: errors.New("not a zip")},
					{Path: "/books/y.mobi", Err: fmt.Errorf("import /books/y.mobi: %w", bookfile.ErrDRM)},
				}}),
			want: `{"folder":1,"found":3,"added":[9],"updated":[],"missing":[],"failed":[` +
				`{"path":"/books/x.epub","error":"not a zip","drm":false},` +
				`{"path":"/books/y.mobi","error":"import /books/y.mobi: this book is protected by DRM","drm":true}]}`,
		},
		{
			name: "Settings",
			give: settingsJSON{Level: 0.5, EDR: true, Gamma: 2.2, FontSize: 21, Accent: "#112233", Images: 2},
			want: `{"level":0.5,"edr":true,"tint":0,"gamma":2.2,"fontSize":21,"spread":0,"accent":"#112233","images":2}`,
		},
		{
			name: "Paragraph",
			give: paragraphJSON{Range: r, Text: "Some text.", Continued: true},
			want: `{"range":{"start":{"chapter":1,"offset":2},"end":{"chapter":1,"offset":9}},"text":"Some text.","continued":true}`,
		},
		{
			name: "Rect",
			give: rectJSON{X: 1, Y: 2, Width: 3, Height: 4.5},
			want: `{"x":1,"y":2,"width":3,"height":4.5}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.give)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got, want any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", data, err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatalf("Unmarshal(want) error: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("JSON = %s, want %s", data, tt.want)
			}
		})
	}
}

func TestFolderOptionsJSON(t *testing.T) {
	var got folderOptionsJSON
	if err := json.Unmarshal([]byte(`{"collections":true,"watch":true}`), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if want := (folderOptionsJSON{Collections: true, Watch: true}); got != want {
		t.Errorf("folder options = %+v, want %+v", got, want)
	}
}
