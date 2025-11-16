package epub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditEPUBMetadata(t *testing.T) {
	input := buildTestEPUB(t, "Old Title", "en")
	defer os.Remove(input)

	title := "New Title"
	lang := "ja"
	id := "urn:test:new"
	desc := "updated description"
	creators := []string{"Author A", "Author B"}

	opts := EditOptions{
		OutPath: input,
		MetadataPatch: MetadataPatch{
			Title:       &title,
			Language:    &lang,
			Identifier:  &id,
			Description: &desc,
			Creators:    &creators,
		},
		TouchModified: false,
	}

	if err := EditEPUB(context.Background(), input, opts); err != nil {
		t.Fatalf("EditEPUB: %v", err)
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	meta := vol.PackageDoc.Metadata
	if got := firstDCValue(meta.Titles); got != title {
		t.Fatalf("title = %q", got)
	}
	if got := firstDCValue(meta.Languages); got != lang {
		t.Fatalf("language = %q", got)
	}
	if got := firstDCValue(meta.Identifiers); got != id {
		t.Fatalf("identifier = %q", got)
	}
	if got := firstDCValue(meta.Descriptions); got != desc {
		t.Fatalf("description = %q", got)
	}
	if len(meta.Creators) != len(creators) {
		t.Fatalf("creator count = %d", len(meta.Creators))
	}
	for i, want := range creators {
		if meta.Creators[i].Value != want {
			t.Fatalf("creator[%d]=%q", i, meta.Creators[i].Value)
		}
	}
}

func TestEditEPUBReplaceNav(t *testing.T) {
	input := buildTestEPUB(t, "Title", "en")
	defer os.Remove(input)

	newNav := `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol><li><a href="chapter.xhtml">Chapter</a></li></ol></nav></body></html>`
	tmpNav := filepath.Join(t.TempDir(), "nav.xhtml")
	if err := os.WriteFile(tmpNav, []byte(newNav), 0o644); err != nil {
		t.Fatalf("write tmp nav: %v", err)
	}

	opts := EditOptions{
		OutPath:        input,
		NavReplacePath: tmpNav,
		TouchModified:  false,
	}

	if err := EditEPUB(context.Background(), input, opts); err != nil {
		t.Fatalf("EditEPUB: %v", err)
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	navPath := filepath.Join(filepath.Dir(vol.PackagePath), filepath.FromSlash(vol.NavHref))
	data, err := os.ReadFile(navPath)
	if err != nil {
		t.Fatalf("read nav: %v", err)
	}
	if strings.TrimSpace(string(data)) != newNav {
		t.Fatalf("nav not replaced")
	}
}

func buildTestEPUB(t *testing.T, title, lang string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "mimetype"), []byte("application/epub+zip"), 0o644); err != nil {
		t.Fatalf("write mimetype: %v", err)
	}

	metaDir := filepath.Join(root, "META-INF")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`
	if err := os.WriteFile(filepath.Join(metaDir, "container.xml"), []byte(container), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}

	oebps := filepath.Join(root, "OEBPS")
	if err := os.MkdirAll(oebps, 0o755); err != nil {
		t.Fatalf("mkdir oebps: %v", err)
	}

	nav := `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc" id="toc"><ol><li><a href="chapter.xhtml">Chapter</a></li></ol></nav></body></html>`
	if err := os.WriteFile(filepath.Join(oebps, "nav.xhtml"), []byte(nav), 0o644); err != nil {
		t.Fatalf("write nav: %v", err)
	}

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookId" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>%s</dc:title>
    <dc:language>%s</dc:language>
    <dc:identifier id="BookId">urn:test:old</dc:identifier>
    <dc:description>orig</dc:description>
    <meta property="dcterms:modified">2020-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="chap" href="chapter.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chap"/>
  </spine>
</package>
`, title, lang)

	if err := os.WriteFile(filepath.Join(oebps, "content.opf"), []byte(content), 0o644); err != nil {
		t.Fatalf("write opf: %v", err)
	}

	if err := os.WriteFile(filepath.Join(oebps, "chapter.xhtml"), []byte("<html><body><p>Chapter 1</p></body></html>"), 0o644); err != nil {
		t.Fatalf("write chapter: %v", err)
	}

	outFile := filepath.Join(t.TempDir(), "test.epub")
	if err := writeZip(root, outFile); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	return outFile
}
