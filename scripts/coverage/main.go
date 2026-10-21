// Command coverage merges Go coverage profiles, leaves out generated code and
// test helpers, reports statement coverage per package and optionally fails
// below a minimum.
//
// Usage:
//
//	go run ./scripts/coverage [-min PERCENT] [-format text|markdown] [-title TITLE] [-o MERGED] PROFILE...
//
// A block that appears in several profiles (or several times in one, as
// written by "go test -coverpkg") counts as covered if any of them covered it.
//
// Exit codes: 0 ok, 1 coverage below -min, 2 invalid input.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// generated matches files that are generated, not written by hand: Protobuf
// and gRPC gateway code, go-bindata bundles and mockery mocks. Keep in sync
// with INV-1 in AGENTS.md.
var generated = regexp.MustCompile(`(\.pb\.go|\.pb\.gw\.go|^webdash/web\.go|/internal/bundle\.go|/mock_[^/]+\.go)$`)

// blockLine matches "file:startLine.startCol,endLine.endCol numStmts count".
var blockLine = regexp.MustCompile(`^(.+):(\d+\.\d+,\d+\.\d+) (\d+) (\d+)$`)

type block struct {
	stmts int
	count int
}

type profile struct {
	mode   string
	blocks map[string]block // key: "file:position"
}

type pkgStats struct {
	Name    string
	Total   int
	Covered int
}

type report struct {
	Total    int
	Covered  int
	Packages []pkgStats
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	minimum := fs.Float64("min", 0, "fail when the total coverage (percent) is below this value")
	format := fs.String("format", "text", "report format: text or markdown")
	title := fs.String("title", "Coverage", "report title (markdown)")
	merged := fs.String("o", "", "write the merged, filtered profile to this file")
	module := fs.String("module", moduleFromGoMod("go.mod"), "module path (default: from ./go.mod)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fail := func(err error) int {
		fmt.Fprintln(stderr, "coverage:", err)
		return 2
	}
	switch {
	case fs.NArg() == 0:
		return fail(errors.New("no coverage profiles given"))
	case *format != "text" && *format != "markdown":
		return fail(fmt.Errorf("unknown format %q (use text or markdown)", *format))
	case *minimum < 0 || *minimum > 100:
		return fail(fmt.Errorf("-min %v is not a percentage between 0 and 100", *minimum))
	}

	p, err := loadProfiles(fs.Args())
	if err != nil {
		return fail(err)
	}
	p.filter(*module)
	r := p.report(*module)
	if r.Total == 0 {
		return fail(errors.New("no statements left after leaving out generated code and test helpers"))
	}

	if *merged != "" {
		if err := p.write(*merged); err != nil {
			return fail(err)
		}
	}
	if *format == "markdown" {
		r.markdown(stdout, *title)
	} else {
		r.text(stdout)
	}

	if float64(r.Covered)*100 < *minimum*float64(r.Total) {
		fmt.Fprintf(stderr, "coverage: total %.2f%% is below the required %.2f%%\n", r.percent(), *minimum)
		return 1
	}
	return 0
}

// moduleFromGoMod returns the module path declared in a go.mod file, or ""
// if it cannot be read.
func moduleFromGoMod(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`)
		}
	}
	return ""
}

// excluded reports whether a file (as named in a profile) is left out of the
// coverage: generated code and the test helpers under <module>/tests/.
func excluded(file, module string) bool {
	rel := strings.TrimPrefix(file, module+"/")
	return generated.MatchString(rel) || strings.HasPrefix(rel, "tests/")
}

func loadProfiles(files []string) (*profile, error) {
	p := &profile{blocks: map[string]block{}}
	for _, file := range files {
		if err := p.load(file); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *profile) load(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return fmt.Errorf("%s: %v", file, err)
		}
		return fmt.Errorf("%s: empty profile", file)
	}
	mode, ok := strings.CutPrefix(sc.Text(), "mode: ")
	if !ok {
		return fmt.Errorf("%s:1: missing \"mode:\" line", file)
	}
	if p.mode != "" && p.mode != mode {
		return fmt.Errorf("%s: mode %q does not match mode %q of the other profiles", file, mode, p.mode)
	}
	p.mode = mode

	for n := 2; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" {
			continue
		}
		m := blockLine.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("%s:%d: malformed line %q", file, n, line)
		}
		stmts, err1 := strconv.Atoi(m[3])
		count, err2 := strconv.Atoi(m[4])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("%s:%d: malformed line %q", file, n, line)
		}
		key := m[1] + ":" + m[2]
		if old, ok := p.blocks[key]; ok {
			if old.stmts != stmts {
				return fmt.Errorf("%s:%d: block %s has %d statements, but %d in an earlier profile (profiles from different sources?)", file, n, key, stmts, old.stmts)
			}
			count = max(count, old.count)
		}
		p.blocks[key] = block{stmts: stmts, count: count}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %v", file, err)
	}
	return nil
}

func (p *profile) filter(module string) {
	for key := range p.blocks {
		if excluded(fileOf(key), module) {
			delete(p.blocks, key)
		}
	}
}

func (p *profile) report(module string) report {
	pkgs := map[string]*pkgStats{}
	var r report
	for key, b := range p.blocks {
		file := fileOf(key)
		if excluded(file, module) {
			continue
		}
		name := path.Dir(strings.TrimPrefix(file, module+"/"))
		s := pkgs[name]
		if s == nil {
			s = &pkgStats{Name: name}
			pkgs[name] = s
		}
		s.Total += b.stmts
		r.Total += b.stmts
		if b.count > 0 {
			s.Covered += b.stmts
			r.Covered += b.stmts
		}
	}
	for _, s := range pkgs {
		r.Packages = append(r.Packages, *s)
	}
	sort.Slice(r.Packages, func(i, j int) bool {
		a, b := r.Packages[i], r.Packages[j]
		if ua, ub := a.Total-a.Covered, b.Total-b.Covered; ua != ub {
			return ua > ub
		}
		return a.Name < b.Name
	})
	return r
}

func (p *profile) write(file string) error {
	keys := make([]string, 0, len(p.blocks))
	for key := range p.blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	fmt.Fprintf(&sb, "mode: %s\n", p.mode)
	for _, key := range keys {
		b := p.blocks[key]
		fmt.Fprintf(&sb, "%s %d %d\n", key, b.stmts, b.count)
	}
	return os.WriteFile(file, []byte(sb.String()), 0o644)
}

// fileOf returns the file name of a block key ("file:start,end").
func fileOf(key string) string {
	return key[:strings.LastIndex(key, ":")]
}

func percent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) * 100 / float64(total)
}

func (r report) percent() float64 { return percent(r.Covered, r.Total) }

func (r report) text(w io.Writer) {
	for _, s := range r.Packages {
		fmt.Fprintf(w, "%-50s %6.1f%%  (%d/%d)\n", s.Name, percent(s.Covered, s.Total), s.Covered, s.Total)
	}
	fmt.Fprintf(w, "total: %.1f%% (%d of %d statements)\n", r.percent(), r.Covered, r.Total)
}

func (r report) markdown(w io.Writer, title string) {
	fmt.Fprintf(w, "## %s\n\n", title)
	fmt.Fprintf(w, "**Total: %.1f%%** (%d of %d statements)\n\n", r.percent(), r.Covered, r.Total)
	fmt.Fprintln(w, "Generated code and the test helpers in `tests/` are not counted. Packages sorted by uncovered statements.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Package | Coverage | Covered | Statements | Uncovered |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|")
	for _, s := range r.Packages {
		fmt.Fprintf(w, "| `%s` | %.1f%% | %d | %d | %d |\n", s.Name, percent(s.Covered, s.Total), s.Covered, s.Total, s.Total-s.Covered)
	}
}
