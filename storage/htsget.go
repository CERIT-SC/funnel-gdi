package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ohsu-comp-bio/funnel/config"
)

const (
	protocol       = "htsget://"
	protocolBearer = protocol + "bearer:"
	privateKeyFile = ".private.key"
	publicKeyFile  = ".public.key"
)

// HTSGET provides read access to public URLs.
//
// Note that it relies on following programs to be installed and available in
// the system PATH:
//
//   - "htsget" (client implementation of the protocol)
//   - "crypt4gh" (to support "*.c4gh" encrypted resources)
//   - "crypt4gh-keygen" (to generate private and public keys)
//
// For more info about the programs:
//   - https://htsget.readthedocs.io/en/latest/
//   - https://crypt4gh.readthedocs.io/en/latest/
type HTSGET struct {
	conf config.HTSGETStorage
}

// NewHTSGET creates a new HTSGET instance.
func NewHTSGET(conf config.HTSGETStorage) (*HTSGET, error) {
	return &HTSGET{conf: conf}, nil
}

// Join a directory URL with a subpath.
func (b *HTSGET) Join(url, path string) (string, error) {
	return "", nil
}

// Stat returns information about the object at the given storage URL.
func (b *HTSGET) Stat(ctx context.Context, url string) (*Object, error) {
	return nil, nil
}

// List a directory. Calling List on a File is an error.
func (b *HTSGET) List(ctx context.Context, url string) ([]*Object, error) {
	return nil, nil
}

func (b *HTSGET) Put(ctx context.Context, url, path string) (*Object, error) {
	return nil, nil
}

// Get copies a file from a given URL to the host path.
//
// If configuration specifies sending a public key, the received content will
// be also decrypted locally before writing to the file.
func (b *HTSGET) Get(ctx context.Context, url, path string) (*Object, error) {
	htsgetArgs := htsgetArgs(url, b.conf.Protocol, b.conf.SendPublicKey)
	cmd1, cmd2 := htsgetCmds(htsgetArgs, b.conf.SendPublicKey)
	cmdPipe(cmd1, cmd2, path)

	// Check that the destination file exists:
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &Object{
		URL:          url,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		Name:         path,
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
	if !strings.HasPrefix(url, protocol) {
		return &ErrUnsupportedProtocol{"htsgetStorage"}
	}
	return nil
}

func htsgetUrl(url, useProtocol string) (updatedUrl string, token string) {
	if useProtocol == "" {
		useProtocol = "https"
	}
	useProtocol += "://"
	updatedUrl = strings.Replace(url, protocol, useProtocol, 1)

	// Optional info: parse the "token" from "htsget://bearer:token@host..."
	if strings.HasPrefix(url, protocolBearer) {
		bearerStart := len(protocolBearer)
		bearerStop := strings.Index(url, "@")

		if bearerStop > bearerStart {
			updatedUrl = useProtocol + url[bearerStop+1:]
			token = url[bearerStart:bearerStop]
		}
	}

	return
}

func htsgetHeaderJson() string {
	ensureKeyFiles()

	b, err := os.ReadFile(publicKeyFile)
	if err != nil {
		fmt.Println("[ERROR] Could not read", publicKeyFile,
			"file, which should exist:", err)
		panic(1)
	}

	// HTTP headers to be encoded as JSON:
	headers := make(map[string]string)
	headers["client-public-key"] = base64.StdEncoding.EncodeToString(b)

	headersJson, err := json.Marshal(&headers)
	if err != nil {
		fmt.Println("[ERROR] Failed to format JSON-header for passing",
			"client-public-key:", err)
		panic(1)
	}

	return string(headersJson)
}

func htsgetArgs(url, useProtocol string, decrypt bool) []string {
	httpsUrl, token := htsgetUrl(url, useProtocol)
	cmdArgs := make([]string, 0)

	if len(token) > 0 {
		cmdArgs = append(cmdArgs, "--bearer-token", token)
	}

	if decrypt {
		cmdArgs = append(cmdArgs, "--headers", htsgetHeaderJson())
	}

	cmdArgs = append(cmdArgs, httpsUrl)
	return cmdArgs
}

func htsgetCmds(htsgetArgs []string, decrypt bool) (cmd1, cmd2 *exec.Cmd) {
	cmd1 = exec.Command("htsget", htsgetArgs...)

	if decrypt {
		cmd2 = exec.Command("crypt4gh", "decrypt", "--sk", privateKeyFile)
	} else {
		cmd2 = exec.Command("cat")
	}

	return
}

func ensureKeyFiles() {
	files := []string{publicKeyFile, privateKeyFile}
	filesExist := true

	for i := range files {
		if file, err := os.Open(files[i]); err == nil {
			file.Close()
		} else {
			filesExist = false
			break
		}
	}

	if !filesExist {
		err := runCmd("crypt4gh-keygen", "-f", "--nocrypt",
			"--sk", privateKeyFile, "--pk", publicKeyFile)
		if err != nil {
			fmt.Println("Could not generate crypt4gh key-files:", err)
			panic(1)
		} else {
			fmt.Println("[INFO] Generated crypt4gh key-pair.")
		}
	}
}

func runCmd(commandName string, commandArgs ...string) error {
	cmd := exec.Command(commandName, commandArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("Error running command %s: %v\nSTDOUT: %s\nSTDERR: %s",
			commandName, err, stdout.String(), stderr.String())
	}
	return err
}

func cmdFailed(cmd *exec.Cmd, stderr *bytes.Buffer) bool {
	fmt.Println("Waiting for ", cmd.Path)
	if err := cmd.Wait(); err != nil {
		fmt.Printf("[ERROR] `%s` command failed: %v\n", cmd.Path, err)
		if stderr.Len() > 0 {
			fmt.Println("Output from STDERR:")
			fmt.Print(stderr.String())
		}
		return true
	} else {
		fmt.Println("Waiting done ")
		return false
	}
}

func cmdPipe(cmd1, cmd2 *exec.Cmd, destFilePath string) {
	fw, err := os.Create(destFilePath)
	if err != nil {
		fmt.Println("[ERROR] Failed to create file for saving content:", destFilePath, err)
		return
	}
	defer fw.Close()

	// Output from cmd1 goes to cmd2, and output from cmd2 goes to the file.
	stderr1 := new(bytes.Buffer)
	stderr2 := new(bytes.Buffer)
	r, w := io.Pipe()

	cmd1.Stdout = w
	cmd1.Stderr = stderr1

	cmd2.Stdin = r
	cmd2.Stdout = fw
	cmd2.Stderr = stderr2

	if err := cmd1.Start(); err != nil {
		fmt.Printf("[ERROR] failed to run `%s` command: %v", cmd1.Path, err)
		return
	}

	if err := cmd2.Start(); err != nil {
		fmt.Printf("[ERROR] failed to run `%s` command: %v", cmd2.Path, err)
		return
	}

	fmt.Println("cmd1:", cmd1.String())
	fmt.Println("cmd2:", cmd2.String())
	fmt.Println("dest:", destFilePath)

	if cmdFailed(cmd1, stderr1) {
		fw.Close()
		os.Remove(destFilePath)
	}

	w.Close()

	if cmdFailed(cmd2, stderr2) {
		fw.Close()
		os.Remove(destFilePath)
	}

	r.Close()
}
