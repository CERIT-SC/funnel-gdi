package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testModule = "example.com/mod"

// writeProfile writes a coverage profile into a temporary directory and
// returns its path.
func writeProfile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cover.out")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// runTool runs the tool with the test module and returns its exit code and
// output.
func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"-module", testModule}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestMergeKeepsMaximumCount(t *testing.T) {
	a := writeProfile(t, "mode: set\n"+
		"example.com/mod/pkg/a.go:1.1,2.2 3 0\n"+
		"example.com/mod/pkg/a.go:3.1,4.2 1 0\n")
	// go test -coverpkg writes the same block once per test binary into one
	// file, so b repeats a block too.
	b := writeProfile(t, "mode: set\n"+
		"example.com/mod/pkg/a.go:1.1,2.2 3 1\n"+
		"example.com/mod/pkg/b.go:1.1,2.2 2 1\n"+
		"example.com/mod/pkg/b.go:1.1,2.2 2 0\n")

	p, err := loadProfiles([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	r := p.report(testModule)

	// a.go block 1 covered in b (3 statements), a.go block 2 only in a and
	// uncovered (1), b.go only in b and covered (2).
	if r.Total != 6 || r.Covered != 5 {
		t.Fatalf("got %d/%d covered/total, want 5/6", r.Covered, r.Total)
	}
}

func TestTotalIsWeightedByStatements(t *testing.T) {
	// pkg/x: 1 of 1 statement covered (100%), pkg/y: 0 of 9 (0%). The mean
	// of the package percentages would be 50%, the weighted total is 10%.
	p := writeProfile(t, "mode: set\n"+
		"example.com/mod/x/x.go:1.1,2.2 1 1\n"+
		"example.com/mod/y/y.go:1.1,2.2 9 0\n")

	code, out, _ := runTool(t, p)
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(out, "10.0%") {
		t.Fatalf("total 10.0%% not in output:\n%s", out)
	}
}

func TestExcludedFiles(t *testing.T) {
	for _, tc := range []struct {
		file     string
		excluded bool
	}{
		{"example.com/mod/tes/tes.pb.go", true},
		{"example.com/mod/tes/tes.pb.gw.go", true},
		{"example.com/mod/tes/tes_grpc.pb.go", true},
		{"example.com/mod/webdash/web.go", true},
		{"example.com/mod/config/internal/bundle.go", true},
		{"example.com/mod/examples/internal/bundle.go", true},
		{"example.com/mod/compute/scheduler/mock_Client.go", true},
		{"example.com/mod/tests/funnel_utils.go", true},
		{"example.com/mod/tests/core/helpers.go", true},
		{"example.com/mod/tes/pbgo.go", false},
		{"example.com/mod/tes/tes.go", false},
		{"example.com/mod/tes/bundle.go", false},
		{"example.com/mod/tes/mocks.go", false},
		{"example.com/mod/webdash/handler.go", false},
		{"example.com/mod/storage/tests.go", false},
		{"example.com/mod/server/tests/x.go", false},
	} {
		if got := excluded(tc.file, testModule); got != tc.excluded {
			t.Errorf("excluded(%q) = %v, want %v", tc.file, got, tc.excluded)
		}
	}
}

func TestExcludedFilesDoNotCount(t *testing.T) {
	p := writeProfile(t, "mode: set\n"+
		"example.com/mod/tes/tes.go:1.1,2.2 1 1\n"+
		"example.com/mod/tes/tes.pb.go:1.1,2.2 99 0\n")

	code, out, _ := runTool(t, p)
	if code != 0 || !strings.Contains(out, "100.0%") {
		t.Fatalf("exit code %d, want 100.0%% in:\n%s", code, out)
	}
}

func TestMinimumGate(t *testing.T) {
	// 499 of 1000 statements = 49.9%, 500 of 1000 = 50.0%.
	half := writeProfile(t, "mode: set\n"+
		"example.com/mod/a/a.go:1.1,2.2 500 1\n"+
		"example.com/mod/a/a.go:3.1,4.2 500 0\n")
	below := writeProfile(t, "mode: set\n"+
		"example.com/mod/a/a.go:1.1,2.2 499 1\n"+
		"example.com/mod/a/a.go:3.1,4.2 501 0\n")

	if code, _, errOut := runTool(t, "-min", "50", half); code != 0 {
		t.Fatalf("50.0%% with -min 50: exit code %d, stderr %q", code, errOut)
	}
	code, _, errOut := runTool(t, "-min", "50", below)
	if code != 1 {
		t.Fatalf("49.9%% with -min 50: exit code %d, want 1", code)
	}
	if !strings.Contains(errOut, "49.90%") || !strings.Contains(errOut, "50.00%") {
		t.Fatalf("message should show actual and required coverage, got %q", errOut)
	}
}

func TestInvalidInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.out")
	for name, args := range map[string][]string{
		"missing file":       {missing},
		"empty file":         {writeProfile(t, "")},
		"no mode line":       {writeProfile(t, "example.com/mod/a/a.go:1.1,2.2 1 1\n")},
		"mixed modes":        {writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 1\n"), writeProfile(t, "mode: atomic\nexample.com/mod/a/a.go:1.1,2.2 1 1\n")},
		"malformed line":     {writeProfile(t, "mode: set\nexample.com/mod/a/a.go 1 1\n")},
		"bad count":          {writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 x\n")},
		"no statements":      {writeProfile(t, "mode: set\n")},
		"no profiles":        {},
		"unknown format":     {"-format", "xml", writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 1\n")},
		"negative min":       {"-min", "-5", writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 1\n")},
		"min above 100":      {"-min", "101", writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 1\n")},
		"only generated":     {writeProfile(t, "mode: set\nexample.com/mod/a/a.pb.go:1.1,2.2 1 1\n")},
		"inconsistent block": {writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 1 1\nexample.com/mod/a/a.go:1.1,2.2 2 1\n")},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, errOut := runTool(t, args...)
			if code != 2 {
				t.Fatalf("exit code %d, want 2 (stderr %q)", code, errOut)
			}
			if errOut == "" {
				t.Fatal("no error message")
			}
		})
	}
}

func TestMarkdownReport(t *testing.T) {
	p := writeProfile(t, "mode: set\n"+
		"example.com/mod/small/s.go:1.1,2.2 2 0\n"+
		"example.com/mod/big/b.go:1.1,2.2 10 0\n"+
		"example.com/mod/big/b.go:3.1,4.2 10 1\n"+
		"example.com/mod/root.go:1.1,2.2 1 1\n")

	code, out, _ := runTool(t, "-format", "markdown", "-title", "Unit coverage", p)
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(out, "## Unit coverage") {
		t.Errorf("missing title:\n%s", out)
	}
	if !strings.Contains(out, "**Total: 47.8%** (11 of 23 statements)") {
		t.Errorf("missing total line:\n%s", out)
	}
	big, small, root := strings.Index(out, "| `big` |"), strings.Index(out, "| `small` |"), strings.Index(out, "| `.` |")
	if big < 0 || small < 0 || root < 0 {
		t.Fatalf("missing package rows:\n%s", out)
	}
	if !(big < small && small < root) {
		t.Errorf("rows not sorted by uncovered statements (big 10, small 2, root 0):\n%s", out)
	}
}

func TestMergedProfileOutput(t *testing.T) {
	a := writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 3 0\nexample.com/mod/a/a.pb.go:1.1,2.2 3 0\n")
	b := writeProfile(t, "mode: set\nexample.com/mod/a/a.go:1.1,2.2 3 1\n")
	out := filepath.Join(t.TempDir(), "merged.out")

	if code, _, errOut := runTool(t, "-o", out, a, b); code != 0 {
		t.Fatalf("exit code %d: %s", code, errOut)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "mode: set\nexample.com/mod/a/a.go:1.1,2.2 3 1\n"
	if string(got) != want {
		t.Fatalf("merged profile:\n%s\nwant:\n%s", got, want)
	}
}

func TestModuleFromGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/other\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := moduleFromGoMod(filepath.Join(dir, "go.mod")); got != "example.com/other" {
		t.Fatalf("got %q", got)
	}
	if got := moduleFromGoMod(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing go.mod: got %q, want empty", got)
	}
}
