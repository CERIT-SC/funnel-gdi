package storage

import (
	// gs "cloud.google.com/go/storage"
	// "context"
	"flag"
	// "fmt"
	"github.com/ohsu-comp-bio/funnel/proto/tes"
	"github.com/ohsu-comp-bio/funnel/storage"
	"github.com/ohsu-comp-bio/funnel/tests"
	"golang.org/x/net/context"
	"io/ioutil"
	"testing"
)

var projectID string

func init() {
	flag.StringVar(&projectID, "projectID", projectID, "Google project ID")
	flag.Parse()
}

func TestGoogleStorage(t *testing.T) {
	tests.SetLogOutput(log, t)

	if !conf.Worker.Storage.GS.Valid() {
		t.Skipf("Skipping google storage e2e tests...")
	}

	testBucket := "funnel-e2e-tests-" + tests.RandomString(6)

	// cli, err := newGsTest()
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// err = cli.createBucket(projectID, testBucket)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer func() {
	// 	cli.deleteBucket(testBucket)
	// }()

	protocol := "gs://"

	store, err := storage.Storage{}.WithConfig(conf.Worker.Storage)
	if err != nil {
		t.Fatal("error configuring storage:", err)
	}

	fPath := "testdata/test_in"
	inFileURL := protocol + testBucket + "/" + fPath
	_, err = store.Put(context.Background(), inFileURL, fPath, tes.FileType_FILE)
	if err != nil {
		t.Fatal("error uploading test file:", err)
	}

	dPath := "testdata/test_dir"
	inDirURL := protocol + testBucket + "/" + dPath
	_, err = store.Put(context.Background(), inDirURL, dPath, tes.FileType_DIRECTORY)
	if err != nil {
		t.Fatal("error uploading test directory:", err)
	}

	outFileURL := protocol + testBucket + "/" + "test-output-file.txt"
	outDirURL := protocol + testBucket + "/" + "test-output-directory"

	task := &tes.Task{
		Name: "gs e2e",
		Inputs: []*tes.Input{
			{
				Url:  inFileURL,
				Path: "/opt/inputs/test-file.txt",
				Type: tes.FileType_FILE,
			},
			{
				Url:  inDirURL,
				Path: "/opt/inputs/test-directory",
				Type: tes.FileType_DIRECTORY,
			},
		},
		Outputs: []*tes.Output{
			{
				Path: "/opt/workdir/test-output-file.txt",
				Url:  outFileURL,
				Type: tes.FileType_FILE,
			},
			{
				Path: "/opt/workdir/test-output-directory",
				Url:  outDirURL,
				Type: tes.FileType_DIRECTORY,
			},
		},
		Executors: []*tes.Executor{
			{
				Image: "alpine:latest",
				Command: []string{
					"sh",
					"-c",
					"cat $(find /opt/inputs -type f) > test-output-file.txt; mkdir test-output-directory; cp *.txt test-output-directory/",
				},
				Workdir: "/opt/workdir",
			},
		},
	}

	resp, err := fun.RPC.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}

	taskFinal := fun.Wait(resp.Id)

	if taskFinal.State != tes.State_COMPLETE {
		t.Fatal("Unexpected task failure")
	}

	expected := "file1 content\nfile2 content\nhello\n"

	err = store.Get(context.Background(), outFileURL, "./test_tmp/test-gs-file.txt", tes.FileType_FILE)
	if err != nil {
		t.Fatal("Failed to download file:", err)
	}

	b, err := ioutil.ReadFile("./test_tmp/test-gs-file.txt")
	if err != nil {
		t.Fatal("Failed to read downloaded file:", err)
	}
	actual := string(b)

	if actual != expected {
		t.Log("expected:", expected)
		t.Log("actual:  ", actual)
		t.Fatal("unexpected content")
	}

	err = store.Get(context.Background(), outDirURL, "./test_tmp/test-gs-directory", tes.FileType_DIRECTORY)
	if err != nil {
		t.Fatal("Failed to download directory:", err)
	}

	b, err = ioutil.ReadFile("./test_tmp/test-gs-directory/test-output-file.txt")
	if err != nil {
		t.Fatal("Failed to read file in downloaded directory", err)
	}
	actual = string(b)

	if actual != expected {
		t.Log("expected:", expected)
		t.Log("actual:  ", actual)
		t.Fatal("unexpected content")
	}
}

// type gsTest struct {
// 	client *gs.Client
// }

// func newGsTest() (*gsTest, error) {
// 	client, err := gs.NewClient(context.Background())
// 	return &gsTest{client}, err
// }

// func (g *gsTest) createBucket(projectID, bucket string) error {
// 	cli := g.client.Bucket(bucket)
// 	return cli.Create(context.Background(), projectID, nil)
// }

// func (g *gsTest) deleteBucket(bucket string) error {
// 	// emptyBucket(g.bucket)
// 	// g.Buckets.Delete(g.bucket)
// 	return nil
// }