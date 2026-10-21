// Package errcodes gives the errors Funnel returns to API clients a stable
// code and keeps their messages outside the code, in messages.<lang>.yaml.
//
// A client sees "FUNNEL-<AREA>-<NNN>: <message>". The codes and their
// meaning are documented in website/content/docs/error-codes.md.
package errcodes

import (
	_ "embed"
	"fmt"
	"regexp"

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

var codeFormat = regexp.MustCompile(`^FUNNEL-[A-Z]+-\d{3}$`)

//go:embed messages.en.yaml
var messagesEN []byte

// catalog maps error codes to fmt format strings.
type catalog map[string]string

// defaultCatalog is the English catalog. Only English ships; the catalog is
// checked by the tests, so an invalid one stops the program at start-up.
var defaultCatalog = mustParseCatalog(messagesEN)

// Message returns "<code>: <message>" with the arguments formatted into the
// message of code.
func Message(code string, args ...any) string {
	return defaultCatalog.message(code, args...)
}

// Status returns a gRPC status error with the gRPC code c and the message
// of Message. An unknown code yields an Internal error.
func Status(c codes.Code, code string, args ...any) error {
	return defaultCatalog.status(c, code, args...)
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

func mustParseCatalog(data []byte) catalog {
	cat, err := parseCatalog(data)
	if err != nil {
		panic(err)
	}
	return cat
}
