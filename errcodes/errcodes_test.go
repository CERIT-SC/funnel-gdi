package errcodes

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusKnownCode(t *testing.T) {
	cat := catalog{"FUNNEL-TEST-001": "task %s not found"}
	err := cat.status(codes.NotFound, "FUNNEL-TEST-001", "abc")
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("code %v, want NotFound", st.Code())
	}
	if st.Message() != "FUNNEL-TEST-001: task abc not found" {
		t.Errorf("message %q", st.Message())
	}
}

func TestUnknownCode(t *testing.T) {
	cat := catalog{"FUNNEL-TEST-001": "x"}
	err := cat.status(codes.NotFound, "FUNNEL-NOPE-999")
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("code %v, want Internal", st.Code())
	}
	if st.Message() != "FUNNEL-GEN-000: internal error (unknown code FUNNEL-NOPE-999)" {
		t.Errorf("message %q", st.Message())
	}
	if got := cat.message("FUNNEL-NOPE-999"); !strings.HasPrefix(got, "FUNNEL-GEN-000: ") {
		t.Errorf("message %q", got)
	}
}

func TestWrongNumberOfArguments(t *testing.T) {
	cat := catalog{"FUNNEL-TEST-001": "%s and %s"}
	// Must not panic; fmt marks the missing and extra arguments.
	if got := cat.message("FUNNEL-TEST-001", "one"); got != "FUNNEL-TEST-001: one and %!s(MISSING)" {
		t.Errorf("missing argument: %q", got)
	}
	if got := cat.message("FUNNEL-TEST-001", "a", "b", "c"); !strings.Contains(got, "%!(EXTRA") {
		t.Errorf("extra argument: %q", got)
	}
}

func TestParseCatalogRejectsInvalid(t *testing.T) {
	for name, data := range map[string]string{
		"duplicate code": "FUNNEL-TEST-001: a\nFUNNEL-TEST-001: b\n",
		"empty message":  "FUNNEL-TEST-001: \"\"\n",
		"bad code":       "FUNNEL-test-1: a\n",
		"no prefix":      "AUTH-001: a\n",
		"not a string":   "FUNNEL-TEST-001: [a, b]\n",
		"not a mapping":  "- FUNNEL-TEST-001\n",
		"empty catalog":  "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCatalog([]byte(data)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEmbeddedCatalog(t *testing.T) {
	cat, err := parseCatalog(messagesEN)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat[unknownCode]; !ok {
		t.Errorf("the catalog must define %s", unknownCode)
	}
}

var codePattern = regexp.MustCompile(`FUNNEL-[A-Z]+-\d{3}`)

// TestReferencePageListsEveryCode keeps the error code reference page in
// sync with the catalog.
func TestReferencePageListsEveryCode(t *testing.T) {
	page, err := os.ReadFile("../website/content/docs/error-codes.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, code := range codePattern.FindAllString(string(page), -1) {
		documented[code] = true
	}
	for code := range defaultCatalog {
		if !documented[code] {
			t.Errorf("%s is in messages.en.yaml but not in website/content/docs/error-codes.md", code)
		}
	}
	for code := range documented {
		if _, ok := defaultCatalog[code]; !ok {
			t.Errorf("%s is in website/content/docs/error-codes.md but not in messages.en.yaml", code)
		}
	}
}

// TestCodesUsedInCodeExist checks that every error code written in the Go
// code of the repository is defined in the catalog.
func TestCodesUsedInCodeExist(t *testing.T) {
	var unknown []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "website") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, code := range codePattern.FindAllString(string(data), -1) {
			if _, ok := defaultCatalog[code]; !ok {
				unknown = append(unknown, path+": "+code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(unknown)
	for _, u := range unknown {
		t.Errorf("code not in messages.en.yaml: %s", u)
	}
}
