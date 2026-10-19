package reading

import (
	"testing"

	"mole/core/internal/testbook"
)

func TestLanguage(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "Declared", give: "pt_BR", want: "pt-BR"},
		{name: "Undeclared", give: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := testbook.Book{Language: tt.give, Chapters: []string{"<p>Text.</p>"}}.Write(t)
			s, err := Open(path, Location{Offset: -1})
			if err != nil {
				t.Fatalf("Open error: %v", err)
			}
			t.Cleanup(s.Close)
			if got := s.Language(); got != tt.want {
				t.Errorf("Language() with dc:language %q = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}
