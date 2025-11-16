## novfmt

`novfmt` is a lightweight Go CLI for EPUB maintenance. Currently it has the following commands:

- `merge`: build an omnibus EPUB from multiple single volumes while keeping assets and reading order intact.
- `edit-meta`: tweak the metadata or navigation of an existing EPUB without cracking open an editor.

### Build

```sh
cd /path/to/novfmt
go build ./cmd/novfmt
```

## Merge volumes

```sh
novfmt merge \
  -out omnibus.epub \
  -title "My Favorite Saga" \
  -list volumes.txt \
  -dir /path/to/extra-volumes \
  -creator "Primary Author" \
  volume01.epub volume02.epub
```

Key merge flags:
- `-out` / `-o`: Output EPUB path (default `merged.epub`)
- `-title` / `-t`: Override combined title (defaults to first volume metadata)
- `-lang`: Force resulting language code
- `-creator` / `-c`: Repeatable override for creator credits
- `-list`: Append newline-separated entries from text files (ignores blank lines / `#` comments)
- `-dir`: Scan directories for `.epub` files; entries are sorted numerically when possible

Under the hood the command extracts each volume into a temp workspace, rewrites manifest/spine IDs (`OEBPS/Volumes/vXXXX/...`), regenerates navigation, and zips the result with a spec-compliant mimetype entry.

## Edit metadata

```sh
novfmt edit-meta \
  -title "New Title" \
  -lang en \
  -creator "Author A" -creator "Author B" \
  -nav nav.xhtml \
  -dump-meta current-meta.json \
  book.epub
```

Useful edit flags:
- `-title`, `-lang`, `-identifier`, `-description`, repeatable `-creator`
- `-meta-json`: apply a JSON patch (`{"title":"...", "creators":["..."]}`)
- `-dump-meta`: write the current metadata snapshot to JSON
- `-nav`: replace the navigation doc from a file; `-dump-nav` saves the existing one
- `-out`: write edits to a new EPUB instead of modifying in place
- `-no-touch-modified`: skip refreshing `dcterms:modified` (touching is the default)

The command can also operate in “dump only” mode if you just want the nav or metadata JSON.

## Future work

- Additional subcommands (string replacement, FB2 conversion, asset cleanup)
- Smarter nav merging that preserves per-chapter structure from each source
- Optional parallel extraction and asset deduplication
