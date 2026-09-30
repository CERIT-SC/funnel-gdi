package server

import (
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
	"golang.org/x/net/context"
)

func TestReplaceInputBearerToken(t *testing.T) {
	newTask := func() *tes.Task {
		return &tes.Task{Inputs: []*tes.Input{
			{Url: "sda://dataset/file.c4gh"},
			{Url: "htsget://reads/sample"},
			{Url: "sda://dataset/explicit#other-token"},
			{Url: "s3://bucket/key"},
		}}
	}

	ctx := context.WithValue(context.Background(), UserInfoKey, &UserInfo{Username: "u", Token: "jwt"})
	task := newTask()
	if err := ReplaceInputBearerToken(ctx, task); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"sda://dataset/file.c4gh#jwt",
		"htsget://reads/sample#jwt",
		"sda://dataset/explicit#other-token",
		"s3://bucket/key",
	}
	for i, in := range task.Inputs {
		if in.Url != want[i] {
			t.Errorf("input %d: got %q, want %q", i, in.Url, want[i])
		}
	}

	// Without a token, SDA inputs (which always require one) are rejected.
	if err := ReplaceInputBearerToken(context.Background(), newTask()); err == nil {
		t.Error("expected an error for an SDA input without a Bearer token")
	}

	// HTSGET inputs are allowed without a token (public HTSGET services).
	task = &tes.Task{Inputs: []*tes.Input{{Url: "htsget://variants/sample"}}}
	if err := ReplaceInputBearerToken(context.Background(), task); err != nil {
		t.Errorf("unexpected error for an HTSGET input without a token: %v", err)
	}
	if task.Inputs[0].Url != "htsget://variants/sample" {
		t.Errorf("HTSGET input URL changed without a token: %q", task.Inputs[0].Url)
	}
}
