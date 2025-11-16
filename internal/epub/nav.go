package epub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type NavItem struct {
	Title    string
	Href     string
	Children []NavItem
}

type navItemState struct {
	item NavItem
	text strings.Builder
}

func parseNavFile(path string) ([]NavItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseNavDocument(data)
}

func parseNavDocument(data []byte) ([]NavItem, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	var (
		items     []NavItem
		listStack []*[]NavItem
		liStack   []*navItemState
		inTOC     bool
		navDepth  int
	)

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "nav" {
				if !inTOC && hasTOCTypeAttr(t.Attr) {
					inTOC = true
					navDepth = 1
					continue
				}
				if inTOC {
					navDepth++
				}
				continue
			}
			if !inTOC {
				continue
			}
			switch t.Name.Local {
			case "ol":
				var target *[]NavItem
				if len(liStack) > 0 {
					target = &liStack[len(liStack)-1].item.Children
				} else {
					target = &items
				}
				listStack = append(listStack, target)
			case "li":
				state := &navItemState{}
				liStack = append(liStack, state)
			case "a":
				if len(liStack) == 0 {
					continue
				}
				curr := liStack[len(liStack)-1]
				if curr.item.Href != "" {
					continue
				}
				for _, attr := range t.Attr {
					if attr.Name.Local == "href" {
						curr.item.Href = strings.TrimSpace(attr.Value)
						break
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "nav" && inTOC {
				navDepth--
				if navDepth == 0 {
					inTOC = false
					return items, nil
				}
				continue
			}
			if !inTOC {
				continue
			}
			switch t.Name.Local {
			case "ol":
				if len(listStack) > 0 {
					listStack = listStack[:len(listStack)-1]
				}
			case "li":
				if len(liStack) == 0 {
					continue
				}
				idx := len(liStack) - 1
				state := liStack[idx]
				liStack = liStack[:idx]
				title := normalizeSpace(state.text.String())
				state.item.Title = title
				target := &items
				if len(listStack) > 0 {
					target = listStack[len(listStack)-1]
				}
				*target = append(*target, state.item)
			}
		case xml.CharData:
			if !inTOC || len(liStack) == 0 {
				continue
			}
			liStack[len(liStack)-1].text.WriteString(string(t))
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("toc nav not found")
	}

	return items, nil
}

func hasTOCTypeAttr(attrs []xml.Attr) bool {
	const navNS = "http://www.idpf.org/2007/ops"
	for _, attr := range attrs {
		if attr.Name.Local != "type" {
			continue
		}
		if attr.Name.Space != "" && attr.Name.Space != navNS {
			continue
		}
		for _, token := range strings.Fields(attr.Value) {
			if token == "toc" {
				return true
			}
		}
	}
	return false
}

func normalizeSpace(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func joinHref(prefix, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "#") || strings.Contains(href, "://") {
		return href
	}
	parts := strings.SplitN(href, "#", 2)
	base := parts[0]
	if base == "" && len(parts) == 2 {
		return "#" + parts[1]
	}
	joined := normalizeEPUBPath(path.Join(prefix, base))
	if len(parts) == 2 {
		joined = joined + "#" + parts[1]
	}
	return joined
}
