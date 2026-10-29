package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/util"
)

// flakyBackend fails the first n calls of every operation.
type flakyBackend struct {
	failures int
	calls    int
}

func (f *flakyBackend) try() error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("temporary failure")
	}
	return nil
}

func (f *flakyBackend) Stat(context.Context, string) (*Object, error) { return &Object{}, f.try() }
func (f *flakyBackend) List(context.Context, string) ([]*Object, error) {
	return []*Object{{}}, f.try()
}
func (f *flakyBackend) Get(context.Context, string, string) (*Object, error) {
	return &Object{}, f.try()
}
func (f *flakyBackend) Put(context.Context, string, string) (*Object, error) {
	return &Object{}, f.try()
}
func (f *flakyBackend) Join(url, path string) (string, error) { return url + "/" + path, nil }
func (f *flakyBackend) UnsupportedOperations(string) UnsupportedOperations {
	return UnsupportedOperations{}
}

func TestRetrier(t *testing.T) {
	newRetrier := func(failures int) (*Retrier, *flakyBackend) {
		b := &flakyBackend{failures: failures}
		r := util.NewRetrier()
		r.InitialInterval = time.Millisecond
		r.MaxInterval = time.Millisecond
		r.MaxTries = 3
		r.ShouldRetry = func(error) bool { return true }
		return &Retrier{Retrier: r, Backend: b}, b
	}
	ctx := context.Background()

	for name, op := range map[string]func(*Retrier) error{
		"Stat": func(r *Retrier) error { _, err := r.Stat(ctx, "x"); return err },
		"List": func(r *Retrier) error { _, err := r.List(ctx, "x"); return err },
		"Get":  func(r *Retrier) error { _, err := r.Get(ctx, "x", "/tmp/x"); return err },
		"Put":  func(r *Retrier) error { _, err := r.Put(ctx, "x", "/tmp/x"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			r, b := newRetrier(2)
			if err := op(r); err != nil || b.calls != 3 {
				t.Errorf("two failures then success: err %v after %d calls", err, b.calls)
			}
			r, _ = newRetrier(10)
			if err := op(r); err == nil {
				t.Error("expected an error when every try fails")
			}
		})
	}

	r, _ := newRetrier(0)
	if u, _ := r.Join("s3://b", "k"); u != "s3://b/k" {
		t.Errorf("Join %q", u)
	}
	if ops := r.UnsupportedOperations("x"); ops.Get != nil {
		t.Error("UnsupportedOperations must come from the backend")
	}
}

func TestMuxBackends(t *testing.T) {
	conf := config.DefaultConfig()
	allowed := t.TempDir()
	conf.LocalStorage = &config.LocalStorage{AllowedDirs: []string{allowed}}
	conf.HTTPStorage = &config.HTTPStorage{Timeout: conf.HTTPStorage.GetTimeout()}
	conf.HTSGETStorage = &config.HTSGETStorage{ServiceURL: "https://htsget.example/"}
	conf.SDAStorage = &config.SDAStorage{ServiceURL: "https://sda.example/"}
	conf.AmazonS3.Disabled = true
	conf.GoogleStorage.Disabled = true
	conf.Swift.Disabled = true
	conf.FTPStorage.Disabled = true

	mux, err := NewMux(conf)
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"file://" + allowed + "/x", "https://example.org/x", "htsget://reads/x", "sda://dataset/x"} {
		if ops := mux.UnsupportedOperations(url); ops.Get != nil {
			t.Errorf("%s: Get unsupported: %v", url, ops.Get)
		}
	}
	ctx := context.Background()
	if _, err := mux.Get(ctx, "gs://bucket/key", "/tmp/x"); err == nil {
		t.Error("gs:// must fail when Google Storage is disabled")
	}
	if _, err := mux.Stat(ctx, "nope://x"); err == nil {
		t.Error("unknown protocol must fail")
	}
	if _, err := mux.Join("nope://x", "y"); err == nil {
		t.Error("Join with an unknown protocol must fail")
	}
}
