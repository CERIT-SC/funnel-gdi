package storage

import (
	"testing"

	"github.com/alecthomas/units"

	"github.com/ohsu-comp-bio/funnel/config"
)

// The chunk size of large uploads is kept between 100 MB (else 500 MB) and
// 5 GB. Operations against a Swift server are not unit-tested: NewSwift does
// not authenticate the connection, and ncw/swift panics on an
// unauthenticated one (reported).
func TestSwiftRetrierConfig(t *testing.T) {
	r, err := NewSwiftRetrier(&config.SwiftStorage{MaxRetries: 3, ChunkSizeBytes: 1})
	if err != nil || r.Retrier.MaxTries != 3 || r.Backend.(*Swift).chunkSize != int64(500*units.MB) {
		t.Fatalf("%+v %v", r, err)
	}
	big, _ := NewSwift(&config.SwiftStorage{ChunkSizeBytes: int64(10 * units.GB)})
	if big.chunkSize != int64(5*units.GB) {
		t.Errorf("chunk size %d", big.chunkSize)
	}
	mid, _ := NewSwift(&config.SwiftStorage{ChunkSizeBytes: int64(200 * units.MB)})
	if mid.chunkSize != int64(200*units.MB) {
		t.Errorf("chunk size %d", mid.chunkSize)
	}
}
