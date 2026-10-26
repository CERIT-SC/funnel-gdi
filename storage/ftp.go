package storage

import (
	"context"
	"fmt"
	"io"
	"net/textproto"
	urllib "net/url"
	"os"
	pathlib "path"
	"strings"

	"github.com/jlaffaye/ftp"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/util/fsutil"
)

// FTP provides read access to public URLs.
type FTP struct {
	conf *config.FTPStorage
}

// NewFTP creates a new FTP instance.
func NewFTP(conf *config.FTPStorage) (*FTP, error) {
	return &FTP{conf: conf}, nil
}

// Stat returns information about the object at the given storage URL.
func (b *FTP) Stat(ctx context.Context, url string) (*Object, error) {
	client, err := connect(url, b.conf)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Stat(ctx, url)
}

// Get copies a file from a given URL to the host path.
func (b *FTP) Get(ctx context.Context, url, path string) (*Object, error) {
	client, err := connect(url, b.conf)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Get(ctx, url, path)
}

// Put is not supported by FTP storage.
func (b *FTP) Put(ctx context.Context, url string, hostPath string) (*Object, error) {
	client, err := connect(url, b.conf)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Put(ctx, url, hostPath)
}

// Join joins the given URL with the given subpath.
func (b *FTP) Join(url, path string) (string, error) {
	return ftpJoin(url, path)
}

// List is not supported by FTP storage.
func (b *FTP) List(ctx context.Context, url string) ([]*Object, error) {
	client, err := connect(url, b.conf)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.List(ctx, url)
}

// UnsupportedOperations describes which operations (Get, Put, etc) are not
// supported for the given URL.
func (b *FTP) UnsupportedOperations(url string) UnsupportedOperations {
	if err := b.supportsPrefix(url); err != nil {
		return AllUnsupported(err)
	}
	return AllSupported()
}

func (b *FTP) supportsPrefix(url string) error {
	if !strings.HasPrefix(url, "ftp://") && !strings.HasPrefix(url, "sftp://") {
		return &ErrUnsupportedProtocol{"ftpStorage"}
	}
	return nil
}

// ftpclient exists implements the storage API and reuses an FTP client
// for recursive calls.
type ftpclient struct {
	client *ftp.ServerConn
}

func connect(url string, conf *config.FTPStorage) (*ftpclient, error) {
	u, err := urllib.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: parsing URL: %s", err)
	}

	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "sftp" {
			host += ":22"
		} else {
			host += ":21"
		}
	}

	client, err := ftp.Dial(host)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: connecting to server: %v", err)
	}

	user := conf.User
	pass := conf.Password

	if u.User != nil {
		user = u.User.Username()
		// "anonymous" doesn't make sense if there's a username,
		// so clear it. Then check if the password is set by the URL.
		p, ok := u.User.Password()
		if !ok {
			pass = ""
		} else {
			pass = p
		}
	}

	err = client.Login(user, pass)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: logging in: %v", err)
	}
	return &ftpclient{client}, nil
}

func (b *ftpclient) Close() {
	b.client.Logout()
	b.client.Quit()
}

// Stat returns information about the object at the given storage URL.
func (b *ftpclient) Stat(ctx context.Context, url string) (*Object, error) {
	u, err := urllib.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: parsing URL: %s", err)
	}

	r, err := b.entry(u.Path)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: stat %q: %v", u.Path, err)
	}

	// TODO there is a "link" file type. can we support that?
	if r.Type != ftp.EntryTypeFile {
		return nil, fmt.Errorf("ftpStorage: stat on non-regular file type: %s", url)
	}

	return &Object{
		URL:          url,
		Name:         strings.TrimPrefix(u.Path, "/"),
		LastModified: r.Time,
		Size:         int64(r.Size),
	}, nil
}

// Get copies a file from a given URL to the host path.
func (b *ftpclient) Get(ctx context.Context, url, path string) (*Object, error) {
	obj, err := b.Stat(ctx, url)
	if err != nil {
		return nil, err
	}

	// The absolute path: entry() may have changed the working directory.
	src, err := b.client.Retr("/" + obj.Name)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: executing RETR request: %s", err)
	}
	defer src.Close()

	dest, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: creating host file: %s", err)
	}

	_, copyErr := io.Copy(dest, fsutil.Reader(ctx, src))
	closeErr := dest.Close()

	if copyErr != nil {
		return nil, fmt.Errorf("ftpStorage: copying file: %s", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("ftpStorage: closing file: %s", closeErr)
	}

	return obj, err
}

func (b *ftpclient) Put(ctx context.Context, url string, hostPath string) (*Object, error) {

	u, err := urllib.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: parsing URL: %s", err)
	}

	reader, err := os.Open(hostPath)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: opening host file for %q: %v", url, err)
	}
	defer reader.Close()

	dirpath, name := pathlib.Split(u.Path)
	if dirpath != "" {
		for _, dir := range strings.Split(strings.Trim(dirpath, "/"), "/") {
			if dir == "" {
				continue // a file in the root directory
			}
			err := b.client.ChangeDir(dir)
			if isUnavailable(err) {
				// Directory doesn't exist. Create it.
				err = b.client.MakeDir(dir)
				// It's possible that the directory was created by a concurrent process.
				// In that case, allow the ChangeDir call below to retry.
				if isUnavailable(err) {
					err = nil
				}
				if err == nil {
					err = b.client.ChangeDir(dir)
				}
			}

			if err != nil {
				return nil, fmt.Errorf("ftpStorage: changing directory to %q: %v", dir, err)
			}
		}
	}

	err = b.client.Stor(name, reader)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: uploading file for %q: %v", url, err)
	}

	return b.Stat(ctx, url)
}

// entry describes the file or directory at the given absolute path. MLST
// describes either. A server without MLST gets a CWD, which only succeeds for a
// directory, and otherwise a LIST, which lists a regular file as itself (a LIST
// of a directory holding a single file looks the same, hence the CWD first).
// MLSD, which List uses when the server has MLST, fails for a regular file.
func (b *ftpclient) entry(path string) (*ftp.Entry, error) {
	e, err := b.client.GetEntry(path)
	if err == nil {
		return e, nil
	}
	if te, ok := err.(*textproto.Error); !ok || te.Code != ftp.StatusNotImplemented {
		return nil, err
	}

	if b.client.ChangeDir(path) == nil {
		return &ftp.Entry{Name: pathlib.Base(path), Type: ftp.EntryTypeFolder}, nil
	}

	resp, err := b.client.List(path)
	if err != nil {
		return nil, err
	}
	if len(resp) != 1 || resp[0].Type != ftp.EntryTypeFile {
		return nil, errFTPNotFound
	}
	return resp[0], nil
}

var errFTPNotFound = fmt.Errorf("object not found")

func isUnavailable(err error) bool {
	e, ok := err.(*textproto.Error)
	return ok && e.Code == ftp.StatusFileUnavailable
}

func (b *ftpclient) List(ctx context.Context, url string) ([]*Object, error) {
	u, err := urllib.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: parsing URL: %s", err)
	}

	r, err := b.entry(u.Path)
	if err == errFTPNotFound || isUnavailable(err) {
		// A missing path lists as empty, as with LIST.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: listing path: %q %v", u.Path, err)
	}

	// Special case where the user called List on a regular file.
	if r.Type == ftp.EntryTypeFile {
		return []*Object{
			{
				URL:          url,
				Name:         strings.TrimPrefix(u.Path, "/"),
				LastModified: r.Time,
				Size:         int64(r.Size),
			},
		}, nil
	}

	resp, err := b.client.List(u.Path)
	if err != nil {
		return nil, fmt.Errorf("ftpStorage: listing path: %q %v", u.Path, err)
	}

	// List the objects, recursively.
	var objects []*Object
	for _, r := range resp {
		switch r.Type {

		case ftp.EntryTypeFolder:
			if r.Name == "." || r.Name == ".." {
				continue
			}

			joined, err := ftpJoin(url, r.Name)
			if err != nil {
				return nil, err
			}

			sub, err := b.List(ctx, joined)
			if err != nil {
				return nil, err
			}
			objects = append(objects, sub...)

		case ftp.EntryTypeLink:
			// Link type is currently not supported. Skip it.
			// TODO there is a "EntryTypeLink" type. can we support that?

		case ftp.EntryTypeFile:
			joined, err := ftpJoin(url, r.Name)
			if err != nil {
				return nil, err
			}

			obj := &Object{
				URL:          joined,
				Name:         strings.TrimPrefix(pathlib.Join(u.Path, r.Name), "/"),
				LastModified: r.Time,
				Size:         int64(r.Size),
			}
			objects = append(objects, obj)
		}
	}
	return objects, nil
}

// ftpJoin joins the given URL with the given subpath.
func ftpJoin(url, path string) (string, error) {
	return strings.TrimSuffix(url, "/") + "/" + path, nil
}
