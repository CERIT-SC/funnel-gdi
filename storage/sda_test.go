package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
)

// fakeSDA is an SDA-Download service for tests. It requires the Bearer token
// "good-token" and records the last request.
type fakeSDA struct {
	*httptest.Server
	mu   sync.Mutex
	last *http.Request
}

func newFakeSDA(t *testing.T) *fakeSDA {
	t.Helper()
	f := &fakeSDA{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.last = r.Clone(context.Background())
		f.mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.Header().Set("ETag", `"etag-1"`)
		switch r.URL.Path {
		case "/api/s3/":
			w.Write([]byte(`<ListAllMyBucketsResult><Buckets>
				<Bucket><Name>dataset1</Name><CreationDate>2024-01-02T03:04:05Z</CreationDate></Bucket>
				<Bucket><Name>dataset2</Name><CreationDate>2024-02-02T03:04:05Z</CreationDate></Bucket>
			</Buckets></ListAllMyBucketsResult>`))
		case "/api/s3/dataset1/":
			w.Write([]byte(`<ListBucketResult>
				<Contents><Key>dir/a.txt</Key><Size>5</Size></Contents>
				<Contents><Key>b.c4gh</Key><Size>100</Size></Contents>
			</ListBucketResult>`))
		case "/api/s3/dataset1/dir/a.txt":
			w.Header().Set("Content-Length", "5")
			w.Write([]byte("hello"))
		case "/api/s3-encrypted/dataset1/b.c4gh":
			if r.Header.Get("Client-Public-Key") == "" {
				http.Error(w, "public key required", http.StatusBadRequest)
				return
			}
			w.Write([]byte("this is not crypt4gh data"))
		case "/api/s3/broken":
			w.Write([]byte("<not xml"))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSDA) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func newTestSDA(t *testing.T, f *fakeSDA) *SDA {
	t.Helper()
	// Keep the Crypt4GH keys of the test out of ~/.c4gh.
	dir := t.TempDir()
	t.Setenv("C4GH_SECRET_KEY", filepath.Join(dir, "key"))
	t.Setenv("C4GH_PUBLIC_KEY", filepath.Join(dir, "key.pub"))
	s, err := NewSDA(&config.SDAStorage{ServiceURL: f.URL + "/api/"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSDAUnsupported(t *testing.T) {
	s := newTestSDA(t, newFakeSDA(t))
	ops := s.UnsupportedOperations("sda://dataset1/file")
	if ops.Join == nil || ops.Put == nil || ops.Get != nil || ops.List != nil || ops.Stat != nil {
		t.Errorf("sda:// operations %+v", ops)
	}
	if ops := s.UnsupportedOperations("s3://bucket/key"); ops.Get == nil {
		t.Error("other protocols must be unsupported")
	}
	if _, err := s.Join("sda://d/", "x"); err == nil {
		t.Error("Join must fail")
	}
	if _, err := s.Put(context.Background(), "sda://d/x", "/tmp/x"); err == nil {
		t.Error("Put must fail")
	}
}

func TestSDAGet(t *testing.T) {
	f := newFakeSDA(t)
	s := newTestSDA(t, f)
	dest := filepath.Join(t.TempDir(), "a.txt")

	obj, err := s.Get(context.Background(), "sda://dataset1/dir/a.txt#good-token", dest)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest); string(data) != "hello" {
		t.Errorf("downloaded %q", data)
	}
	if obj.ETag != `"etag-1"` || obj.LastModified.IsZero() {
		t.Errorf("object %+v", obj)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.Header.Get("Client-Public-Key") != "" {
		t.Errorf("plain files are requested without a public key: %s %v", req.Method, req.Header)
	}
}

func TestSDAGetEncrypted(t *testing.T) {
	f := newFakeSDA(t)
	s := newTestSDA(t, f)
	dest := filepath.Join(t.TempDir(), "b")

	// The service re-encrypts .c4gh files for the key sent in the request;
	// a payload that is not Crypt4GH data cannot be decrypted.
	_, err := s.Get(context.Background(), "sda://dataset1/b.c4gh#good-token", dest)
	if err == nil || !strings.Contains(err.Error(), "decrypting") {
		t.Fatalf("got %v, want a decryption error", err)
	}
	if f.lastRequest().Header.Get("Client-Public-Key") == "" {
		t.Error("encrypted files must be requested with the Crypt4GH public key")
	}
}

func TestSDAStatAndList(t *testing.T) {
	s := newTestSDA(t, newFakeSDA(t))
	ctx := context.Background()

	obj, err := s.Stat(ctx, "sda://dataset1/dir/a.txt#good-token")
	if err != nil {
		t.Fatal(err)
	}
	if obj.URL != "sda://dataset1/dir/a.txt" || !strings.HasSuffix(obj.Name, "dir/a.txt") || obj.Size != 5 {
		t.Errorf("stat %+v (the token must not be part of the URL)", obj)
	}

	datasets, err := s.List(ctx, "sda://#good-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 2 || datasets[0].URL != "sda://dataset1/" || datasets[1].Name != "dataset2" || datasets[0].LastModified.IsZero() {
		t.Errorf("datasets %+v", datasets)
	}

	files, err := s.List(ctx, "sda://dataset1/#good-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].URL != "sda://dataset1/dir/a.txt" || files[0].Name != "dir/a.txt" || files[1].Size != 100 {
		t.Errorf("files %+v", files)
	}

	single, err := s.List(ctx, "sda://dataset1/dir/a.txt#good-token")
	if err != nil || len(single) != 1 || single[0].URL != "sda://dataset1/dir/a.txt" {
		t.Errorf("single object: %+v %v", single, err)
	}
}

func TestSDAErrors(t *testing.T) {
	s := newTestSDA(t, newFakeSDA(t))
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "x")

	for name, call := range map[string]func() error{
		"no token": func() error {
			_, err := s.Stat(ctx, "sda://dataset1/dir/a.txt")
			return err
		},
		"wrong token": func() error {
			_, err := s.Get(ctx, "sda://dataset1/dir/a.txt#bad-token", dest)
			return err
		},
		"missing file": func() error {
			_, err := s.Get(ctx, "sda://dataset1/missing.txt#good-token", dest)
			return err
		},
		"invalid listing": func() error {
			_, err := s.List(ctx, "sda://broken#good-token")
			return err
		},
		"destination not writable": func() error {
			_, err := s.Get(ctx, "sda://dataset1/dir/a.txt#good-token", filepath.Join(dest, "no", "such", "dir"))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSDAResolveURL(t *testing.T) {
	s := &SDA{conf: &config.SDAStorage{ServiceURL: "https://sda.example/download/"}}
	for _, tc := range []struct {
		in, url, token string
	}{
		{"sda://ds/file.txt", "https://sda.example/download/s3/ds/file.txt", ""},
		{"sda://ds/file.c4gh#tok", "https://sda.example/download/s3-encrypted/ds/file.c4gh", "tok"},
		{"sda://ds/file.txt?startCoordinate=0&endCoordinate=10#tok", "https://sda.example/download/s3/ds/file.txt?startCoordinate=0&endCoordinate=10", "tok"},
	} {
		url, token, err := s.resolveUrl(tc.in)
		if err != nil || url != tc.url || token != tc.token {
			t.Errorf("%s: got %q %q %v, want %q %q", tc.in, url, token, err, tc.url, tc.token)
		}
	}
}
