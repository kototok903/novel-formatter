package epub

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Volume struct {
	Index       int
	SourcePath  string
	TempDir     string
	RootDir     string
	PackagePath string
	PackageDir  string
	PackageDoc  *PackageDocument
	NavHref     string
	NavItems    []NavItem
	DisplayName string
	Prefix      string
	FirstHref   string
	CoverID     string
}

func loadVolume(ctx context.Context, idx int, source string) (*Volume, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "novfmt-volume-*")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}

	cleanup := func(err error) (*Volume, error) {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return cleanup(err)
	}

	if err := unzip(source, tmpDir); err != nil {
		return cleanup(fmt.Errorf("extract %s: %w", source, err))
	}

	containerPath := filepath.Join(tmpDir, "META-INF", "container.xml")
	if err := ctx.Err(); err != nil {
		return cleanup(err)
	}

	data, err := os.ReadFile(containerPath)
	if err != nil {
		return cleanup(fmt.Errorf("read container.xml: %w", err))
	}

	var root containerRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return cleanup(fmt.Errorf("parse container.xml: %w", err))
	}

	if len(root.Rootfiles) == 0 {
		return cleanup(fmt.Errorf("container missing rootfile"))
	}

	pkgRel := filepath.Clean(root.Rootfiles[0].FullPath)
	pkgPath := filepath.Join(tmpDir, filepath.FromSlash(pkgRel))
	if err := ctx.Err(); err != nil {
		return cleanup(err)
	}

	pkgBytes, err := os.ReadFile(pkgPath)
	if err != nil {
		return cleanup(fmt.Errorf("read package %s: %w", pkgRel, err))
	}

	var pkg PackageDocument
	if err := xml.Unmarshal(pkgBytes, &pkg); err != nil {
		return cleanup(fmt.Errorf("parse package: %w", err))
	}

	var navHref string
	for _, item := range pkg.Manifest.Items {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}

	var coverID string
	for _, meta := range pkg.Metadata.Meta {
		if strings.EqualFold(meta.Name, "cover") && strings.TrimSpace(meta.Content) != "" {
			coverID = strings.TrimSpace(meta.Content)
			break
		}
	}
	if coverID == "" {
		for _, item := range pkg.Manifest.Items {
			if hasProperty(item.Properties, "cover-image") {
				coverID = item.ID
				break
			}
		}
	}

	var navItems []NavItem
	if navHref != "" {
		navPath := filepath.Join(filepath.Dir(pkgPath), filepath.FromSlash(navHref))
		items, err := parseNavFile(navPath)
		if err != nil {
			return cleanup(fmt.Errorf("parse nav %s: %w", navHref, err))
		}
		navItems = items
	}

	display := fmt.Sprintf("Volume %d", idx+1)
	if len(pkg.Metadata.Titles) > 0 && strings.TrimSpace(pkg.Metadata.Titles[0].Value) != "" {
		display = pkg.Metadata.Titles[0].Value
	}

	return &Volume{
		Index:       idx,
		SourcePath:  source,
		TempDir:     tmpDir,
		RootDir:     tmpDir,
		PackagePath: pkgPath,
		PackageDir:  filepath.Dir(pkgPath),
		PackageDoc:  &pkg,
		NavHref:     navHref,
		NavItems:    navItems,
		DisplayName: display,
		CoverID:     coverID,
	}, nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dst, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, dst) {
			return fmt.Errorf("zip entry %s escapes destination", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		out.Close()
	}

	return nil
}
