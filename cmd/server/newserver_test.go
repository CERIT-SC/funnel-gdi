package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	conf := config.DefaultConfig()
	dir := t.TempDir()
	conf.BoltDB.Path = filepath.Join(dir, "funnel.db")
	conf.Badger.Path = filepath.Join(dir, "badger")
	conf.Worker.WorkDir = filepath.Join(dir, "work")
	conf.Slurm.DisableReconciler = true
	conf.HTCondor.DisableReconciler = true
	conf.PBS.DisableReconciler = true
	return conf
}

// NewServer assembles the server from the configured database, event
// writers and compute backend.
func TestNewServer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*config.Config)
		err    string
	}{
		{"boltdb + local", func(c *config.Config) {}, ""},
		{"boltdb + manual scheduler", func(c *config.Config) { c.Compute = "manual" }, ""},
		{"boltdb + noop", func(c *config.Config) { c.Compute = "noop" }, ""},
		{"boltdb + slurm, log writer", func(c *config.Config) { c.Compute = "slurm"; c.EventWriters = []string{"log"} }, ""},
		{"boltdb + htcondor", func(c *config.Config) { c.Compute = "htcondor" }, ""},
		{"boltdb + pbs", func(c *config.Config) { c.Compute = "pbs" }, ""},
		{"boltdb + gridengine", func(c *config.Config) { c.Compute = "gridengine" }, ""},
		{"badger + local, boltdb writer", func(c *config.Config) { c.Database = "badger"; c.EventWriters = []string{"badger", "boltdb"} }, ""},
		{"badger cannot schedule", func(c *config.Config) { c.Database = "badger"; c.Compute = "manual" }, "does not implement the scheduler service"},
		{"unknown database", func(c *config.Config) { c.Database = "nosuchdb" }, "unknown database"},
		{"unknown event writer", func(c *config.Config) { c.EventWriters = []string{"carrier-pigeon"} }, "unknown event writer"},
		{"unknown compute", func(c *config.Config) { c.Compute = "mainframe" }, "unknown compute backend"},
		{"kubernetes without template", func(c *config.Config) { c.Compute = "kubernetes"; c.Kubernetes.WorkerTemplate = "" }, "job template"},
		{"plugin without path", func(c *config.Config) { c.Plugins = &config.Plugins{} }, "'Path' is not found"},
		{"plugin that does not exist", func(c *config.Config) { c.Plugins = &config.Plugins{Path: "/no/such/plugin"} }, "plugin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := testConfig(t)
			tc.modify(conf)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			srv, err := NewServer(ctx, conf, logger.NewLogger("test", logger.DefaultConfig()))
			if tc.err == "" {
				if err != nil || srv == nil || srv.Server == nil {
					t.Fatalf("got %v", err)
				}
				if (conf.Compute == "manual") != (srv.Scheduler != nil) {
					t.Errorf("scheduler %v for compute %s", srv.Scheduler, conf.Compute)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("got %v, want an error containing %q", err, tc.err)
			}
		})
	}
}
