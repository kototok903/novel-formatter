## novfmt

`novfmt` is a lightweight Go CLI for EPUB maintenance. The initial release focuses on merging multiple single-volume EPUB files (text + images) into one omnibus volume while preserving reading order.

### Build

```sh
cd /Users/stas/Desktop/novel-formatter
go build ./cmd/novfmt
```

### Usage

```sh
novfmt merge \
  -out omnibus.epub \
  -title "My Favorite Saga" \
  -list volumes.txt \
  -dir /path/to/extra-volumes \
  -creator "Primary Author" \
  volume01.epub volume02.epub
```

Flags:
- `-out` / `-o`: Output EPUB path (default `merged.epub`)
- `-title` / `-t`: Override combined title (defaults to first volume metadata)
- `-lang`: Force resulting language code
- `-creator` / `-c`: Repeatable override for creator credits
- `-list`: Append newline-separated entries from one or more files (blank lines and lines starting with `#` are ignored)
- `-dir`: Scan directories for `.epub` files; entries are sorted using the first number found in the filename (if any), falling back to case-insensitive alphabetical order

The tool extracts each volume into a temporary workspace, rewrites the manifest/spine with stable IDs, copies assets under `OEBPS/Volumes/vXXXX`, generates a fresh navigation document, and zips everything back into a valid EPUB (storing the mimetype uncompressed as required by the spec).

### Future extensions

- Additional subcommands (e.g., string replacement, FB2 conversion)
- Smarter nav merging that preserves chapter-level entries
- Asset deduplication and optional parallel extraction

