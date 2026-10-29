package tes

import (
	"reflect"
	"testing"
)

func TestBase64Encode(t *testing.T) {
	task := &Task{
		Id: "task1",
		Executors: []*Executor{
			{
				Image:   "alpine",
				Command: []string{"echo", "hello world"},
			},
		},
	}

	expected := "ewogICJleGVjdXRvcnMiOiAgWwogICAgewogICAgICAiY29tbWFuZCI6ICBbCiAgICAgICAgImVjaG8iLAogICAgICAgICJoZWxsbyB3b3JsZCIKICAgICAgXSwKICAgICAgImltYWdlIjogICJhbHBpbmUiCiAgICB9CiAgXSwKICAiaWQiOiAgInRhc2sxIgp9"

	// TODO: Odd (and as of yet unexpained!) encoding/decoding issue with two spaces after field (e.g. `"id":  "task1"`)?
	expected_alt := "ewogICJleGVjdXRvcnMiOiBbCiAgICB7CiAgICAgICJjb21tYW5kIjogWwogICAgICAgICJlY2hvIiwKICAgICAgICAiaGVsbG8gd29ybGQiCiAgICAgIF0sCiAgICAgICJpbWFnZSI6ICJhbHBpbmUiCiAgICB9CiAgXSwKICAiaWQiOiAidGFzazEiCn0="

	encoded, err := Base64Encode(task)
	if err != nil {
		t.Fatal(err)
	}

	if encoded != expected && encoded != expected_alt {
		t.Logf("expected: %+v", expected)
		t.Logf("expected_alt: %+v", expected_alt)
		t.Logf("actual: %+v", encoded)
		t.Fatal("unexpected value returned from Base64Encode")
	}

	decoded, err := Base64Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(decoded, task) {
		t.Logf("expected: %+v", task)
		t.Logf("actual: %+v", decoded)
		t.Fatal("incorrect decoded task from Base64Decode")
	}
}

func TestGetView(t *testing.T) {
	task := &Task{
		Id:     "task1",
		Name:   "name",
		State:  State_RUNNING,
		Inputs: []*Input{{Path: "/data/in", Content: "secret"}},
	}
	// The name, and the input content, are in the view.
	for view, want := range map[string][2]bool{
		"":        {false, false},
		"MINIMAL": {false, false},
		"BASIC":   {true, false},
		"FULL":    {true, true},
	} {
		got, err := task.GetView(view)
		if err != nil {
			t.Fatal(err)
		}
		hasContent := len(got.Inputs) == 1 && got.Inputs[0].Content == "secret"
		if got.Id != "task1" || got.State != State_RUNNING || (got.Name == "name") != want[0] || hasContent != want[1] {
			t.Errorf("view %q: %v", view, got)
		}
	}
	if _, err := task.GetView("BOGUS"); err == nil {
		t.Error("an unknown view must fail")
	}
	if task.Inputs[0].Content != "secret" {
		t.Error("the task must not be changed")
	}
}
