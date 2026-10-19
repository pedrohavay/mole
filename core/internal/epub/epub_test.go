package epub

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeEPUB builds a minimal EPUB with the given package document and files.
func writeEPUB(t *testing.T, opf string, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name, content string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	add("META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)
	add("OEBPS/content.opf", opf)
	for name, content := range files {
		add(name, content)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write epub: %v", err)
	}
	return path
}

func TestInspect(t *testing.T) {
	tests := []struct {
		name      string
		opf       string
		files     map[string]string
		wantTitle string
		wantAuth  string
		wantCover string
	}{
		{
			name: "EPUB3CoverImage",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>  Grokking   CD </dc:title><dc:creator>Christie Wilson</dc:creator></metadata>
  <manifest><item id="c" href="images/cover%20art.jpg" media-type="image/jpeg" properties="cover-image"/></manifest>
</package>`,
			files:     map[string]string{"OEBPS/images/cover art.jpg": "JPEG"},
			wantTitle: "Grokking CD",
			wantAuth:  "Christie Wilson",
			wantCover: "JPEG",
		},
		{
			name: "EPUB2CoverMeta",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>Old</dc:title><meta name="cover" content="img"/></metadata>
  <manifest><item id="img" href="../cover.png" media-type="image/png"/></manifest>
</package>`,
			files:     map[string]string{"cover.png": "PNG"},
			wantTitle: "Old",
			wantCover: "PNG",
		},
		{
			name: "EPUB2CoverMetaByHref",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>Href</dc:title><meta name="cover" content="img/c.jpg"/></metadata>
  <manifest><item id="x" href="img/c.jpg" media-type="image/jpeg"/></manifest>
</package>`,
			files:     map[string]string{"OEBPS/img/c.jpg": "JPEG"},
			wantTitle: "Href",
			wantCover: "JPEG",
		},
		{
			name: "GuideCoverPage",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>Guide</dc:title></metadata>
  <manifest><item id="p" href="text/cover.xhtml" media-type="application/xhtml+xml"/></manifest>
  <guide><reference type="Cover" href="text/cover.xhtml#top"/></guide>
</package>`,
			files: map[string]string{
				"OEBPS/text/cover.xhtml":      `<html><body><p>Cover &nbsp;<br></p><img src="../images/front%20page.png" alt=""/></body></html>`,
				"OEBPS/images/front page.png": "PNG",
			},
			wantTitle: "Guide",
			wantCover: "PNG",
		},
		{
			name: "SVGCoverPage",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>SVG</dc:title><meta name="cover" content="cover-page"/></metadata>
  <manifest><item id="cover-page" href="cover.xhtml" media-type="application/xhtml+xml"/></manifest>
</package>`,
			files: map[string]string{
				"OEBPS/cover.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:xlink="http://www.w3.org/1999/xlink"><body>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 800"><image width="600" height="800" xlink:href="c.png"/></svg></body></html>`,
				"OEBPS/c.png": "SVG PNG",
			},
			wantTitle: "SVG",
			wantCover: "SVG PNG",
		},
		{
			name: "UnreadableCover",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>Missing</dc:title></metadata>
  <manifest><item id="c" href="gone.jpg" media-type="image/jpeg" properties="cover-image"/></manifest>
  <guide><reference type="cover" href="also-gone.xhtml"/></guide>
</package>`,
			wantTitle: "Missing",
		},
		{
			name: "Latin1Package",
			opf: "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n" +
				`<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
				"<metadata><dc:title>A\xe7\xe3o</dc:title><dc:creator>Jos\xe9</dc:creator></metadata></package>",
			wantTitle: "Ação",
			wantAuth:  "José",
		},
		{
			name: "Windows1252Package",
			opf: "<?xml version=\"1.0\" encoding=\"windows-1252\"?>\n" +
				`<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
				"<metadata><dc:title>\x93Quoted\x94</dc:title></metadata></package>",
			wantTitle: "\u201cQuoted\u201d",
		},
		{
			name: "NoCover",
			opf: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata><dc:title>Plain</dc:title></metadata>
  <manifest><item id="t" href="text.xhtml" media-type="application/xhtml+xml"/></manifest>
</package>`,
			wantTitle: "Plain",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Inspect(writeEPUB(t, tt.opf, tt.files))
			if err != nil {
				t.Fatalf("Inspect error: %v", err)
			}
			if got.Title != tt.wantTitle || got.Author != tt.wantAuth || string(got.Cover) != tt.wantCover {
				t.Errorf("Inspect = {%q, %q, %q}, want {%q, %q, %q}",
					got.Title, got.Author, got.Cover, tt.wantTitle, tt.wantAuth, tt.wantCover)
			}
		})
	}
}

func TestInspectRejectsNonEPUB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(path); err == nil {
		t.Error("Inspect(text file): got nil error, want error")
	}
}
