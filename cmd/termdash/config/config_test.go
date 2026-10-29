package config

import "testing"

func TestConfig(t *testing.T) {
	Init()
	if GetVal("sortField") != "id" || !GetSwitchVal("enableHeader") {
		t.Fatalf("defaults: %q %v", GetVal("sortField"), GetSwitchVal("enableHeader"))
	}
	Update("sortField", "name")
	if GetVal("sortField") != "name" {
		t.Error("Update")
	}
	before := GetSwitchVal("allTasks")
	Toggle("allTasks")
	if GetSwitchVal("allTasks") == before {
		t.Error("Toggle")
	}
	// Unknown keys have empty defaults.
	if GetVal("nope") != "" || GetSwitchVal("nope") || Get("nope").Label != "" {
		t.Error("unknown keys")
	}
}
