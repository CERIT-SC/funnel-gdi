package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/storage/htsget"
	"google.golang.org/protobuf/types/known/durationpb"
)

// newFakeHTSGET serves an HTSGET ticket per resource; data blocks are served
// under /blocks/ and require the headers the ticket names.
func newFakeHTSGET(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticket := func(urls ...htsget.Url) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(htsget.Response{FileInfo: htsget.FileInfo{Format: "BAM", Urls: urls}})
		}
		if !strings.HasPrefix(r.URL.Path, "/blocks/") && r.Header.Get("client-public-key") == "" {
			http.Error(w, "client-public-key header missing", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/reads/sample1":
			// The token from the URL must reach the service; whether with a
			// "Bearer " prefix or without is not decided here.
			if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != "good-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ticket(
				htsget.Url{Url: "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString([]byte("header|"))},
				htsget.Url{Url: srv.URL + "/blocks/1", Headers: map[string]string{"Range": "bytes=0-4"}},
				htsget.Url{Url: "data:text/plain,|footer"},
				htsget.Url{Url: "data:,"},
			)
		case "/variants/basic":
			if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "pw" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ticket(htsget.Url{Url: "data:,vcf"})
		case "/reads/error":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(htsget.Response{FileInfo: htsget.FileInfo{ErrorCode: "NotFound", ErrorMsg: "no such read"}})
		case "/reads/empty":
			ticket()
		case "/reads/html":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html/>"))
		case "/reads/bad-block":
			ticket(htsget.Url{Url: srv.URL + "/blocks/missing"})
		case "/reads/ftp-block":
			ticket(htsget.Url{Url: "ftp://example.org/block"})
		case "/reads/bad-base64":
			ticket(htsget.Url{Url: "data:;base64,!!!"})
		case "/reads/bad-data-url":
			ticket(htsget.Url{Url: "data:text/plain;no-comma-here"})
		case "/reads/not-crypt4gh":
			// Starts like a Crypt4GH file, but is not one.
			ticket(htsget.Url{Url: "data:;base64," + base64.StdEncoding.EncodeToString(append([]byte("crypt4gh\x01\x00\x00\x00"), make([]byte, 64)...))})
		case "/blocks/1":
			if r.Header.Get("Range") != "bytes=0-4" {
				http.Error(w, "range header missing", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte("reads"))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestHTSGET(t *testing.T, srv *httptest.Server) *HTSGET {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("C4GH_SECRET_KEY", filepath.Join(dir, "key"))
	t.Setenv("C4GH_PUBLIC_KEY", filepath.Join(dir, "key.pub"))
	store, err := NewHTSGET(&config.HTSGETStorage{
		ServiceURL: srv.URL + "/",
		Timeout:    &config.TimeoutConfig{TimeoutOption: &config.TimeoutConfig_Duration{Duration: durationpb.New(5e9)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestHTSGETGet(t *testing.T) {
	store := newTestHTSGET(t, newFakeHTSGET(t))
	dest := filepath.Join(t.TempDir(), "sub", "sample1.bam")

	obj, err := store.Get(context.Background(), "htsget://reads/sample1#good-token", dest)
	if err != nil {
		t.Fatal(err)
	}
	// The parts of the ticket are concatenated in order.
	if data, _ := os.ReadFile(dest); string(data) != "header|reads|footer" {
		t.Errorf("downloaded %q", data)
	}
	if obj.URL != "htsget://reads/sample1" || obj.Size != int64(len("header|reads|footer")) {
		t.Errorf("object %+v (the token must not be part of the URL)", obj)
	}

	// user:pass after '#' is sent as Basic authentication.
	dest2 := filepath.Join(t.TempDir(), "basic.vcf")
	if _, err := store.Get(context.Background(), "htsget://variants/basic#alice:pw", dest2); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest2); string(data) != "vcf" {
		t.Errorf("downloaded %q", data)
	}
}

func TestHTSGETGetErrors(t *testing.T) {
	store := newTestHTSGET(t, newFakeHTSGET(t))
	for _, tc := range []struct{ url, want string }{
		{"htsget://reads/sample1", "HTTP 401"},
		{"htsget://reads/sample1#wrong", "HTTP 401"},
		{"htsget://reads/error", "no such read"},
		{"htsget://reads/empty", "at least one URL"},
		{"htsget://reads/html", "text/html"},
		{"htsget://reads/missing", "HTTP 404"},
		{"htsget://reads/bad-block", "part 1/1"},
		{"htsget://reads/ftp-block", "Unsupported HTSGET URL"},
		{"htsget://reads/bad-base64", "part 1/1"},
		{"htsget://reads/bad-data-url", "comma-separator"},
		{"htsget://reads/not-crypt4gh", ""},
	} {
		t.Run(tc.url, func(t *testing.T) {
			_, err := store.Get(context.Background(), tc.url, filepath.Join(t.TempDir(), "out"))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

func TestHTSGETResolveURL(t *testing.T) {
	store := &HTSGET{conf: &config.HTSGETStorage{ServiceURL: "https://htsget.example/api/"}}
	for _, tc := range []struct {
		in, http, clean, token string
	}{
		{"htsget://reads/id1", "https://htsget.example/api/reads/id1", "htsget://reads/id1", ""},
		{"htsget://variants/id2?referenceName=chr1&start=10#tok", "https://htsget.example/api/variants/id2?referenceName=chr1&start=10", "htsget://variants/id2", "tok"},
		{"htsget://reads/id3#user:pass", "https://user:pass@htsget.example/api/reads/id3", "htsget://reads/id3", ""},
	} {
		httpURL, clean, token, err := store.resolveUrl(tc.in)
		if err != nil || httpURL != tc.http || clean != tc.clean || token != tc.token {
			t.Errorf("%s: got %q %q %q %v", tc.in, httpURL, clean, token, err)
		}
	}

	if ops := store.UnsupportedOperations("htsget://other/x"); ops.Get == nil {
		t.Error("only reads/ and variants/ are valid HTSGET URLs")
	}
}
