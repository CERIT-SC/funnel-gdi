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
	cat, err := parseCatalog(mustReadFile("messages.en.yaml"))
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
	for code := range english {
		if !documented[code] {
			t.Errorf("%s is in messages.en.yaml but not in website/content/docs/error-codes.md", code)
		}
	}
	for code := range documented {
		if _, ok := english[code]; !ok {
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
			if _, ok := english[code]; !ok {
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

// writeCatalog writes messages.<language>.yaml into a temporary directory.
func writeCatalog(t *testing.T, language, data string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "messages."+language+".yaml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := Configure("", ""); err != nil {
			t.Error(err)
		}
	})
	return dir
}

func TestConfigureTranslation(t *testing.T) {
	dir := writeCatalog(t, "cs", "FUNNEL-TASK-001: \"úloha nenalezena: taskID: %s\"\n")
	if err := Configure("cs", dir); err != nil {
		t.Fatal(err)
	}
	if got := Message("FUNNEL-TASK-001", "abc"); got != "FUNNEL-TASK-001: úloha nenalezena: taskID: abc" {
		t.Errorf("translated message %q", got)
	}
	// Codes the translation leaves out keep the English message.
	if got := Message("FUNNEL-AUTH-003"); got != "FUNNEL-AUTH-003: "+english["FUNNEL-AUTH-003"] {
		t.Errorf("fallback message %q", got)
	}
	st, _ := status.FromError(Status(codes.NotFound, "FUNNEL-TASK-001", "x"))
	if st.Code() != codes.NotFound || st.Message() != "FUNNEL-TASK-001: úloha nenalezena: taskID: x" {
		t.Errorf("status %v %q", st.Code(), st.Message())
	}

	// Back to English.
	if err := Configure("en", ""); err != nil {
		t.Fatal(err)
	}
	if got := Message("FUNNEL-TASK-001", "abc"); got != "FUNNEL-TASK-001: task not found: taskID: abc" {
		t.Errorf("English message %q", got)
	}
}

func TestConfigureEnglishFromDir(t *testing.T) {
	dir := writeCatalog(t, "en", "FUNNEL-AUTH-003: \"Wrong user name or password\"\n")
	if err := Configure("", dir); err != nil {
		t.Fatal(err)
	}
	if got := Message("FUNNEL-AUTH-003"); got != "FUNNEL-AUTH-003: Wrong user name or password" {
		t.Errorf("message %q", got)
	}
}

func TestConfigureRejects(t *testing.T) {
	for name, tc := range map[string]struct{ language, data string }{
		"unknown code":         {"cs", "FUNNEL-NOPE-001: \"x\"\n"},
		"missing verb":         {"cs", "FUNNEL-TASK-001: \"úloha nenalezena\"\n"},
		"other verb":           {"cs", "FUNNEL-TASK-001: \"úloha nenalezena: %d\"\n"},
		"extra verb":           {"cs", "FUNNEL-AUTH-003: \"chyba %s\"\n"},
		"invalid catalog":      {"cs", "FUNNEL-TASK-001: [a]\n"},
		"no file for language": {"de", ""},
		"path in language":     {"../cs", ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.data != "" {
				dir = writeCatalog(t, tc.language, tc.data)
			}
			if err := Configure(tc.language, dir); err == nil {
				t.Fatal("expected an error")
			}
			// The active catalog is unchanged.
			if got := Message("FUNNEL-TASK-001", "a"); got != "FUNNEL-TASK-001: task not found: taskID: a" {
				t.Errorf("message after a failed Configure: %q", got)
			}
		})
	}
	// Only English is built in.
	if err := Configure("cs", ""); err == nil {
		t.Error("expected an error for a language that is not built in")
	}
}

func TestVerbs(t *testing.T) {
	got := verbs("%s and %-5d, 100%% %[1]q %v")
	want := []string{"%s", "%-5d", "%[1]q", "%v"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("verbs %q, want %q", got, want)
	}
}

func TestEnsure(t *testing.T) {
	if Ensure(nil) != nil {
		t.Error("nil error")
	}
	coded := Status(codes.NotFound, "FUNNEL-TASK-001", "x")
	if Ensure(coded) != coded {
		t.Error("an error with a code must be returned as it is")
	}
	for c, code := range map[codes.Code]string{
		codes.InvalidArgument:    "FUNNEL-GEN-001",
		codes.FailedPrecondition: "FUNNEL-GEN-001",
		codes.OutOfRange:         "FUNNEL-GEN-001",
		codes.NotFound:           "FUNNEL-GEN-002",
		codes.Unauthenticated:    "FUNNEL-GEN-003",
		codes.PermissionDenied:   "FUNNEL-GEN-004",
		codes.AlreadyExists:      "FUNNEL-GEN-005",
		codes.Aborted:            "FUNNEL-GEN-005",
		codes.Canceled:           "FUNNEL-GEN-006",
		codes.DeadlineExceeded:   "FUNNEL-GEN-006",
		codes.Unimplemented:      "FUNNEL-GEN-007",
		codes.Internal:           "FUNNEL-GEN-008",
		codes.Unknown:            "FUNNEL-GEN-008",
		codes.Unavailable:        "FUNNEL-GEN-008",
	} {
		st, _ := status.FromError(Ensure(status.Error(c, "Not Found")))
		if st.Code() != c {
			t.Errorf("%v: gRPC code changed to %v", c, st.Code())
		}
		if want := Message(code, "Not Found"); st.Message() != want {
			t.Errorf("%v: message %q, want %q", c, st.Message(), want)
		}
	}
	st, _ := status.FromError(Ensure(os.ErrNotExist))
	if st.Code() != codes.Internal || st.Message() != "FUNNEL-GEN-008: internal error: file does not exist" {
		t.Errorf("plain error: %v %q", st.Code(), st.Message())
	}
	// A message that only mentions a code later on is not taken as coded.
	st, _ = status.FromError(Ensure(status.Error(codes.InvalidArgument, "bad FUNNEL-TASK-001: x")))
	if !strings.HasPrefix(st.Message(), "FUNNEL-GEN-001: ") {
		t.Errorf("message %q", st.Message())
	}
}
