package local

import (
	"context"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/plugins/proto"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// With an authorization plugin, the plugin's response may change the
// configuration the task runs with.
func TestPluginConfig(t *testing.T) {
	conf := config.DefaultConfig()
	conf.Plugins = &config.Plugins{Path: "plugin"}
	b, _ := NewBackend(context.Background(), conf, logger.NewLogger("test", logger.DefaultConfig()))

	// Without a plugin response the event is rejected.
	if err := b.WriteEvent(context.Background(), events.NewState("t1", tes.Running)); err == nil {
		t.Error("expected an error without a plugin response")
	}

	pluginConf := config.EmptyConfig()
	pluginConf.Worker = &config.Worker{WorkDir: "/plugin/workdir"}
	ctx := context.WithValue(context.Background(), "pluginResponse", &proto.JobResponse{Config: pluginConf})
	// Events other than TASK_CREATED only update the configuration.
	if err := b.WriteEvent(ctx, events.NewState("t1", tes.Running)); err != nil {
		t.Fatal(err)
	}
	if b.conf.Worker.WorkDir != "/plugin/workdir" {
		t.Errorf("work dir %q: the plugin config was not merged", b.conf.Worker.WorkDir)
	}

	// A response without a config leaves the configuration as it is.
	ctx = context.WithValue(context.Background(), "pluginResponse", &proto.JobResponse{})
	if err := b.UpdateConfig(ctx); err != nil || b.conf.Worker.WorkDir != "/plugin/workdir" {
		t.Errorf("%v %q", err, b.conf.Worker.WorkDir)
	}
}
