package epub

import "testing"

func TestParseNavDocument(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
  <nav epub:type="toc">
    <ol>
      <li><a href="cover.xhtml">Cover</a></li>
      <li>
        <span>Volume 1</span>
        <ol>
          <li><a href="chapter.xhtml#p1">Part 1</a></li>
        </ol>
      </li>
    </ol>
  </nav>
</body>
</html>`)
	items, err := parseNavDocument(data)
	if err != nil {
		t.Fatalf("parse nav: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0].Title != "Cover" || items[0].Href != "cover.xhtml" {
		t.Fatalf("unexpected first item %+v", items[0])
	}
	if items[1].Title != "Volume 1" {
		t.Fatalf("unexpected second title %q", items[1].Title)
	}
	if len(items[1].Children) != 1 {
		t.Fatalf("expected child entry")
	}
	child := items[1].Children[0]
	if child.Title != "Part 1" || child.Href != "chapter.xhtml#p1" {
		t.Fatalf("unexpected child %+v", child)
	}
}

func TestJoinHref(t *testing.T) {
	cases := map[string]struct {
		prefix string
		href   string
		want   string
	}{
		"simple":        {"Volumes/v0001", "chapter.xhtml", "Volumes/v0001/chapter.xhtml"},
		"fragment":      {"Volumes/v0001", "chapter.xhtml#s1", "Volumes/v0001/chapter.xhtml#s1"},
		"anchor-only":   {"Volumes/v0001", "#frag", "#frag"},
		"absolute-http": {"Volumes/v0001", "https://example.com/foo", "https://example.com/foo"},
	}

	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			if got := joinHref(tc.prefix, tc.href); got != tc.want {
				t.Fatalf("joinHref(%q,%q)=%q want %q", tc.prefix, tc.href, got, tc.want)
			}
		})
	}
}
