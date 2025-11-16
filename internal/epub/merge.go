package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func MergeEPUBs(ctx context.Context, sources []string, opts MergeOptions) error {
	if len(sources) < 2 {
		return fmt.Errorf("need at least two input EPUB files")
	}

	if opts.OutPath == "" {
		return fmt.Errorf("output path is required")
	}

	volumes := make([]*Volume, len(sources))
	for i, src := range sources {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		vol, err := loadVolume(ctx, i, src)
		if err != nil {
			for _, v := range volumes {
				if v != nil {
					os.RemoveAll(v.TempDir)
				}
			}
			return err
		}
		volumes[i] = vol
	}
	defer func() {
		for _, v := range volumes {
			os.RemoveAll(v.TempDir)
		}
	}()

	stageDir, err := os.MkdirTemp("", "novfmt-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	oebpsDir := filepath.Join(stageDir, "OEBPS")
	if err := os.MkdirAll(oebpsDir, 0o755); err != nil {
		return err
	}

	manifest := Manifest{}
	spine := Spine{}
	idHref := make(map[string]string)
	var coverItemID string

	for _, vol := range volumes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		vol.Prefix = path.Join("Volumes", fmt.Sprintf("v%04d", vol.Index+1))
		destDir := filepath.Join(oebpsDir, filepath.FromSlash(vol.Prefix))
		if err := copyVolumePayload(vol, destDir); err != nil {
			return fmt.Errorf("%s: %w", vol.SourcePath, err)
		}

		idMap := make(map[string]string)

		for _, item := range vol.PackageDoc.Manifest.Items {
			if hasProperty(item.Properties, "nav") {
				continue
			}
			newID := fmt.Sprintf("v%04d_%s", vol.Index+1, item.ID)
			idMap[item.ID] = newID
			href := normalizeEPUBPath(path.Join(vol.Prefix, item.Href))
			entry := ManifestItem{
				ID:         newID,
				Href:       href,
				MediaType:  item.MediaType,
				Properties: item.Properties,
			}
			if item.Fallback != "" {
				entry.Fallback = fmt.Sprintf("v%04d_%s", vol.Index+1, item.Fallback)
			}
			if coverItemID == "" {
				switch {
				case vol.CoverID != "" && item.ID == vol.CoverID:
					entry.Properties = addProperty(entry.Properties, "cover-image")
					coverItemID = newID
				case vol.CoverID == "" && hasProperty(item.Properties, "cover-image"):
					entry.Properties = addProperty(entry.Properties, "cover-image")
					coverItemID = newID
				}
			}
			manifest.Items = append(manifest.Items, entry)
			idHref[newID] = href
		}

		if spine.PageProgressionDirection == "" && vol.PackageDoc.Spine.PageProgressionDirection != "" {
			spine.PageProgressionDirection = vol.PackageDoc.Spine.PageProgressionDirection
		}

		for _, ref := range vol.PackageDoc.Spine.Itemrefs {
			newID, ok := idMap[ref.IDRef]
			if !ok {
				continue
			}
			spine.Itemrefs = append(spine.Itemrefs, SpineItemRef{
				IDRef:  newID,
				Linear: ref.Linear,
			})

			if vol.FirstHref == "" {
				vol.FirstHref = idHref[newID]
			}
		}
	}

	manifest.Items = append(manifest.Items, ManifestItem{
		ID:         "nav",
		Href:       "nav.xhtml",
		MediaType:  "application/xhtml+xml",
		Properties: "nav",
	})

	if err := writeNav(volumes, filepath.Join(oebpsDir, "nav.xhtml")); err != nil {
		return err
	}

	pkg := buildPackage(volumes, manifest, spine, opts, coverItemID)
	if err := writePackage(pkg, filepath.Join(oebpsDir, "content.opf")); err != nil {
		return err
	}

	if err := writeContainer(filepath.Join(stageDir, "META-INF")); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(stageDir, "mimetype"), []byte("application/epub+zip"), 0o644); err != nil {
		return err
	}

	if err := writeZip(stageDir, opts.OutPath); err != nil {
		return err
	}

	return nil
}

func buildPackage(vols []*Volume, manifest Manifest, spine Spine, opts MergeOptions, coverID string) *PackageDocument {
	title := opts.Title
	if title == "" && len(vols) > 0 {
		if len(vols[0].PackageDoc.Metadata.Titles) > 0 {
			title = vols[0].PackageDoc.Metadata.Titles[0].Value
		} else {
			title = vols[0].DisplayName
		}
	}
	if title == "" {
		title = "Merged EPUB"
	}

	lang := opts.Language
	if lang == "" && len(vols) > 0 {
		if len(vols[0].PackageDoc.Metadata.Languages) > 0 {
			lang = vols[0].PackageDoc.Metadata.Languages[0].Value
		} else {
			lang = "en"
		}
	}
	if lang == "" {
		lang = "en"
	}

	creators := make([]string, 0, len(opts.Creators))
	if len(opts.Creators) > 0 {
		creators = append(creators, opts.Creators...)
	} else {
		seen := map[string]struct{}{}
		for _, v := range vols {
			for _, c := range v.PackageDoc.Metadata.Creators {
				name := strings.TrimSpace(c.Value)
				if name == "" {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				creators = append(creators, name)
			}
		}
	}
	if len(creators) == 0 {
		creators = []string{"Unknown"}
	}
	sort.Strings(creators)

	identifier := randomURN()

	meta := Metadata{
		Titles: []DCMeta{
			{Value: title},
		},
		Languages: []DCMeta{
			{Value: lang},
		},
		Identifiers: []DCMeta{
			{ID: "bookid", Value: identifier},
		},
	}

	for _, creator := range creators {
		meta.Creators = append(meta.Creators, DCMeta{Value: creator})
	}

	meta.Meta = append(meta.Meta, MetaNode{
		Property: "novfmt:source-count",
		Value:    fmt.Sprintf("%d", len(vols)),
	})
	if coverID != "" {
		meta.Meta = append(meta.Meta, MetaNode{
			Name:    "cover",
			Content: coverID,
		})
	}

	pkg := &PackageDocument{
		XMLNS:            nsOPF,
		XMLNSDC:          nsDC,
		XMLNSOPF:         nsOPF,
		Version:          "3.0",
		UniqueIdentifier: "bookid",
		Lang:             lang,
		Metadata:         meta,
		Manifest:         manifest,
		Spine:            spine,
		Prefix:           "novfmt: https://novfmt.local/vocab#",
	}

	return pkg
}

func writePackage(pkg *PackageDocument, dest string) error {
	data, err := xml.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.Write(data)
	buf.WriteByte('\n')
	return os.WriteFile(dest, buf.Bytes(), 0o644)
}

func writeContainer(metaDir string) error {
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`
	return os.WriteFile(filepath.Join(metaDir, "container.xml"), []byte(container), 0o644)
}

func writeNav(vols []*Volume, dest string) error {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + "\n")
	buf.WriteString("<head><title>Table of Contents</title></head>\n<body>\n")
	buf.WriteString(`<nav epub:type="toc" id="toc">` + "\n")
	buf.WriteString("<h1>Table of Contents</h1>\n<ol>\n")

	for _, vol := range vols {
		entry := buildVolumeNav(vol)
		if entry == nil {
			continue
		}
		writeNavItem(&buf, *entry)
	}

	buf.WriteString("</ol>\n</nav>\n</body>\n</html>\n")
	return os.WriteFile(dest, buf.Bytes(), 0o644)
}

func writeZip(srcDir, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := zipWriter{w: out}
	if err := w.addEPUBTree(srcDir); err != nil {
		return err
	}
	return nil
}

func randomURN() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "urn:uuid:00000000-0000-0000-0000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

func normalizeEPUBPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

func buildVolumeNav(vol *Volume) *NavItem {
	if vol == nil {
		return nil
	}
	if len(vol.NavItems) == 0 && vol.FirstHref == "" {
		return nil
	}
	entry := &NavItem{
		Title: vol.DisplayName,
		Href:  vol.FirstHref,
	}
	if len(vol.NavItems) > 0 {
		entry.Children = cloneNavItems(vol.NavItems, vol.Prefix)
		if entry.Href == "" && len(entry.Children) > 0 {
			entry.Href = entry.Children[0].Href
		}
	}
	return entry
}

func cloneNavItems(items []NavItem, prefix string) []NavItem {
	out := make([]NavItem, 0, len(items))
	for _, item := range items {
		clone := NavItem{
			Title: item.Title,
		}
		if item.Href != "" {
			clone.Href = joinHref(prefix, item.Href)
		}
		if len(item.Children) > 0 {
			clone.Children = cloneNavItems(item.Children, prefix)
		}
		out = append(out, clone)
	}
	return out
}

func writeNavItem(buf *bytes.Buffer, item NavItem) {
	buf.WriteString("<li>")
	label := html.EscapeString(item.Title)
	href := html.EscapeString(item.Href)
	switch {
	case href != "":
		buf.WriteString(`<a href="` + href + `">`)
		if label != "" {
			buf.WriteString(label)
		} else {
			buf.WriteString(href)
		}
		buf.WriteString("</a>")
	case label != "":
		buf.WriteString(label)
	}
	if len(item.Children) > 0 {
		buf.WriteString("\n<ol>\n")
		for _, child := range item.Children {
			writeNavItem(buf, child)
		}
		buf.WriteString("</ol>\n")
	}
	buf.WriteString("</li>\n")
}

func copyVolumePayload(vol *Volume, dst string) error {
	pkgRel := filepath.Base(vol.PackagePath)
	navRel := path.Clean(filepath.ToSlash(vol.NavHref))
	return filepath.Walk(vol.PackageDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(vol.PackageDir, p)
		if err != nil {
			return err
		}
		if rel == pkgRel {
			return nil
		}
		relSlash := path.Clean(filepath.ToSlash(rel))
		if navRel != "" && relSlash == navRel {
			return nil
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(p, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

type zipWriter struct {
	w io.Writer
}

func (zw *zipWriter) addEPUBTree(root string) error {
	writer := zip.NewWriter(zw.w)

	mimePath := filepath.Join(root, "mimetype")
	mimeData, err := os.ReadFile(mimePath)
	if err != nil {
		writer.Close()
		return err
	}

	mimeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mimeHeader.SetMode(0o644)
	mimeWriter, err := writer.CreateHeader(mimeHeader)
	if err != nil {
		writer.Close()
		return err
	}
	if _, err := mimeWriter.Write(mimeData); err != nil {
		writer.Close()
		return err
	}

	if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "mimetype" {
			return nil
		}
		header := &zip.FileHeader{
			Name:   filepath.ToSlash(rel),
			Method: zip.Deflate,
		}
		header.SetMode(info.Mode())
		w, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
		return nil
	}); err != nil {
		writer.Close()
		return err
	}

	return writer.Close()
}
