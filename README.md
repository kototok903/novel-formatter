# novfmt

A lightweight Go CLI for EPUB maintenance: merge volumes, edit metadata and navigation, search/replace text.

## Build

```sh
go build ./cmd/novfmt
```

## Commands

- **merge** — combine multiple EPUB volumes into one omnibus file
- **edit-meta** — view or modify metadata and navigation
- **rewrite** — search/replace text (and optionally metadata)

Run `novfmt -h` or `novfmt <command> -h` for the full flag reference.

> **Note:** `edit-meta` and `rewrite` modify the input file in place by default. Use `-out` to write to a new file instead.

## Example workflows

### Merging a multi-volume series

Download all volumes into a folder, making sure filenames contain volume numbers so they sort correctly (e.g. `vol01.epub`, `vol02.epub`, …). Then:

```sh
novfmt merge \
  -dir ./my-series \
  -title "My Favorite Saga" \
  -o saga.epub
```

Files in `-dir` are sorted numerically by the first number in each filename.

### Fixing metadata and navigation after a merge

Dump the current metadata and nav to temporary files:

```sh
novfmt edit-meta \
  -dump-meta meta.json \
  -dump-nav nav.xhtml \
  saga.epub
```

Open `meta.json` in a text editor — fix the language, add a description, correct the title (useful for translated books where the title may be in the wrong language):

```json
{
  "title": "Corrected Title",
  "language": "en",
  "description": "A short summary of the book.",
  "creators": ["Author Name"]
}
```

Edit `nav.xhtml` to fix the hierarchy — nest chapters under their volumes, group subchapters (2.1, 2.2, 2.3) under their parent chapter, etc.

Apply both changes and write to a new file:

```sh
novfmt edit-meta \
  -meta-json meta.json \
  -nav nav.xhtml \
  -out saga-fixed.epub \
  saga.epub
```

### Search/replace text

Rename a character across the entire book:

```sh
novfmt rewrite \
  -find "Old Name" \
  -replace "New Name" \
  -out fixed.epub \
  book.epub
```

Preview changes without writing anything:

```sh
novfmt rewrite \
  -find "typo" \
  -replace "correction" \
  -dry-run \
  book.epub
```

Apply multiple rules from a JSON file:

```sh
novfmt rewrite -rules fixes.json book.epub
```

## Future work

- FB2 conversion, asset cleanup
- Smarter nav merging that preserves per-chapter structure from each source
- Parallel extraction and asset deduplication
