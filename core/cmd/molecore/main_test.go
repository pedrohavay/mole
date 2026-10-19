package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

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
			give: bookJSON{ID: 1, Path: "/lib/Books/a-1.epub", OriginalPath: "/dl/a.epub", Title: "A", Fraction: 0.5, OpenedAt: 3, AddedAt: 2},
			want: `{"id":1,"path":"/lib/Books/a-1.epub","originalPath":"/dl/a.epub","title":"A","author":"","fraction":0.5,"openedAt":3,"addedAt":2}`,
		},
		{
			name: "Settings",
			give: settingsJSON{Level: 0.5, EDR: true, Gamma: 2.2, FontSize: 21, Accent: "#112233", Pictures: 1, BookColors: true},
			want: `{"level":0.5,"edr":true,"tint":0,"gamma":2.2,"fontSize":21,"spread":0,"accent":"#112233","pictures":1,"bookColors":true}`,
		},
		{
			name: "Picture",
			give: pictureJSON{Page: 4, Index: 1, Rect: rectJSON{X: 1, Y: 2, Width: 3, Height: 4}},
			want: `{"page":4,"index":1,"rect":{"x":1,"y":2,"width":3,"height":4}}`,
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
