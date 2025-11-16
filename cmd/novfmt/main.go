package main

import (
	"bufio"
	"context"
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

	"github.com/kototok903/novel-formatter/internal/epub"
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

Options:
  -o, -out        Output EPUB path (default merged.epub)
  -t, -title      Override merged title
  -lang           Override merged language (default first volume)
  -c, -creator    Repeatable author credit override
  -list           Text file listing volumes; can repeat
  -dir            Directory to scan for EPUB files; can repeat
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
