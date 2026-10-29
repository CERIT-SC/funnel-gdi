package gce

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
)

func TestWithMetadataConfigNode(t *testing.T) {
	conf := config.DefaultConfig()
	meta := &Metadata{}
	meta.Instance.Name = "node-1"
	meta.Instance.Hostname = "node-1.internal"
	meta.Instance.Attributes.FunnelNodeServerAddress = "server.internal:9999"

	got, err := WithMetadataConfig(conf, meta)
	if err != nil {
		t.Fatal(err)
	}
	// A worker node takes its ID from the instance and talks to the server.
	if got.Node.ID != "node-1" || got.Server.HostName != "server.internal" || got.Server.RPCPort != "9999" {
		t.Errorf("node config: %v / %v", got.Node, got.Server)
	}
	if got.GoogleStorage == nil {
		t.Error("nodes use Google Cloud Storage")
	}
}

func TestWithMetadataConfigServer(t *testing.T) {
	conf := config.DefaultConfig()
	meta := &Metadata{}
	meta.Instance.Hostname = "server.internal"
	meta.Instance.Attributes.FunnelConfig = "Server:\n  HTTPPort: \"8123\"\n"

	got, err := WithMetadataConfig(conf, meta)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.HostName != "server.internal" || got.Server.HTTPPort != "8123" || got.Node.ID != "" {
		t.Errorf("server config: %v / %v", got.Server, got.Node)
	}

	meta.Instance.Attributes.FunnelConfig = "NoSuchKey: 1\n"
	if _, err := WithMetadataConfig(config.DefaultConfig(), meta); err == nil {
		t.Error("expected an error for an invalid config in the metadata")
	}
}

func TestLoadMetadataErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "missing header", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("bad") != "" {
			w.Write([]byte("not json"))
			return
		}
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := LoadMetadataFromURL(srv.URL); err == nil {
		t.Error("expected an error for HTTP 500")
	}
	if _, err := LoadMetadataFromURL("http://127.0.0.1:1"); err == nil {
		t.Error("expected an error for an unreachable server")
	}
}
