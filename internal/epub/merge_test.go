package epub

import "testing"

func TestBuildPackageDefaults(t *testing.T) {
	vols := []*Volume{
		{
			DisplayName: "Vol 1",
			PackageDoc: &PackageDocument{
				Metadata: Metadata{
					Titles:    []DCMeta{{Value: "Source Title"}},
					Languages: []DCMeta{{Value: "ja"}},
					Creators:  []DCMeta{{Value: "Author B"}},
				},
			},
		},
		{
			DisplayName: "Vol 2",
			PackageDoc: &PackageDocument{
				Metadata: Metadata{
					Creators: []DCMeta{{Value: "Author A"}},
				},
			},
		},
	}

	pkg := buildPackage(vols, Manifest{}, Spine{}, MergeOptions{}, "")

	if got := pkg.Metadata.Titles[0].Value; got != "Source Title" {
		t.Fatalf("title mismatch: %q", got)
	}
	if got := pkg.Metadata.Languages[0].Value; got != "ja" {
		t.Fatalf("language mismatch: %q", got)
	}
	wantCreators := []string{"Author A", "Author B"}
	if len(pkg.Metadata.Creators) != len(wantCreators) {
		t.Fatalf("creator count mismatch: %d", len(pkg.Metadata.Creators))
	}
	for i, want := range wantCreators {
		if pkg.Metadata.Creators[i].Value != want {
			t.Fatalf("creator[%d]=%q want %q", i, pkg.Metadata.Creators[i].Value, want)
		}
	}
	if pkg.Metadata.Identifiers[0].ID != "bookid" {
		t.Fatalf("identifier id mismatch: %s", pkg.Metadata.Identifiers[0].ID)
	}
	if pkg.UniqueIdentifier != "bookid" {
		t.Fatalf("unique identifier mismatch: %s", pkg.UniqueIdentifier)
	}
}

func TestNormalizeEPUBPath(t *testing.T) {
	cases := map[string]string{
		"foo\\bar\\baz.xhtml":      "foo/bar/baz.xhtml",
		"foo/../bar/chapter.xhtml": "bar/chapter.xhtml",
		"./nav.xhtml":              "nav.xhtml",
	}

	for in, want := range cases {
		if got := normalizeEPUBPath(in); got != want {
			t.Fatalf("normalize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHasProperty(t *testing.T) {
	if !hasProperty("nav cover", "nav") {
		t.Fatalf("expected property nav")
	}
	if hasProperty("nav cover", "ava") {
		t.Fatalf("unexpected partial match")
	}
}
