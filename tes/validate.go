package tes

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/ohsu-comp-bio/funnel/errcodes"
)

// isForbiddenPath reports whether path is, or is nested under, any of the
// given forbidden path prefixes. The comparison is exact-segment based so that
// "/devices" is not treated as being under "/dev".
func isForbiddenPath(candidate string, prefixes []string) bool {
	if candidate == "" {
		return false
	}

	candidate = path.Clean(candidate)
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}

		prefix = path.Clean(prefix)
		if !path.IsAbs(prefix) {
			continue
		}

		if prefix == "/" || candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
			return true
		}
	}
	return false
}

// ValidationError contains task validation errors.
type ValidationError []error

// add records the validation error with the given code (errcodes) and the
// arguments of its message.
func (v *ValidationError) add(code string, a ...interface{}) {
	*v = append(*v, errors.New(errcodes.Message(code, a...)))
}
func (v ValidationError) Error() string {
	s := ""
	for _, e := range v {
		s += fmt.Sprintln(e.Error())
	}
	return s
}

// Validate validates the given task and returns ValidationError,
// or nil if the task is valid.
func Validate(t *Task) ValidationError {
	return ValidateWithForbiddenPathPrefixes(t, nil)
}

// ValidateWithForbiddenPathPrefixes validates the given task and rejects
// inputs, outputs, volumes, and working directories at or beneath any of the
// configured container path prefixes.
func ValidateWithForbiddenPathPrefixes(t *Task, forbiddenPathPrefixes []string) ValidationError {
	var errs ValidationError

	if len(t.Executors) == 0 {
		errs.add("FUNNEL-VAL-001")
	}

	for i, exec := range t.Executors {
		if exec.Image == "" {
			errs.add("FUNNEL-VAL-002", i)
		}

		if len(exec.Command) == 0 {
			errs.add("FUNNEL-VAL-003", i)
		}

		if exec.Workdir != "" && !strings.HasPrefix(exec.Workdir, "/") {
			errs.add("FUNNEL-VAL-004", i)
		}

		if isForbiddenPath(exec.Workdir, forbiddenPathPrefixes) {
			errs.add("FUNNEL-VAL-005", i, exec.Workdir)
		}

		if exec.Stdin != "" && !strings.HasPrefix(exec.Stdin, "/") {
			errs.add("FUNNEL-VAL-006", i)
		}

		if exec.Stdout != "" && !strings.HasPrefix(exec.Stdout, "/") {
			errs.add("FUNNEL-VAL-007", i)
		}

		if exec.Stderr != "" && !strings.HasPrefix(exec.Stderr, "/") {
			errs.add("FUNNEL-VAL-008", i)
		}
	}

	for i, input := range t.Inputs {
		if input.Content != "" && input.Url != "" {
			errs.add("FUNNEL-VAL-009", i)
		} else if input.Url == "" && input.Content == "" {
			errs.add("FUNNEL-VAL-010", i)
		}

		if input.Path == "" {
			errs.add("FUNNEL-VAL-011", i)
		}

		if input.Path != "" && !strings.HasPrefix(input.Path, "/") {
			errs.add("FUNNEL-VAL-012", i)
		}

		if isForbiddenPath(input.Path, forbiddenPathPrefixes) {
			errs.add("FUNNEL-VAL-013", i, input.Path)
		}
	}

	for i, output := range t.Outputs {
		if output.Url == "" {
			errs.add("FUNNEL-VAL-014", i)
		}

		if output.Path == "" {
			errs.add("FUNNEL-VAL-015", i)
		}

		if output.Path != "" && !strings.HasPrefix(output.Path, "/") {
			errs.add("FUNNEL-VAL-016", i)
		}

		if isForbiddenPath(output.Path, forbiddenPathPrefixes) {
			errs.add("FUNNEL-VAL-017", i, output.Path)
		}
	}

	for i, vol := range t.Volumes {
		if !strings.HasPrefix(vol, "/") {
			errs.add("FUNNEL-VAL-018", i)
		}

		if isForbiddenPath(vol, forbiddenPathPrefixes) {
			errs.add("FUNNEL-VAL-019", i, vol)
		}
	}

	for k, v := range t.Tags {
		if k == "" {
			errs.add("FUNNEL-VAL-020", v)
		}
	}

	return errs
}
