package main

import (
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

// definitions splits a .proto file into its enums, messages and rpcs. The
// generator iterates over maps, so the order of the definitions varies.
func definitions(proto string) []string {
	var defs []string
	var cur []string
	for _, line := range strings.Split(proto, "\n") {
		switch {
		case strings.HasPrefix(line, "enum "), strings.HasPrefix(line, "message "), strings.HasPrefix(line, "    rpc "):
			cur = []string{line}
		case cur != nil:
			cur = append(cur, line)
			if line == "}" || line == "    }" {
				defs = append(defs, strings.Join(cur, "\n"))
				cur = nil
			}
		}
	}
	sort.Strings(defs)
	return defs
}

// runMain runs the generator on the OpenAPI document and returns its output.
func runMain(t *testing.T, input string) string {
	t.Helper()
	args, stdout := os.Args, os.Stdout
	t.Cleanup(func() { os.Args, os.Stdout = args, stdout })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"openapi2proto", input}
	os.Stdout = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	main()
	w.Close()
	return <-out
}

// The generated definitions are the ones in tes/tes.proto ("make proto").
func TestGenerateTESProto(t *testing.T) {
	spec := "../../tes/task-execution-schemas/openapi/task_execution_service.openapi.yaml"
	if _, err := os.Stat(spec); err != nil {
		t.Skip("the tes/task-execution-schemas submodule is not checked out")
	}
	got := definitions(runMain(t, spec))
	want, err := os.ReadFile("../../tes/tes.proto")
	if err != nil {
		t.Fatal(err)
	}
	wantDefs := definitions(string(want))
	if len(got) < 30 || len(got) != len(wantDefs) {
		t.Fatalf("got %d definitions, tes/tes.proto has %d", len(got), len(wantDefs))
	}
	for i := range got {
		if got[i] != wantDefs[i] {
			t.Errorf("generated:\n%s\ntes/tes.proto:\n%s", got[i], wantDefs[i])
		}
	}
}
