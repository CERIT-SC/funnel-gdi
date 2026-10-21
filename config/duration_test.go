package config

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	var d Duration
	if err := d.Set("1m30s"); err != nil {
		t.Fatal(err)
	}
	if d.AsProto().AsDuration() != 90*time.Second {
		t.Errorf("got %v", d.AsProto().AsDuration())
	}
	text, err := d.MarshalText()
	if err != nil || string(text) == "" {
		t.Errorf("MarshalText: %q %v", text, err)
	}
	if d.Type() != "duration" {
		t.Errorf("Type %q", d.Type())
	}

	// Empty text leaves the value unchanged; invalid text is an error.
	if err := d.UnmarshalText(nil); err != nil || d.AsProto().AsDuration() != 90*time.Second {
		t.Errorf("empty: %v %v", err, d.AsProto().AsDuration())
	}
	if err := d.Set("ten minutes"); err == nil {
		t.Error("expected an error for an invalid duration")
	}
}
