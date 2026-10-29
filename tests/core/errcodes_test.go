package core

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/tests"
)

// Error codes reach HTTP clients unchanged, with the TES status code.
func TestHTTPErrorCodes(t *testing.T) {
	tests.SetLogOutput(log, t)

	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"unknown task", http.MethodGet, "/v1/tasks/does-not-exist", "", http.StatusNotFound, "FUNNEL-TASK-001: "},
		{"cancel unknown task", http.MethodPost, "/v1/tasks/does-not-exist:cancel", "", http.StatusNotFound, "FUNNEL-TASK-001: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, fun.Conf.Server.HTTPAddress()+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Errorf("HTTP %d, want %d (%s)", resp.StatusCode, tc.status, body)
			}
			if !strings.Contains(string(body), tc.code) {
				t.Errorf("response %s does not contain %q", body, tc.code)
			}
		})
	}
}
