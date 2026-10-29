package worker

import (
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
)

func TestDockerMemoryMB(t *testing.T) {
	for _, tc := range []struct {
		res  *tes.Resources
		want int64
	}{
		{nil, 0},
		{&tes.Resources{}, 0},
		{&tes.Resources{RamGb: -1}, 0},
		{&tes.Resources{RamGb: 2}, 2048},
		{&tes.Resources{RamGb: 0.5}, 512},
		{&tes.Resources{RamGb: 1.0001}, 1024},
	} {
		if got := (DockerCommand{Resources: tc.res}).MemoryMB(); got != tc.want {
			t.Errorf("%v: %d, want %d", tc.res, got, tc.want)
		}
	}
}

func TestShellEscape(t *testing.T) {
	for in, want := range map[string]string{
		"":           "''",
		"plain":      "'plain'",
		"with space": "'with space'",
		"it's":       `'it'"'"'s'`,
		"$HOME;rm":   "'$HOME;rm'",
	} {
		if got := shellEscape(in); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
