package util

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
)

func TestMergeConfigFileWithFlags(t *testing.T) {
	fileConfig := config.DefaultConfig()
	flagConf := &config.Config{
		Server: &config.Server{
			HostName: "test",
			RPCPort:  "9999",
		},
	}
	serverAddress := flagConf.Server.RPCAddress()

	result, err := MergeConfigFileWithFlags("", flagConf)
	if err != nil {
		t.Error("unexpected error", err)
	}
	if result.Server == nil {
		t.Fatal("unexpected nil Server config")
	}
	if result.Server.RPCAddress() != serverAddress {
		t.Error("unexpected server address")
	}
	if result.Server.HTTPPort != fileConfig.Server.HTTPPort {
		t.Error("expected Config.Server.HTTPPort to equal the value from from config.DefaultValue()")
	}
	if result.RPCClient.ServerAddress != serverAddress {
		t.Error("unexpected Config.RPCClient.ServerAddress")
	}
	if result.Compute != fileConfig.Compute {
		t.Error("expected Config.Compute to equal default value from config.DefaultValue()")
	}
	if got, want := result.Kubernetes.ForbiddenPathPrefixes, []string{"/dev", "/proc", "/sys", "/run", "/var/run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected runtime deny-list defaults %v, got %v", want, got)
	}

	fileConfig.Server.HTTPPort = "8888"
	tmp, cleanup := TempConfigFile(fileConfig, "testconfig.yaml")
	defer cleanup()
	result, err = MergeConfigFileWithFlags(tmp, flagConf)
	if err != nil {
		t.Error("unexpected error", err)
	}
	if result.Server.RPCAddress() != serverAddress {
		t.Error("unexpected server address")
	}
	if result.RPCClient.ServerAddress != serverAddress {
		t.Error("unexpected Config.RPCClient.ServerAddress")
	}
	if result.Server.HTTPPort != fileConfig.Server.HTTPPort {
		t.Error("expected Config.Server.HTTPPort to equal the value from the config file")
	}
	if result.Compute != fileConfig.Compute {
		t.Error("expected Config.Compute to equal default value from config.DefaultValue()")
	}
}

func TestMergeConfigFileReplacesForbiddenPathPrefixes(t *testing.T) {
	fileConfig := config.DefaultConfig()
	fileConfig.Kubernetes.ForbiddenPathPrefixes = []string{"/secret"}
	tmp, cleanup := TempConfigFile(fileConfig, "testconfig.yaml")
	defer cleanup()

	result, err := MergeConfigFileWithFlags(tmp, config.EmptyConfig())
	if err != nil {
		t.Fatal("unexpected error", err)
	}

	if got, want := result.Kubernetes.ForbiddenPathPrefixes, []string{"/secret"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected configured deny list %v, got %v", want, got)
	}
}

// Lists from the config file replace the default lists instead of being
// appended to them; other fields keep their defaults.
func TestMergeConfigFileReplacesLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "funnel.yml")
	err := os.WriteFile(path, []byte("EventWriters:\n  - log\n  - kafka\nMongoDB:\n  Addrs:\n    - localhost:27000\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	result, err := MergeConfigFileWithFlags(path, config.EmptyConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.MongoDB.Addrs, []string{"localhost:27000"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MongoDB.Addrs %v, want %v", got, want)
	}
	if got, want := result.EventWriters, []string{"log", "kafka"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EventWriters %v, want %v", got, want)
	}
	def := config.DefaultConfig()
	if result.MongoDB.Database != def.MongoDB.Database || !reflect.DeepEqual(result.Kubernetes.ForbiddenPathPrefixes, def.Kubernetes.ForbiddenPathPrefixes) {
		t.Errorf("defaults lost: %q %v", result.MongoDB.Database, result.Kubernetes.ForbiddenPathPrefixes)
	}
}
