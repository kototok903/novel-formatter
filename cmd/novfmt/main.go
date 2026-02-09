package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/kototok903/novfmt/internal/epub"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "merge":
		err = runMerge(ctx, os.Args[2:])
	case "edit-meta":
		err = runEditMeta(ctx, os.Args[2:])
	case "rewrite":
		err = runRewrite(ctx, os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runMerge(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usageMerge) }

	out := fs.String("out", "merged.epub", "")
	fs.StringVar(out, "o", "merged.epub", "")

	title := fs.String("title", "", "")
	fs.StringVar(title, "t", "", "")

	lang := fs.String("lang", "", "")

	var creatorVals multiValue
	fs.Var(&creatorVals, "creator", "")
	fs.Var(&creatorVals, "c", "")

	var listFiles multiValue
	fs.Var(&listFiles, "list", "")

	var dirInputs multiValue
	fs.Var(&dirInputs, "dir", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	files := fs.Args()

	if len(listFiles) > 0 {
		fromLists, err := expandListFiles(listFiles)
		if err != nil {
			return err
		}
		files = append(files, fromLists...)
	}

	if len(dirInputs) > 0 {
		fromDirs, err := expandDirectories(dirInputs)
		if err != nil {
			return err
		}
		files = append(files, fromDirs...)
	}

	if len(files) < 2 {
		return fmt.Errorf("need at least two EPUB files to merge")
	}

	opts := epub.MergeOptions{
		Title:    *title,
		Language: *lang,
		Creators: creatorVals,
		OutPath:  *out,
	}

	return epub.MergeEPUBs(ctx, files, opts)
}

const usageHeader = `novfmt — lightweight CLI for EPUB maintenance

Usage:
  novfmt <command> [options] <file(s)>
  novfmt <command> -h        show help for a command

Commands:
  merge       combine multiple EPUB volumes into one
  edit-meta   view or modify EPUB metadata and navigation
  rewrite     search/replace text inside an EPUB
`

const usageMerge = `Merge:
  novfmt merge [options] <vol1.epub> <vol2.epub> [...]

  Requires at least 2 input volumes (from any combination of positional
  args, -list, and -dir). Volumes are appended in the order given.

  -o, -out <path>       output file path (default: merged.epub)
  -t, -title <str>      title for the merged book (default: first volume's title)
  -lang <code>          language code, e.g. "en" (default: first volume's language)
  -c, -creator <name>   author credit; repeatable; replaces original creator lists
  -list <file>          text file with one volume path per line; blank lines and
                        lines starting with # are ignored; repeatable
  -dir <path>           directory to scan for .epub files, sorted numerically
                        when filenames contain numbers; repeatable
`

const usageEditMeta = `Edit-meta:
  novfmt edit-meta [options] <book.epub>

  Without -out the input file is modified in place.
  Can run in dump-only mode (just -dump-meta / -dump-nav, no edits).

  -title <str>          set primary title
  -lang <code>          set language code
  -identifier <str>     set primary identifier (e.g. ISBN, UUID)
  -description <str>    set description text
  -creator <name>       author credit; repeatable; replaces existing creator list
  -meta-json <file>     apply metadata patch from a JSON file
                        (format: {"title":"...", "language":"...", "creators":["..."]})
  -dump-meta <file>     export current metadata snapshot as JSON to <file>
  -dump-nav <file>      export current nav document (XHTML) to <file>
  -nav <file>           replace the entire nav document from an XHTML file
  -out <path>           write result to a new file instead of editing in place
  -no-touch-modified    don't update the last-modified timestamp (dcterms:modified)

  CLI flags override values from -meta-json when both are given.
`

const usageRewrite = `Rewrite:
  novfmt rewrite [options] <book.epub>

  Without -out the input file is modified in place.
  At least one of -find or -rules is required.

  -find <str>           literal string to search for (see -regex)
  -replace <str>        replacement text (default: empty string, i.e. delete matches)
  -regex                treat -find as a Go regular expression
  -case-sensitive       make matching case-sensitive (default: case-insensitive)
  -scope <s>            body, meta, or all — limit where rewrites apply (default: body)
  -selector <sel>       CSS-like selector to target elements (e.g. p, .note, p.chapter);
                        repeatable; applies to the -find/-replace rule
  -rules <file>         JSON file with an array of rule objects, each with:
                        find, replace, regex, case_sensitive, selectors
  -dry-run              report match counts without writing any changes
  -out <path>           write result to a new file instead of editing in place
`

const usageExamples = `Examples:
  novfmt merge -o combined.epub vol1.epub vol2.epub vol3.epub
  novfmt merge -title "Full Series" -dir ./volumes -o series.epub
  novfmt edit-meta -title "New Title" -creator "Author" book.epub
  novfmt edit-meta -dump-meta meta.json book.epub
  novfmt rewrite -find "oldname" -replace "newname" book.epub
  novfmt rewrite -rules fixes.json -dry-run book.epub
`

func printUsage() {
	fmt.Fprint(os.Stderr, usageHeader+"\n"+usageMerge+"\n"+usageEditMeta+"\n"+usageRewrite+"\n"+usageExamples)
}

type multiValue []string

func (m *multiValue) String() string {
	return strings.Join(*m, ",")
}

func (m *multiValue) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func expandListFiles(paths []string) ([]string, error) {
	var volumes []string
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", p, err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			volumes = append(volumes, line)
		}
		if err := scanner.Err(); err != nil {
			f.Close()
			return nil, fmt.Errorf("list %s: %w", p, err)
		}
		f.Close()
	}
	return volumes, nil
}

func expandDirectories(dirs []string) ([]string, error) {
	var volumes []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("dir %s: %w", dir, err)
		}
		candidates := make([]dirEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.EqualFold(filepath.Ext(name), ".epub") {
				continue
			}
			num, hasNum := extractVolumeNumber(name)
			candidates = append(candidates, dirEntry{
				path:      filepath.Join(dir, name),
				name:      name,
				number:    num,
				hasNumber: hasNum,
			})
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			a := candidates[i]
			b := candidates[j]
			if a.hasNumber && b.hasNumber {
				if a.number != b.number {
					return a.number < b.number
				}
				return strings.ToLower(a.name) < strings.ToLower(b.name)
			}
			if a.hasNumber != b.hasNumber {
				return a.hasNumber
			}
			an := strings.ToLower(a.name)
			bn := strings.ToLower(b.name)
			if an == bn {
				return a.name < b.name
			}
			return an < bn
		})
		for _, c := range candidates {
			volumes = append(volumes, c.path)
		}
	}
	return volumes, nil
}

type dirEntry struct {
	path      string
	name      string
	number    int
	hasNumber bool
}

var digitPattern = regexp.MustCompile(`\d+`)

func extractVolumeNumber(name string) (int, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	match := digitPattern.FindString(base)
	if match == "" {
		return 0, false
	}
	num, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	return num, true
}

func runRewrite(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rewrite", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usageRewrite) }

	out := fs.String("out", "", "")
	find := fs.String("find", "", "")
	replace := fs.String("replace", "", "")
	regex := fs.Bool("regex", false, "")
	caseSensitive := fs.Bool("case-sensitive", false, "")
	scopeStr := fs.String("scope", "body", "")

	var selectors multiValue
	fs.Var(&selectors, "selector", "")

	rulesPath := fs.String("rules", "", "")
	dryRun := fs.Bool("dry-run", false, "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return fmt.Errorf("rewrite requires exactly one EPUB path")
	}
	input := fs.Arg(0)

	var rules []epub.RewriteRule
	if *rulesPath != "" {
		fileRules, err := epub.LoadRewriteRulesJSON(*rulesPath)
		if err != nil {
			return fmt.Errorf("read rules: %w", err)
		}
		rules = append(rules, fileRules...)
	}

	if *find != "" {
		rules = append(rules, epub.RewriteRule{
			Find:          *find,
			Replace:       *replace,
			Regex:         *regex,
			CaseSensitive: *caseSensitive,
			Selectors:     selectors,
		})
	}

	var scope epub.RewriteScope
	switch strings.ToLower(*scopeStr) {
	case "body":
		scope = epub.RewriteScopeBody
	case "meta":
		scope = epub.RewriteScopeMeta
	case "all":
		scope = epub.RewriteScopeAll
	default:
		return fmt.Errorf("invalid scope %q (want body, meta, all)", *scopeStr)
	}

	stats, err := epub.RewriteEPUB(ctx, input, epub.RewriteOptions{
		OutPath: *out,
		Scope:   scope,
		Rules:   rules,
		DryRun:  *dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "rewrite: %d matches across %d files\n", stats.MatchCount, stats.FilesChanged)
	return nil
}

func runEditMeta(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("edit-meta", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usageEditMeta) }

	out := fs.String("out", "", "")
	title := fs.String("title", "", "")
	lang := fs.String("lang", "", "")
	identifier := fs.String("identifier", "", "")
	description := fs.String("description", "", "")

	var creators multiValue
	fs.Var(&creators, "creator", "")

	metaJSONPath := fs.String("meta-json", "", "")
	dumpMeta := fs.String("dump-meta", "", "")
	dumpNav := fs.String("dump-nav", "", "")
	navReplace := fs.String("nav", "", "")
	noTouch := fs.Bool("no-touch-modified", false, "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return fmt.Errorf("edit-meta requires exactly one EPUB path")
	}

	input := fs.Arg(0)

	var patch epub.MetadataPatch
	if *metaJSONPath != "" {
		data, err := os.ReadFile(*metaJSONPath)
		if err != nil {
			return fmt.Errorf("read meta-json: %w", err)
		}
		if err := json.Unmarshal(data, &patch); err != nil {
			return fmt.Errorf("parse meta-json: %w", err)
		}
	}

	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if setFlags["title"] {
		patch.Title = stringPtr(*title)
	}
	if setFlags["lang"] {
		patch.Language = stringPtr(*lang)
	}
	if setFlags["identifier"] {
		patch.Identifier = stringPtr(*identifier)
	}
	if setFlags["description"] {
		patch.Description = stringPtr(*description)
	}
	if len(creators) > 0 {
		list := make([]string, len(creators))
		copy(list, creators)
		patch.Creators = &list
	}

	opts := epub.EditOptions{
		OutPath:        *out,
		NavReplacePath: *navReplace,
		DumpNavPath:    *dumpNav,
		DumpMetaPath:   *dumpMeta,
		MetadataPatch:  patch,
		TouchModified:  !*noTouch,
	}

	return epub.EditEPUB(ctx, input, opts)
}

func stringPtr(s string) *string {
	return &s
}
