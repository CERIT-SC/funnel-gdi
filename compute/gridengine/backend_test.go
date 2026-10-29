package gridengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
)

func TestBackend(t *testing.T) {
	if got := extractID("Your job 7 (\"funnel-task\") has been submitted\n"); got != "7" {
		t.Errorf("extractID: %q", got)
	}
	tpl := filepath.Join(t.TempDir(), "sge.tpl")
	os.WriteFile(tpl, []byte("#$ -N {{.TaskId}}"), 0o644)
	conf := config.DefaultConfig()
	conf.GridEngine.TemplateFile = tpl
	b, err := NewBackend(conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig()))
	if err != nil || b.Template != "#$ -N {{.TaskId}}" || b.SubmitCmd != "qsub" || b.MapStates != nil {
		t.Fatalf("%+v %v", b, err)
	}
	conf.GridEngine.TemplateFile = filepath.Join(t.TempDir(), "missing")
	if _, err := NewBackend(conf, nil, nil, nil); err == nil {
		t.Error("expected an error for a missing template file")
	}
}
