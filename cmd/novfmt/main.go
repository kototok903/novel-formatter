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

	out := fs.String("out", "merged.epub", "output EPUB file")
	fs.StringVar(out, "o", "merged.epub", "alias for -out")

	title := fs.String("title", "", "override merged title")
	fs.StringVar(title, "t", "", "alias for -title")

	lang := fs.String("lang", "", "override merged language code")

	var creatorVals multiValue
	fs.Var(&creatorVals, "creator", "repeatable author credit")
	fs.Var(&creatorVals, "c", "alias for -creator")

	var listFiles multiValue
	fs.Var(&listFiles, "list", "text file containing newline-separated volume paths (repeatable)")

	var dirInputs multiValue
	fs.Var(&dirInputs, "dir", "directory to scan for EPUB files (repeatable)")

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

func printUsage() {
	fmt.Fprintf(os.Stderr, `novfmt — EPUB utilities

Usage:
  novfmt merge [options] <volume1.epub> <volume2.epub> [...]
  novfmt edit-meta [options] <book.epub>
  novfmt rewrite [options] <book.epub>

Merge flags:
  -o, -out        Output EPUB path (default merged.epub)
  -t, -title      Override merged title
  -lang           Override merged language (default first volume)
  -c, -creator    Repeatable author credit override
  -list           Text file listing volumes; can repeat
  -dir            Directory to scan for EPUB files; can repeat

Edit-meta flags:
  -title, -lang, -identifier, -description   Override core metadata fields
  -creator                                   Repeatable creator override
  -meta-json <file>                          Apply JSON metadata patch
  -dump-meta <file>                          Write current metadata snapshot
  -nav <xhtml>                               Replace nav (XHTML) document (use -dump-nav to export)
  -out <file>                                Write edits to a new EPUB
  -no-touch-modified                         Skip touching dcterms:modified

Rewrite flags:
  -find <str>                                String or pattern to search for
  -replace <str>                             Replacement text
  -regex                                     Treat -find as regular expression
  -case-sensitive                            Make matching case-sensitive (default case-insensitive)
  -scope body|meta|all                       Rewrite only XHTML body, metadata, or both (default body)
  -selector <sel>                            CSS-like selector (tag, .class, tag.class); repeatable
  -rules <file>                              JSON rules file (list of {find, replace, ...})
  -dry-run                                   Analyze and report matches without writing output
  -out <file>                                Write rewritten book to a new EPUB
`)
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

	out := fs.String("out", "", "output EPUB path (defaults to input for in-place rewrite)")
	find := fs.String("find", "", "string or pattern to search for")
	replace := fs.String("replace", "", "replacement string")
	regex := fs.Bool("regex", false, "treat find pattern as regular expression")
	caseSensitive := fs.Bool("case-sensitive", false, "make matching case-sensitive (default false)")
	scopeStr := fs.String("scope", "body", "rewrite scope: body, meta, all")

	var selectors multiValue
	fs.Var(&selectors, "selector", "CSS-like selector (tag, .class or tag.class); repeatable")

	rulesPath := fs.String("rules", "", "JSON file with rewrite rules")
	dryRun := fs.Bool("dry-run", false, "analyze and report matches without writing output")

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

	out := fs.String("out", "", "output EPUB path (defaults to input for in-place edits)")
	title := fs.String("title", "", "set primary title")
	lang := fs.String("lang", "", "set language code")
	identifier := fs.String("identifier", "", "set primary identifier value")
	description := fs.String("description", "", "set description text")

	var creators multiValue
	fs.Var(&creators, "creator", "repeatable creator credit (overrides existing list)")

	metaJSONPath := fs.String("meta-json", "", "apply metadata patch from JSON file")
	dumpMeta := fs.String("dump-meta", "", "write current metadata snapshot (JSON) to file")
	dumpNav := fs.String("dump-nav", "", "write current nav document to file")
	navReplace := fs.String("nav", "", "replace nav document with this file")
	noTouch := fs.Bool("no-touch-modified", false, "do not update dcterms:modified")

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
