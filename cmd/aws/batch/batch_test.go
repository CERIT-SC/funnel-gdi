package batch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeAWS serves the AWS Batch (REST/JSON), EC2 and IAM (query/XML) calls
// made by the batch commands, and keeps the created resources.
type fakeAWS struct {
	mu       sync.Mutex
	roles    map[string]bool
	policies map[string][]string // role -> inline policies
	attached int
	envs     []map[string]any
	queues   []map[string]any
	jobDefs  []map[string]any
	creates  int
}

func newFakeAWS(t *testing.T) *fakeAWS {
	t.Helper()
	f := &fakeAWS{roles: map[string]bool{}, policies: map[string][]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	// The commands use the package configuration and keep flag values
	// between runs; point it at the fake and restore the defaults afterwards.
	conf.Endpoint, conf.Key, conf.Secret = srv.URL, "key", "secret"
	t.Cleanup(func() {
		def := DefaultConfig()
		conf.Endpoint, conf.Key, conf.Secret, conf.Region = "", "", "", ""
		conf.JobDef.Image, conf.JobDef.JobRoleArn = def.JobDef.Image, def.JobDef.JobRoleArn
		force, funnelConfigFile = false, ""
	})
	return f
}

func (f *fakeAWS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		f.batch(w, r)
		return
	}
	r.ParseForm()
	xml := func(action, result string) {
		fmt.Fprintf(w, "<%sResponse><%sResult>%s</%sResult></%sResponse>", action, action, result, action, action)
	}
	role := r.Form.Get("RoleName")
	switch action := r.Form.Get("Action"); action {
	case "DescribeSecurityGroups":
		fmt.Fprint(w, "<DescribeSecurityGroupsResponse><securityGroupInfo><item><groupId>sg-1</groupId></item></securityGroupInfo></DescribeSecurityGroupsResponse>")
	case "DescribeSubnets":
		fmt.Fprint(w, "<DescribeSubnetsResponse><subnetSet><item><subnetId>subnet-1</subnetId></item><item><subnetId>subnet-2</subnetId></item></subnetSet></DescribeSubnetsResponse>")
	case "GetRole":
		if !f.roles[role] {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "<ErrorResponse><Error><Code>NoSuchEntity</Code><Message>no such role</Message></Error></ErrorResponse>")
			return
		}
		xml(action, "<Role><Arn>arn:aws:iam::1:role/"+role+"</Arn></Role>")
	case "CreateRole":
		f.roles[role] = true
		f.creates++
		xml(action, "<Role><Arn>arn:aws:iam::1:role/"+role+"</Arn></Role>")
	case "AttachRolePolicy":
		f.attached++
		xml(action, "")
	case "ListRolePolicies":
		names := ""
		for _, p := range f.policies[role] {
			names += "<member>" + p + "</member>"
		}
		xml(action, "<PolicyNames>"+names+"</PolicyNames>")
	case "PutRolePolicy":
		f.policies[role] = append(f.policies[role], r.Form.Get("PolicyName"))
		xml(action, "")
	default:
		http.Error(w, "unexpected action "+action, http.StatusBadRequest)
	}
}

func (f *fakeAWS) batch(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	reply := func(v map[string]any) { json.NewEncoder(w).Encode(v) }
	switch strings.TrimPrefix(r.URL.Path, "/v1/") {
	case "describecomputeenvironments":
		reply(map[string]any{"computeEnvironments": f.envs})
	case "createcomputeenvironment":
		f.creates++
		f.envs = append(f.envs, in)
		reply(map[string]any{"computeEnvironmentName": in["computeEnvironmentName"]})
	case "describejobqueues":
		reply(map[string]any{"jobQueues": f.queues})
	case "createjobqueue":
		f.creates++
		f.queues = append(f.queues, in)
		reply(map[string]any{"jobQueueName": in["jobQueueName"]})
	case "describejobdefinitions":
		reply(map[string]any{"jobDefinitions": f.jobDefs})
	case "registerjobdefinition":
		if props, _ := in["containerProperties"].(map[string]any); props["image"] == "invalid" {
			w.Header().Set("X-Amzn-Errortype", "ClientException")
			w.WriteHeader(http.StatusBadRequest)
			reply(map[string]any{"message": "invalid image"})
			return
		}
		f.creates++
		in["revision"] = len(f.jobDefs) + 1
		f.jobDefs = append(f.jobDefs, in)
		reply(map[string]any{"jobDefinitionName": in["jobDefinitionName"], "revision": in["revision"]})
	default:
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}
}

func run(args ...string) error {
	Cmd.SetArgs(args)
	return Cmd.Execute()
}

func TestCreateAllResources(t *testing.T) {
	f := newFakeAWS(t)

	if err := run("create-all-resources", "--Region", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	// Compute environment, job queue and job definition, the two service
	// roles and the job role.
	if f.creates != 6 || len(f.envs) != 1 || len(f.queues) != 1 || len(f.jobDefs) != 1 {
		t.Fatalf("created %d resources: %v %v %v", f.creates, f.envs, f.queues, f.jobDefs)
	}
	if f.attached != 2 || len(f.policies["FunnelEcsTaskRole"]) != 2 {
		t.Errorf("attached %d policies, inline policies %v", f.attached, f.policies)
	}
	res := f.envs[0]["computeResources"].(map[string]any)
	if fmt.Sprint(res["securityGroupIds"]) != "[sg-1]" || fmt.Sprint(res["subnets"]) != "[subnet-1 subnet-2]" ||
		res["instanceRole"] != "arn:aws:iam::1:role/ecsInstanceRole" || f.envs[0]["serviceRole"] != "arn:aws:iam::1:role/AWSBatchServiceRole" {
		t.Errorf("compute environment %v", f.envs[0])
	}
	props := f.jobDefs[0]["containerProperties"].(map[string]any)
	cmd := fmt.Sprint(props["command"])
	for _, want := range []string{"--DynamoDB.Region us-east-1", "--taskID Ref::taskID", "--EventWriters dynamodb --EventWriters log"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("job command %s does not contain %q", cmd, want)
		}
	}
	if props["jobRoleArn"] != "arn:aws:iam::1:role/FunnelEcsTaskRole" {
		t.Errorf("job role %v", props["jobRoleArn"])
	}

	// A second run finds the resources and creates nothing.
	if err := run("create-all-resources", "--Region", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if f.creates != 6 {
		t.Errorf("second run created resources: %d", f.creates)
	}
}

func TestCreateJobDefinition(t *testing.T) {
	f := newFakeAWS(t)

	if err := run("create-job-def", "--Region", "us-east-1", "--JobDef.JobRoleArn", "arn:role"); err != nil {
		t.Fatal(err)
	}
	// It exists: no new revision without --force, a new one with it.
	if err := run("create-job-def", "--Region", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if err := run("create-job-def", "--Region", "us-east-1", "--force", "--JobDef.JobRoleArn", ""); err != nil {
		t.Fatal(err)
	}
	if len(f.jobDefs) != 2 {
		t.Fatalf("%d job definitions, want 2", len(f.jobDefs))
	}
	if props := f.jobDefs[0]["containerProperties"].(map[string]any); props["jobRoleArn"] != "arn:role" {
		t.Errorf("job role %v", props["jobRoleArn"])
	}
	// Without a role ARN, the job role and its policies are created.
	if !f.roles["FunnelEcsTaskRole"] || len(f.policies["FunnelEcsTaskRole"]) != 2 {
		t.Errorf("roles %v, policies %v", f.roles, f.policies)
	}
}

func TestCreateWithFunnelConfig(t *testing.T) {
	f := newFakeAWS(t)
	funnel := conf.Funnel
	t.Cleanup(func() { conf.Funnel = funnel })

	// The file replaces the default Funnel configuration, so it needs the
	// DynamoDB section the job command refers to.
	path := filepath.Join(t.TempDir(), "funnel.yml")
	os.WriteFile(path, []byte("Worker:\n  WorkDir: /custom-work-dir\nDynamoDB:\n  TableBasename: custom\n  AWSConfig:\n    Region: eu-west-1\n"), 0o600)
	if err := run("create-all-resources", "--Region", "us-east-1", "--config", path); err != nil {
		t.Fatal(err)
	}
	cmd := fmt.Sprint(f.jobDefs[0]["containerProperties"].(map[string]any)["command"])
	if !strings.Contains(cmd, "--Worker.WorkDir /custom-work-dir") {
		t.Errorf("job command %s", cmd)
	}
}

func TestCreateErrors(t *testing.T) {
	newFakeAWS(t)
	conf.Region = ""
	for _, args := range [][]string{{"create-all-resources"}, {"create-job-def"}} {
		if err := run(args...); err == nil || !strings.Contains(err.Error(), "region") {
			t.Errorf("%v without a region: %v", args, err)
		}
	}
	err := run("create-job-def", "--Region", "us-east-1", "--JobDef.JobRoleArn", "arn:role", "--JobDef.Image", "invalid")
	if err == nil || !strings.Contains(err.Error(), "invalid image") {
		t.Errorf("rejected job definition: %v", err)
	}
	err = run("create-all-resources", "--Region", "us-east-1", "--JobDef.Image", "invalid")
	if err == nil || !strings.Contains(err.Error(), "failed to create JobDefinition") {
		t.Errorf("rejected job definition: %v", err)
	}
}
