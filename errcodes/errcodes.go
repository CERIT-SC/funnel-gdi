// Package errcodes gives the errors Funnel returns to API clients a stable
// code and keeps their messages outside the code, in messages.<lang>.yaml.
//
// A client sees "FUNNEL-<AREA>-<NNN>: <message>". The codes and their
// meaning are documented in website/content/docs/error-codes.md. The
// language is selected with Configure (config ErrorMessages).
package errcodes

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync/atomic"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unknownCode is returned instead of a code that is not in the catalog, with
// unknownMessage if the catalog does not define it either.
const (
	unknownCode    = "FUNNEL-GEN-000"
	unknownMessage = "internal error (unknown code %s)"
)

// internalCode is the code of errors without a code whose gRPC code has no
// generic code in genericCodes.
const internalCode = "FUNNEL-GEN-008"

// genericCodes are the codes Ensure gives to errors without a code, by their
// gRPC code.
var genericCodes = map[codes.Code]string{
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
}

var (
	codeFormat = regexp.MustCompile(`^FUNNEL-[A-Z]+-\d{3}$`)
	// codePrefix matches a message that already starts with a code.
	codePrefix = regexp.MustCompile(`^FUNNEL-[A-Z]+-\d{3}: `)
	// languageFormat keeps the language out of paths: "cs", "pt-BR", ...
	languageFormat = regexp.MustCompile(`^[a-z]{2,3}([_-][A-Za-z0-9]{2,8})?$`)
	// verb matches a fmt verb ("%s", "%-10d", "%[1]q", "%%").
	verb = regexp.MustCompile(`%[^a-zA-Z%]*[a-zA-Z%]`)
)

//go:embed messages.*.yaml
var catalogFiles embed.FS

// catalog maps error codes to fmt format strings.
type catalog map[string]string

// english is the catalog of messages.en.yaml. It has every code and is the
// fallback for codes a translation leaves out; the tests check it, so an
// invalid one stops the program at start-up.
var english = mustParseCatalog(mustReadFile("messages.en.yaml"))

// active is the catalog in use: English until Configure selects another.
var active atomic.Pointer[catalog]

func init() {
	active.Store(&english)
}

// Configure selects the language of the messages: language "" or "en" is
// English. The catalog messages.<language>.yaml is read from dir, or from the
// catalogs built into Funnel when dir is empty. Codes the catalog leaves out
// keep their English message. A catalog with a code that is not in English,
// or with other fmt verbs than the English message, is an error.
func Configure(language, dir string) error {
	if language == "" {
		language = "en"
	}
	if !languageFormat.MatchString(language) {
		return fmt.Errorf("error messages: invalid language %q", language)
	}
	if language == "en" && dir == "" {
		active.Store(&english)
		return nil
	}
	name := "messages." + language + ".yaml"
	var data []byte
	var err error
	if dir != "" {
		name = filepath.Join(dir, name)
		data, err = os.ReadFile(name)
	} else {
		data, err = catalogFiles.ReadFile(name)
	}
	if err != nil {
		return fmt.Errorf("error messages: language %q: %w", language, err)
	}
	translation, err := parseCatalog(data)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	cat, err := english.translate(translation)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	active.Store(&cat)
	return nil
}

// Message returns "<code>: <message>" with the arguments formatted into the
// message of code.
func Message(code string, args ...any) string {
	return active.Load().message(code, args...)
}

// Status returns a gRPC status error with the gRPC code c and the message
// of Message. An unknown code yields an Internal error.
func Status(c codes.Code, code string, args ...any) error {
	return active.Load().status(c, code, args...)
}

// Ensure returns err with an error code in its message. A gRPC status error
// whose message starts with a code is returned as it is; any other one keeps
// its gRPC code and gets the generic code of that gRPC code, with its message
// as the detail. An error that is not a gRPC status is an Internal error.
func Ensure(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return Status(codes.Internal, internalCode, err.Error())
	}
	if codePrefix.MatchString(st.Message()) {
		return err
	}
	code, ok := genericCodes[st.Code()]
	if !ok {
		code = internalCode
	}
	p := st.Proto()
	p.Message = Message(code, st.Message())
	return status.ErrorProto(p)
}

func (cat catalog) message(code string, args ...any) string {
	format, ok := cat[code]
	if !ok {
		format, ok = cat[unknownCode]
		if !ok {
			format = unknownMessage
		}
		return unknownCode + ": " + fmt.Sprintf(format, code)
	}
	return code + ": " + fmt.Sprintf(format, args...)
}

func (cat catalog) status(c codes.Code, code string, args ...any) error {
	if _, ok := cat[code]; !ok {
		c = codes.Internal
	}
	return status.Error(c, cat.message(code, args...))
}

// translate returns a copy of cat with the messages of translation, which
// must only have codes of cat, with the same fmt verbs.
func (cat catalog) translate(translation catalog) (catalog, error) {
	out := catalog{}
	for code, msg := range cat {
		out[code] = msg
	}
	for code, msg := range translation {
		orig, ok := cat[code]
		if !ok {
			return nil, fmt.Errorf("%s is not an error code of Funnel", code)
		}
		if want, got := verbs(orig), verbs(msg); !slices.Equal(want, got) {
			return nil, fmt.Errorf("%s: the message has the fmt verbs %v, the English one %v", code, got, want)
		}
		out[code] = msg
	}
	return out, nil
}

// verbs returns the fmt verbs of a format string, without "%%".
func verbs(format string) []string {
	var out []string
	for _, v := range verb.FindAllString(format, -1) {
		if v != "%%" {
			out = append(out, v)
		}
	}
	return out
}

func parseCatalog(data []byte) (catalog, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("error catalog: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("error catalog: no codes")
	}
	cat := catalog{}
	for code, v := range raw {
		msg, ok := v.(string)
		switch {
		case !codeFormat.MatchString(code):
			return nil, fmt.Errorf("error catalog: %q does not match FUNNEL-<AREA>-<NNN>", code)
		case !ok:
			return nil, fmt.Errorf("error catalog: %s: the message must be a string", code)
		case msg == "":
			return nil, fmt.Errorf("error catalog: %s: empty message", code)
		}
		cat[code] = msg
	}
	return cat, nil
}

func mustReadFile(name string) []byte {
	data, err := catalogFiles.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func mustParseCatalog(data []byte) catalog {
	cat, err := parseCatalog(data)
	if err != nil {
		panic(err)
	}
	return cat
}
