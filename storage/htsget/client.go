package htsget

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ohsu-comp-bio/funnel/storage/crypt4gh"
)

// The main struct for holding the data of an HTSGET client instance
type HtsgetClient struct {
	Timeout       time.Duration
	Url           string
	authorization string
	keyPair       *crypt4gh.Crypt4ghKeyPair
}

// JSON struct (HTSGET response) for holding an item to be fetched
type HtsgetUrl struct {
	Url       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	DataClass string            `json:"class"`
}

// JSON struct (HTSGET response) for holding items to be fetched
type HtsgetFileInfo struct {
	Format string      `json:"format"`
	Urls   []HtsgetUrl `json:"urls"`
}

// Main JSON struct (HTSGET response)
type HtsgetResponse struct {
	FileInfo HtsgetFileInfo `json:"htsget"`
}

// Returns a new HTSGET client for fetching an HTSGET resource.
// Optionally, a value can be provided for the Authorization header (in the
// HTTP request). A timeout limit (per request) is also expected.
func NewHtsgetClient(url, authorization string, timeout time.Duration) *HtsgetClient {
	keys, err := crypt4gh.ResolveKeyPair()
	if err != nil {
		fmt.Println("[WARN] Minor issue while resolving Crypt4gh key-pair:", err)
	}
	return &HtsgetClient{timeout, url, authorization, keys}
}

// Downloads the HTSGET resource (specified when the client was created) to the
// specified local file path. This method ensures that the data gets copied to
// the specified file, or it returns an error to indicate a failure.
func (hc *HtsgetClient) DownloadTo(destFile string) error {
	fileInfo, err := hc.fetchHtsgetFileInfo()
	if err != nil {
		return err
	}

	tempFile, err := fileInfo.downloadPartsToTempFile(hc.Timeout)
	if err != nil {
		return err
	}

	err = os.MkdirAll(path.Dir(destFile), 0700)
	if err != nil {
		return err
	}

	if crypt4gh.IsCrypt4ghFile(tempFile) {
		return hc.decryptFile(tempFile, destFile)
	}

	err = os.Rename(tempFile, destFile)
	if err != nil {
		return errors.Join(errors.New("Cannot move the downloaded file to target file-path"), err)
	}
	return nil
}

// Performs the initial HTSGET request and returns the extracted JSON.
func (hc *HtsgetClient) fetchHtsgetFileInfo() (*HtsgetFileInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hc.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", hc.Url, nil)
	if err != nil {
		return nil, err
	}

	if hc.authorization != "" {
		req.Header.Add("Authorization", hc.authorization)
	}

	req.Header.Add("client-public-key", hc.keyPair.EncodePublicKeyBase64())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != 200 && contentType != "application/json" {
		return nil, fmt.Errorf("Bad response from HTSGET service: "+
			"HTTP [%d] content-type [%s]", resp.StatusCode, contentType)
	}

	var parsedJson HtsgetResponse
	err = json.NewDecoder(resp.Body).Decode(&parsedJson)
	if err != nil {
		return nil, err
	}

	if len(parsedJson.FileInfo.Urls) == 0 {
		return nil, errors.New("Bad JSON from the HTSGET service: expected a " +
			"JSON object with the 'htsget.urls' array with at least one URL")
	}

	return &parsedJson.FileInfo, nil
}

// Decrypts (Crypt4gh) the temporaray file to the final file path.
// Does not remove the temporary file.
func (hc *HtsgetClient) decryptFile(tempFile, destFile string) error {
	defer os.Remove(tempFile)

	tempStream, err := os.Open(tempFile)
	if err != nil {
		return errors.Join(fmt.Errorf("Failed to read the downloaded file: %s", tempFile), err)
	}

	defer tempStream.Close()

	destStream, err := os.Create(destFile)
	if err != nil {
		return errors.Join(fmt.Errorf("Failed to write to the target file: %s", destFile), err)
	}

	defer destStream.Close()

	decryptedStrem, err := hc.keyPair.Decrypt(tempStream)
	if err != nil {
		return err
	}

	_, err = io.Copy(destStream, decryptedStrem)
	return err
}

// Downloads parts of the HTSGET resource to a temporary file.
func (fi *HtsgetFileInfo) downloadPartsToTempFile(timeout time.Duration) (string, error) {
	file, err := os.CreateTemp("", "htsget.partial")
	if err != nil {
		return "", err
	}
	defer file.Close()

	for i := range fi.Urls {
		err := fi.Urls[i].copyTo(file, timeout)

		if err != nil {
			return "", errors.Join(
				fmt.Errorf("Failed to retrieve HTSGET file part %d/%d", i+1,
					len(fi.Urls)), err)
		}
	}

	return file.Name(), nil
}

// Downloads or copies the current part of data to the specified file-writer.
func (hu *HtsgetUrl) copyTo(dst io.Writer, timeout time.Duration) error {
	if strings.HasPrefix(hu.Url, "data:") {
		return hu.copyFromData(dst)
	}

	if strings.HasPrefix(hu.Url, "https:") || strings.HasPrefix(hu.Url, "http:") {
		return hu.copyFromHttp(dst, timeout)
	}

	return fmt.Errorf("Unsupported HTSGET URL: [%s]", hu.Url)
}

// Decodes the current (BASE64) part of data to the specified file-writer.
func (hu *HtsgetUrl) copyFromData(dst io.Writer) error {
	url := hu.Url

	contentSepPos := strings.Index(url, ",")
	if contentSepPos < 0 {
		return fmt.Errorf("Received invalid data-URL: [%s...] (comma-separator not found)", url[:20])
	}

	content := url[contentSepPos+1:]
	if len(content) == 0 {
		return nil
	}

	// Write the content as-is when there is no ";base64":
	base64Pos := strings.Index(url, ";base64")
	if base64Pos < 0 || base64Pos > contentSepPos {
		_, err := dst.Write([]byte(content))
		return err
	}

	// The content needs to be decoded from base64:
	b, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return err
	}

	_, err = dst.Write(b)
	return err
}

// Downloads the current part of data (over HTTP) to the specified file-writer.
func (hu *HtsgetUrl) copyFromHttp(dst io.Writer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", hu.Url, nil)
	if err != nil {
		return err
	}

	for key, value := range hu.Headers {
		req.Header.Add(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != 206 {
		return fmt.Errorf("Bad response from HTSGET service while fetching data: "+
			"HTTP [%d] content-type [%s]", resp.StatusCode, contentType)
	}

	_, err = io.Copy(dst, resp.Body)
	return err
}
