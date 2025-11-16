package epub

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type RewriteScope int

const (
	RewriteScopeBody RewriteScope = iota
	RewriteScopeMeta
	RewriteScopeAll
)

type RewriteRule struct {
	Find          string   `json:"find"`
	Replace       string   `json:"replace"`
	Regex         bool     `json:"regex,omitempty"`
	CaseSensitive bool     `json:"case_sensitive,omitempty"`
	Selectors     []string `json:"selectors,omitempty"`
}

type RewriteOptions struct {
	OutPath string
	Scope   RewriteScope
	Rules   []RewriteRule
	DryRun  bool
}

type RewriteStats struct {
	FilesChanged int
	MatchCount   int
}

type compiledSelector struct {
	Tag   string
	Class string
}

type compiledRule struct {
	raw       RewriteRule
	re        *regexp.Regexp
	selectors []compiledSelector
}

type ruleState struct {
	depthStack []bool
	active     int
}

func RewriteEPUB(ctx context.Context, input string, opts RewriteOptions) (RewriteStats, error) {
	var stats RewriteStats
	if input == "" {
		return stats, fmt.Errorf("input EPUB path is required")
	}
	if len(opts.Rules) == 0 {
		return stats, fmt.Errorf("no rewrite rules provided")
	}

	compiled, err := compileRules(opts.Rules)
	if err != nil {
		return stats, err
	}

	vol, err := loadVolume(ctx, 0, input)
	if err != nil {
		return stats, err
	}
	defer os.RemoveAll(vol.TempDir)

	pkg := vol.PackageDoc

	// Rewrite metadata if requested.
	if opts.Scope == RewriteScopeMeta || opts.Scope == RewriteScopeAll {
		metaRules := metadataApplicableRules(compiled)
		matches, changed := rewriteMetadata(&pkg.Metadata, metaRules, !opts.DryRun)
		stats.MatchCount += matches
		if changed {
			stats.FilesChanged++
		}
	}

	// Rewrite XHTML content if requested.
	if opts.Scope == RewriteScopeBody || opts.Scope == RewriteScopeAll {
		for _, item := range pkg.Manifest.Items {
			if item.MediaType != "application/xhtml+xml" {
				continue
			}
			src := filepath.Join(filepath.Dir(vol.PackagePath), filepath.FromSlash(item.Href))
			fileMatches, changed, rewritten, err := rewriteXHTMLFile(src, compiled)
			if err != nil {
				return stats, err
			}
			stats.MatchCount += fileMatches
			if changed {
				stats.FilesChanged++
				if !opts.DryRun {
					if err := os.WriteFile(src, rewritten, 0o644); err != nil {
						return stats, err
					}
				}
			}
		}
	}

	if opts.DryRun {
		return stats, nil
	}

	if stats.FilesChanged == 0 {
		return stats, nil
	}

	if err := writePackage(pkg, vol.PackagePath); err != nil {
		return stats, err
	}

	outPath := opts.OutPath
	if outPath == "" {
		outPath = input
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(outPath), "novfmt-rewrite-*.epub")
	if err != nil {
		return stats, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if err := writeZip(vol.RootDir, tmpPath); err != nil {
		return stats, err
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return stats, err
	}
	tmpPath = ""

	return stats, nil
}

func compileRules(rules []RewriteRule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if r.Find == "" {
			return nil, fmt.Errorf("rule missing find pattern")
		}
		cr := compiledRule{raw: r}

		if r.Regex {
			pat := r.Find
			if !r.CaseSensitive && !strings.HasPrefix(pat, "(?i)") {
				pat = "(?i)" + pat
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("compile regex %q: %w", pat, err)
			}
			cr.re = re
		}

		for _, sel := range r.Selectors {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			for _, part := range strings.Split(sel, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				outSel := compiledSelector{}
				token := part
				if strings.Contains(token, ".") {
					parts := strings.SplitN(token, ".", 2)
					outSel.Tag = strings.ToLower(strings.TrimSpace(parts[0]))
					outSel.Class = strings.TrimSpace(parts[1])
				} else {
					outSel.Tag = strings.ToLower(token)
				}
				cr.selectors = append(cr.selectors, outSel)
			}
		}

		out = append(out, cr)
	}
	return out, nil
}

func metadataApplicableRules(rules []compiledRule) []compiledRule {
	out := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if len(r.selectors) == 0 {
			out = append(out, r)
		}
	}
	return out
}

func rewriteMetadata(meta *Metadata, rules []compiledRule, mutate bool) (int, bool) {
	var matches int
	changed := false

	apply := func(nodes []DCMeta) ([]DCMeta, bool) {
		localChanged := false
		for i := range nodes {
			orig := nodes[i].Value
			val, mc := applyRulesToText(orig, rules)
			if mc > 0 {
				if mutate {
					nodes[i].Value = val
				}
				matches += mc
				localChanged = true
			}
		}
		return nodes, localChanged
	}

	if len(meta.Titles) > 0 {
		var c bool
		meta.Titles, c = apply(meta.Titles)
		changed = changed || c
	}
	if len(meta.Languages) > 0 {
		var c bool
		meta.Languages, c = apply(meta.Languages)
		changed = changed || c
	}
	if len(meta.Identifiers) > 0 {
		var c bool
		meta.Identifiers, c = apply(meta.Identifiers)
		changed = changed || c
	}
	if len(meta.Descriptions) > 0 {
		var c bool
		meta.Descriptions, c = apply(meta.Descriptions)
		changed = changed || c
	}
	if len(meta.Creators) > 0 {
		var c bool
		meta.Creators, c = apply(meta.Creators)
		changed = changed || c
	}

	return matches, changed
}

func rewriteXHTMLFile(path string, rules []compiledRule) (int, bool, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, nil, err
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	var out bytes.Buffer
	enc := xml.NewEncoder(&out)

	type frame struct {
		name xml.Name
	}
	var stack []frame

	states := make([]ruleState, len(rules))

	var totalMatches int
	changed := false

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, false, nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, frame{name: t.Name})
			for i := range rules {
				match := selectorMatches(rules[i], t)
				st := &states[i]
				st.depthStack = append(st.depthStack, match)
				if match {
					st.active++
				}
			}
			if err := enc.EncodeToken(t); err != nil {
				return 0, false, nil, err
			}

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			for i := range rules {
				st := &states[i]
				if len(st.depthStack) == 0 {
					continue
				}
				last := st.depthStack[len(st.depthStack)-1]
				st.depthStack = st.depthStack[:len(st.depthStack)-1]
				if last && st.active > 0 {
					st.active--
				}
			}
			if err := enc.EncodeToken(t); err != nil {
				return 0, false, nil, err
			}

		case xml.CharData:
			text := string(t)
			orig := text
			for i := range rules {
				if selectorInactive(rules[i], &states[i]) {
					continue
				}
				updated, mc := applyRuleToText(text, rules[i])
				if mc > 0 {
					text = updated
					totalMatches += mc
				}
			}
			if text != orig {
				changed = true
			}
			if err := enc.EncodeToken(xml.CharData([]byte(text))); err != nil {
				return 0, false, nil, err
			}

		default:
			if err := enc.EncodeToken(t); err != nil {
				return 0, false, nil, err
			}
		}
	}

	if err := enc.Flush(); err != nil {
		return 0, false, nil, err
	}

	if !changed {
		return totalMatches, false, nil, nil
	}

	return totalMatches, true, out.Bytes(), nil
}

func selectorMatches(rule compiledRule, el xml.StartElement) bool {
	if len(rule.selectors) == 0 {
		// No selector: apply everywhere in body scope.
		return true
	}
	tag := strings.ToLower(el.Name.Local)
	var classAttr string
	for _, a := range el.Attr {
		if a.Name.Local == "class" {
			classAttr = a.Value
			break
		}
	}
	classes := map[string]struct{}{}
	for _, token := range strings.Fields(classAttr) {
		classes[token] = struct{}{}
	}
	for _, sel := range rule.selectors {
		if sel.Tag != "" && sel.Tag != tag {
			continue
		}
		if sel.Class != "" {
			if _, ok := classes[sel.Class]; !ok {
				continue
			}
		}
		return true
	}
	return false
}

func selectorInactive(rule compiledRule, st *ruleState) bool {
	if len(rule.selectors) == 0 {
		// Global rule, always active.
		return false
	}
	return st.active == 0
}

func applyRulesToText(s string, rules []compiledRule) (string, int) {
	total := 0
	for i := range rules {
		var mc int
		s, mc = applyRuleToText(s, rules[i])
		total += mc
	}
	return s, total
}

func applyRuleToText(s string, rule compiledRule) (string, int) {
	if s == "" {
		return s, 0
	}
	if rule.re != nil {
		matches := len(rule.re.FindAllStringIndex(s, -1))
		if matches == 0 {
			return s, 0
		}
		out := rule.re.ReplaceAllString(s, rule.raw.Replace)
		return out, matches
	}
	if rule.raw.CaseSensitive {
		count := strings.Count(s, rule.raw.Find)
		if count == 0 {
			return s, 0
		}
		return strings.ReplaceAll(s, rule.raw.Find, rule.raw.Replace), count
	}
	// Case-insensitive plain text.
	findLower := strings.ToLower(rule.raw.Find)
	if findLower == "" {
		return s, 0
	}
	var buf strings.Builder
	buf.Grow(len(s))
	lower := strings.ToLower(s)
	i := 0
	matches := 0
	for {
		j := strings.Index(lower[i:], findLower)
		if j < 0 {
			buf.WriteString(s[i:])
			break
		}
		j += i
		buf.WriteString(s[i:j])
		buf.WriteString(rule.raw.Replace)
		i = j + len(rule.raw.Find)
		matches++
	}
	if matches == 0 {
		return s, 0
	}
	return buf.String(), matches
}

func LoadRewriteRulesJSON(path string) ([]RewriteRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []RewriteRule
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}
