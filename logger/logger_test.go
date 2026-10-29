package logger

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLevelsAndFormatters(t *testing.T) {
	for _, tc := range []struct {
		level     string
		formatter string
		logs      map[string]bool // which of Debug/Info/Warn/Error are written
	}{
		{"debug", "text", map[string]bool{"debug": true, "info": true, "warn": true, "error": true}},
		{"info", "json", map[string]bool{"info": true, "warn": true, "error": true}},
		{"WARNING", "text", map[string]bool{"warn": true, "error": true}},
		{"error", "json", map[string]bool{"error": true}},
		{"nonsense", "text", map[string]bool{"info": true, "warn": true, "error": true}},
	} {
		t.Run(tc.level+"/"+tc.formatter, func(t *testing.T) {
			conf := DefaultConfig()
			conf.Level = tc.level
			conf.Formatter = tc.formatter
			conf.JsonFormat = &JSONFormatConfig{}
			l := NewLogger("test", conf)
			var buf bytes.Buffer
			l.SetOutput(&buf)

			l.Debug("debug-msg")
			l.Info("info-msg", "key", "value")
			l.Warn("warn-msg")
			l.Error("error-msg", errors.New("boom"))

			out := buf.String()
			for _, lvl := range []string{"debug", "info", "warn", "error"} {
				if got := strings.Contains(out, lvl+"-msg"); got != tc.logs[lvl] {
					t.Errorf("%s message written: %v, want %v\n%s", lvl, got, tc.logs[lvl], out)
				}
			}
			if tc.formatter == "json" && tc.logs["info"] && !strings.Contains(out, `"key":"value"`) {
				t.Errorf("JSON output without fields: %s", out)
			}
		})
	}
}

func TestSubLogger(t *testing.T) {
	l := NewLogger("test", DefaultConfig())
	var buf bytes.Buffer
	l.SetOutput(&buf)
	l.Sub("child").WithFields("taskID", "t1").Info("hello")
	if !strings.Contains(buf.String(), "child") || !strings.Contains(buf.String(), "t1") {
		t.Errorf("output %q", buf.String())
	}
}
