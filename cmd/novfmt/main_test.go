package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandListFiles(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "volumes.txt")
	content := `
# comment
/path/Vol 01.epub

   /path/Vol 02.epub
`
	if err := os.WriteFile(list, []byte(content), 0o644); err != nil {
		t.Fatalf("write list: %v", err)
	}

	out, err := expandListFiles([]string{list})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	want := []string{"/path/Vol 01.epub", "/path/Vol 02.epub"}
	if len(out) != len(want) {
		t.Fatalf("got %d entries want %d", len(out), len(want))
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("entry %d = %q want %q", i, out[i], want[i])
		}
	}
}

func TestExpandListFilesMissing(t *testing.T) {
	if _, err := expandListFiles([]string{"/no/such/file"}); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestExpandDirectoriesOrdering(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"Vol 10.epub",
		"[2] Second.epub",
		"My Saga 001.epub",
		"special.epub",
		"Another.EPUB",
		"notes.txt",
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := expandDirectories([]string{dir})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	wantNames := []string{
		"My Saga 001.epub",
		"[2] Second.epub",
		"Vol 10.epub",
		"Another.EPUB",
		"special.epub",
	}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d files want %d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if filepath.Base(got[i]) != want {
			t.Fatalf("idx %d = %q want %q", i, filepath.Base(got[i]), want)
		}
	}
}

func TestExpandDirectoriesMultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	must := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatalf("write %s/%s: %v", dir, name, err)
		}
	}

	must(dir1, "Vol 01.epub")
	must(dir2, "Vol 02.epub")

	paths, err := expandDirectories([]string{dir1, dir2})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if filepath.Base(paths[0]) != "Vol 01.epub" || filepath.Base(paths[1]) != "Vol 02.epub" {
		t.Fatalf("unexpected order: %v", paths)
	}
}
