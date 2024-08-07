package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/storage/htsget"
)

const (
	protocolPrefix = "htsget://"
	protocolBearer = protocolPrefix + "bearer:"
)

// HTSGET provides read-access to public URLs.
// It is a client implementation based on the specification
// http://samtools.github.io/hts-specs/htsget.html
// HTSGET URLs need to provided in Funnel tasks as
// `htsget://[bearer:token@]host/path/to/api/{reads|variants}/resource-id`
// Where a Bearer token can be optionally specified to forward JWT credentials.
type HTSGET struct {
	conf config.HTSGETStorage
}

// NewHTSGET creates a new HTSGET instance based on the provided configuration.
func NewHTSGET(conf config.HTSGETStorage) (*HTSGET, error) {
	return &HTSGET{conf: conf}, nil
}

// Join a directory URL with a subpath. Not supported with HTSGET.
func (b *HTSGET) Join(url, path string) (string, error) {
	return "", fmt.Errorf("htsgetStorage: Join operation is not supported")
}

// Stat returns information about the object at the given storage URL. Not supported with HTSGET.
func (b *HTSGET) Stat(ctx context.Context, url string) (*Object, error) {
	return nil, fmt.Errorf("htsgetStorage: Stat operation is not supported")
}

// List a directory. Calling List on a File is an error. Not supported with HTSGET.
func (b *HTSGET) List(ctx context.Context, url string) ([]*Object, error) {
	return nil, fmt.Errorf("htsgetStorage: List operation is not supported")
}

// Not supported with HTSGET.
func (b *HTSGET) Put(ctx context.Context, url, path string) (*Object, error) {
	return nil, fmt.Errorf("htsgetStorage: Put operation is not supported")
}

// Get operation copies a file from a given URL to the host path.
//
// If configuration specifies sending a public key, the received content will
// be also decrypted locally before writing to the file.
func (b *HTSGET) Get(ctx context.Context, url, path string) (*Object, error) {
	httpsUrl, token := htsgetUrl(url, b.conf.Protocol)

	client := htsget.NewHtsgetClient(httpsUrl, token, time.Duration(b.conf.Timeout))
	err := client.DownloadTo(path)
	if err != nil {
		return nil, err
	}

	// Check that the destination file exists:
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &Object{
		URL:          url,
		Name:         path,
		Size:         info.Size(),
		LastModified: info.ModTime(),
	}, nil
}

// UnsupportedOperations describes which operations (Get, Put, etc) are not
// supported for the given URL.
func (b *HTSGET) UnsupportedOperations(url string) UnsupportedOperations {
	if err := b.supportsPrefix(url); err != nil {
		return AllUnsupported(err)
	}

	ops := UnsupportedOperations{
		List: fmt.Errorf("htsgetStorage: List operation is not supported"),
		Put:  fmt.Errorf("htsgetStorage: Put operation is not supported"),
		Join: fmt.Errorf("htsgetStorage: Join operation is not supported"),
		Stat: fmt.Errorf("htsgetStorage: Stat operation is not supported"),
	}
	return ops
}

func (b *HTSGET) supportsPrefix(url string) error {
	if !strings.HasPrefix(url, protocolPrefix) {
		return &ErrUnsupportedProtocol{"htsgetStorage"}
	}
	return nil
}

func htsgetUrl(url, useProtocol string) (updatedUrl string, token string) {
	if useProtocol == "" {
		useProtocol = "https"
	}
	useProtocol += "://"
	updatedUrl = strings.Replace(url, protocolPrefix, useProtocol, 1)

	// Optional info: parse the "token" from "htsget://bearer:token@host..."
	if strings.HasPrefix(url, protocolBearer) {
		bearerStart := len(protocolBearer)
		bearerStop := strings.Index(url, "@")

		if bearerStop > bearerStart {
			updatedUrl = useProtocol + url[bearerStop+1:]
			token = url[bearerStart:bearerStop]
		}
	}

	return updatedUrl, token
}
