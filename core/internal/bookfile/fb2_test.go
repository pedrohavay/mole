package bookfile

import (
	"strings"
	"testing"
)

func TestReadFB2(t *testing.T) {
	tests := []struct {
		name       string
		give       string
		wantTitle  string
		wantAuthor string
	}{
		{
			name: "Windows1251",
			give: "<?xml version=\"1.0\" encoding=\"windows-1251\"?><FictionBook><description><title-info>" +
				"<author><first-name>\xcb\xe5\xe2</first-name><last-name>\xd2\xee\xeb\xf1\xf2\xee\xe9</last-name></author>" +
				"<book-title>\xc2\xee\xe9\xed\xe0 \xe8 \xec\xe8\xf0</book-title></title-info></description><body/></FictionBook>",
			wantTitle:  "Война и мир",
			wantAuthor: "Лев Толстой",
		},
		{
			name: "NicknameAndSecondAuthor",
			give: `<FictionBook><description><title-info><author><nickname>Anon</nickname></author>` +
				`<author><first-name>Second</first-name></author><book-title> Spaced
				Title </book-title></title-info></description><body/></FictionBook>`,
			wantTitle:  "Spaced Title",
			wantAuthor: "Anon",
		},
		{
			name:       "DamagedAfterTheTitle",
			give:       `<FictionBook><description><title-info><book-title>Kept</book-title></title-info></description><body><p>&#xD800;`,
			wantTitle:  "Kept",
			wantAuthor: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info Info
			if err := readFB2(strings.NewReader(tt.give), &info); err != nil {
				t.Fatalf("readFB2 error: %v", err)
			}
			if info.Title != tt.wantTitle || info.Author != tt.wantAuthor {
				t.Errorf("readFB2 = %q by %q, want %q by %q", info.Title, info.Author, tt.wantTitle, tt.wantAuthor)
			}
		})
	}
}
