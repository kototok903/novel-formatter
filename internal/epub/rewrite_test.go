package epub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteEPUBBodySimple(t *testing.T) {
	input := buildTestEPUB(t, "Old Title", "en")
	defer os.Remove(input)

	rules := []RewriteRule{
		{
			Find:    "Chapter",
			Replace: "Section",
		},
	}

	stats, err := RewriteEPUB(context.Background(), input, RewriteOptions{
		OutPath: input,
		Scope:   RewriteScopeBody,
		Rules:   rules,
		DryRun:  false,
	})
	if err != nil {
		t.Fatalf("RewriteEPUB: %v", err)
	}
	if stats.MatchCount == 0 {
		t.Fatalf("expected at least one match")
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	chPath := filepath.Join(filepath.Dir(vol.PackagePath), "chapter.xhtml")
	data, err := os.ReadFile(chPath)
	if err != nil {
		t.Fatalf("read chapter: %v", err)
	}
	if !strings.Contains(string(data), "Section") {
		t.Fatalf("replacement not applied in body")
	}
}

func TestRewriteEPUBMetaScope(t *testing.T) {
	input := buildTestEPUB(t, "Foo Title", "en")
	defer os.Remove(input)

	rules := []RewriteRule{
		{
			Find:    "Foo",
			Replace: "Bar",
		},
	}

	_, err := RewriteEPUB(context.Background(), input, RewriteOptions{
		OutPath: input,
		Scope:   RewriteScopeMeta,
		Rules:   rules,
		DryRun:  false,
	})
	if err != nil {
		t.Fatalf("RewriteEPUB: %v", err)
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	if got := firstDCValue(vol.PackageDoc.Metadata.Titles); got != "Bar Title" {
		t.Fatalf("meta rewrite not applied, title=%q", got)
	}
}

func TestRewriteMetadataSelectorsIgnored(t *testing.T) {
	input := buildTestEPUB(t, "Foo Title", "en")
	defer os.Remove(input)

	rules := []RewriteRule{
		{
			Find:      "Foo",
			Replace:   "Bar",
			Selectors: []string{"p.note"},
		},
	}

	stats, err := RewriteEPUB(context.Background(), input, RewriteOptions{
		OutPath: input,
		Scope:   RewriteScopeMeta,
		Rules:   rules,
	})
	if err != nil {
		t.Fatalf("RewriteEPUB: %v", err)
	}
	if stats.MatchCount != 0 || stats.FilesChanged != 0 {
		t.Fatalf("expected selector-scoped rules to be ignored for metadata, stats=%+v", stats)
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	if got := firstDCValue(vol.PackageDoc.Metadata.Titles); got != "Foo Title" {
		t.Fatalf("metadata should remain unchanged, title=%q", got)
	}
}

func TestRewriteSelectors(t *testing.T) {
	root := t.TempDir()
	content := `<html xmlns="http://www.w3.org/1999/xhtml"><body><p class="keep">Chapter 1</p><p class="change">Chapter 2</p></body></html>`
	p := filepath.Join(root, "test.xhtml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rules := []RewriteRule{
		{
			Find:      "Chapter",
			Replace:   "Section",
			Selectors: []string{"p.change"},
		},
	}

	cr, err := compileRules(rules)
	if err != nil {
		t.Fatalf("compileRules: %v", err)
	}
	matches, changed, out, err := rewriteXHTMLFile(p, cr)
	if err != nil {
		t.Fatalf("rewriteXHTMLFile: %v", err)
	}
	if !changed || matches == 0 {
		t.Fatalf("expected changes")
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "Chapter 1") == false {
		t.Fatalf("first paragraph should be unchanged")
	}
	if !strings.Contains(s, "Section 2") {
		t.Fatalf("second paragraph should be rewritten, got %q", s)
	}
}

func TestRewriteDryRunNoMutation(t *testing.T) {
	input := buildTestEPUB(t, "Old Title", "en")
	defer os.Remove(input)

	rules := []RewriteRule{
		{Find: "Chapter", Replace: "Section"},
	}

	stats, err := RewriteEPUB(context.Background(), input, RewriteOptions{
		OutPath: input,
		Scope:   RewriteScopeBody,
		Rules:   rules,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("RewriteEPUB: %v", err)
	}
	if stats.MatchCount == 0 {
		t.Fatalf("expected matches in dry-run")
	}

	vol, err := loadVolume(context.Background(), 0, input)
	if err != nil {
		t.Fatalf("reopen epub: %v", err)
	}
	defer os.RemoveAll(vol.TempDir)

	chPath := filepath.Join(filepath.Dir(vol.PackagePath), "chapter.xhtml")
	data, err := os.ReadFile(chPath)
	if err != nil {
		t.Fatalf("read chapter: %v", err)
	}
	if strings.Contains(string(data), "Section") {
		t.Fatalf("dry-run should not mutate files")
	}
}
