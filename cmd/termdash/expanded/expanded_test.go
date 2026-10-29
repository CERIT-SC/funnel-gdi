package expanded

import (
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
)

func TestTaskInfo(t *testing.T) {
	info := NewTaskInfo(&tes.Task{Id: "t1", State: tes.Running, Name: "name", Description: "desc", Tags: map[string]string{"k": "v"}})
	want := [][]string{{"id", "t1"}, {"state", "RUNNING"}, {"name", "name"}, {"description", "desc"}, {"tags", "k: v"}}
	if len(info.Rows) != len(want) || info.Height != len(want)+2 {
		t.Fatalf("rows %v height %d", info.Rows, info.Height)
	}
	for i, row := range want {
		if info.Rows[i][0] != row[0] || info.Rows[i][1] != row[1] {
			t.Errorf("row %d: %v, want %v", i, info.Rows[i], row)
		}
	}
}

func TestTaskParameters(t *testing.T) {
	in := NewTaskInputs(nil, "INPUTS")
	in.Set("path", "/in")
	in.Set("url", "s3://b/k")
	// Rows follow the fixed field order, not the order they were set in.
	if in.BorderLabel != "INPUTS" || len(in.Rows) != 2 || in.Rows[0][0] != "url" || in.Rows[1][1] != "/in" || in.Height != 4 {
		t.Errorf("inputs %v", in.Rows)
	}
	out := NewTaskOutputs(nil, "OUTPUTS")
	out.Set("type", "FILE")
	if out.BorderLabel != "OUTPUTS" || len(out.Rows) != 1 || out.Rows[0][1] != "FILE" {
		t.Errorf("outputs %v", out.Rows)
	}
	if sizeError.Text != "screen too small!" {
		t.Errorf("size error %q", sizeError.Text)
	}
}
