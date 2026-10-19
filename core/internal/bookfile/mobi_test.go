package bookfile

import (
	"os"
	"path/filepath"
	"testing"

	"mole/core/internal/testbook"
)

// TestDamagedMOBI checks that a MOBI cut short anywhere in its headers and
// metadata fails cleanly or yields what can be read, without panicking.
func TestDamagedMOBI(t *testing.T) {
	path := testbook.MOBI{Title: "Cut", Author: "A", HTML: "<p>Words.</p>", Cover: coverPNG(t)}.Write(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	dir := t.TempDir()
	for n := 60; n < min(len(data), 700); n++ {
		cut := filepath.Join(dir, "cut.mobi")
		if err := os.WriteFile(cut, data[:n], 0o644); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}
		var info Info
		_ = inspectMOBI(cut, &info) // errors are expected; panics are not
	}
	if got := readEXTH([]byte("EXTH\x00\x00\x00\x20\x00\x00\x00\x09\x00\x00\x00\x64\xff\xff\xff\xff")); len(got) != 0 {
		t.Errorf("readEXTH of a record longer than the block = %v, want nothing", got)
	}
}
