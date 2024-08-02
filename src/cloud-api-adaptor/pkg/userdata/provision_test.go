package userdata

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/azure"
)

var testAgentConfig string = `server_addr = 'unix:///run/kata-containers/agent.sock'
guest_components_procs = 'none'
`

var testDaemonConfig string = `{
	"pod-network": {
		"podip": "10.244.0.19/24",
		"pod-hw-addr": "0e:8f:62:f3:81:ad",
		"interface": "eth0",
		"worker-node-ip": "10.224.0.4/16",
		"tunnel-type": "vxlan",
		"routes": [
			{
				"Dst": "",
				"GW": "10.244.0.1",
				"Dev": "eth0"
			}
		],
		"mtu": 1500,
		"index": 1,
		"vxlan-port": 8472,
		"vxlan-id": 555001,
		"dedicated": false
	},
	"pod-namespace": "default",
	"pod-name": "nginx-866fdb5bfb-b98nw",
	"tls-server-key": "-----BEGIN PRIVATE KEY-----\n....\n-----END PRIVATE KEY-----\n",
	"tls-server-cert": "-----BEGIN CERTIFICATE-----\n....\n-----END CERTIFICATE-----\n",
	"tls-client-ca": "-----BEGIN CERTIFICATE-----\n....\n-----END CERTIFICATE-----\n"
}
`

var testAuthJson string = `{
	"auths":{}
}
`

var testCDHConfig string = `socket = 'unix:///run/confidential-containers/cdh.sock'
credentials = []

[kbc]
name = 'cc_kbc'
url = 'http://1.2.3.4:8080'
`

var testInitdataMeta string = `algorithm = 'sha384'
version = '0.1.0'
`

var testAAConfig string = `[token_configs]
[token_configs.coco_as]
url = 'http://127.0.0.1:8080'

[token_configs.kbs]
url = 'http://127.0.0.1:8080'
`

var testPolicyConfig string = `package agent_policy

import future.keywords.in
import future.keywords.every

import input

# Default values, returned by OPA when rules cannot be evaluated to true.
default CopyFileRequest := false
default CreateContainerRequest := false
default CreateSandboxRequest := true
default DestroySandboxRequest := true
default ExecProcessRequest := false
default GetOOMEventRequest := true
default GuestDetailsRequest := true
default OnlineCPUMemRequest := true
default PullImageRequest := true
default ReadStreamRequest := false
default RemoveContainerRequest := true
default RemoveStaleVirtiofsShareMountsRequest := true
default SignalProcessRequest := true
default StartContainerRequest := true
default StatsContainerRequest := true
default TtyWinResizeRequest := true
default UpdateEphemeralMountsRequest := true
default UpdateInterfaceRequest := true
default UpdateRoutesRequest := true
default WaitProcessRequest := true
default WriteStreamRequest := false
`

var testInitdataToml string = testInitdataMeta +
	"\n[data]" +
	"\n\"aa.toml\" = '''\n" +
	testAAConfig +
	"'''" +
	"\n\"cdh.toml\" = '''\n" +
	testCDHConfig +
	"'''" +
	"\n\"policy.repo\" = '''\n" +
	testPolicyConfig +
	"'''"

var testCheckSum = "752e973df66fb381d4d319c49169be0182a9000bc74aa9661a8eeab4f8a674043872f151bfb2cd93cff9b948cf028703"

var testInitdataMarshalled string = `algorithm = 'sha384'
version = '0.1.0'

[data]
'aa.toml' = "[token_configs]\n[token_configs.coco_as]\nurl = 'http://127.0.0.1:8080'\n\n[token_configs.kbs]\nurl = 'http://127.0.0.1:8080'\n"
'cdh.toml' = "socket = 'unix:///run/confidential-containers/cdh.sock'\ncredentials = []\n\n[kbc]\nname = 'cc_kbc'\nurl = 'http://1.2.3.4:8080'\n"
'policy.rego' = "package agent_policy\n\nimport future.keywords.in\nimport future.keywords.every\n\nimport input\n\n# Default values, returned by OPA when rules cannot be evaluated to true.\ndefault CopyFileRequest := false\ndefault CreateContainerRequest := false\ndefault CreateSandboxRequest := true\ndefault DestroySandboxRequest := true\ndefault ExecProcessRequest := false\ndefault GetOOMEventRequest := true\ndefault GuestDetailsRequest := true\ndefault OnlineCPUMemRequest := true\ndefault PullImageRequest := true\ndefault ReadStreamRequest := false\ndefault RemoveContainerRequest := true\ndefault RemoveStaleVirtiofsShareMountsRequest := true\ndefault SignalProcessRequest := true\ndefault StartContainerRequest := true\ndefault StatsContainerRequest := true\ndefault TtyWinResizeRequest := true\ndefault UpdateEphemeralMountsRequest := true\ndefault UpdateInterfaceRequest := true\ndefault UpdateRoutesRequest := true\ndefault WaitProcessRequest := true\ndefault WriteStreamRequest := false\n"
`
// Test server to simulate the metadata service
func startTestServer() *httptest.Server {
	// Create base64 encoded test data
	testUserDataString := base64.StdEncoding.EncodeToString([]byte("test data"))

	// Create a handler function for the desired path /metadata/instance/compute/userData
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Check that the request path is correct
		if r.URL.Path != "/metadata/instance/compute/userData" {
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}

		// Check that the request method is correct
		if r.Method != "GET" {
			http.Error(w, "Method is not supported.", http.StatusNotFound)
			return
		}

		// Write the test data to the response
		if _, err := io.WriteString(w, testUserDataString); err != nil {
			http.Error(w, "Error writing response.", http.StatusNotFound)
		}
	}

	// Create a test server
	srv := httptest.NewServer(http.HandlerFunc(handler))

	fmt.Printf("Started metadata server at srv.URL: %s\n", srv.URL)

	return srv
}

// test server, serving plain text userData
func startTestServerPlainText() *httptest.Server {

	// Create a handler function for the desired path /metadata/instance/compute/userData
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Check that the request path is correct
		if r.URL.Path != "/metadata/instance/compute/userData" {
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}

		// Check that the request method is correct
		if r.Method != "GET" {
			http.Error(w, "Method is not supported.", http.StatusNotFound)
			return
		}

		// Write the test data to the response
		if _, err := io.WriteString(w, "test data"); err != nil {
			http.Error(w, "Error writing response.", http.StatusNotFound)
		}
	}

	// Create a test server
	srv := httptest.NewServer(http.HandlerFunc(handler))

	fmt.Printf("Started metadata server at srv.URL: %s\n", srv.URL)

	return srv

}

// TestGetUserData tests the getUserData function
func TestGetUserData(t *testing.T) {
	// Start a temporary HTTP server for the test simulating
	// the Azure metadata service
	srv := startTestServer()
	defer srv.Close()

	// Create a context
	ctx := context.Background()

	// Send request to srv.URL at path /metadata/instance/compute/userData

	reqPath := srv.URL + "/metadata/instance/compute/userData"
	// Call the getUserData function
	userData, _ := azure.GetUserData(ctx, reqPath)

	// Check that the userData is not empty
	if userData == nil {
		t.Fatalf("getUserData returned empty userData")
	}
}

// TestInvalidGetUserDataInvalidUrl tests the getUserData function with an invalid URL
func TestInvalidGetUserDataInvalidUrl(t *testing.T) {

	// Create a context
	ctx := context.Background()

	// Send request to invalid URL
	reqPath := "invalidURL"
	// Call the getUserData function
	userData, _ := azure.GetUserData(ctx, reqPath)

	// Check that the userData is empty
	if userData != nil {
		t.Fatalf("getUserData returned non-empty userData")
	}
}

// TestInvalidGetUserDataEmptyUrl tests the getUserData function with an empty URL
func TestInvalidGetUserDataEmptyUrl(t *testing.T) {

	// Create a context
	ctx := context.Background()

	// Send request to empty URL
	reqPath := ""
	// Call the getUserData function
	userData, _ := azure.GetUserData(ctx, reqPath)

	// Check that the userData is empty
	if userData != nil {
		t.Fatalf("getUserData returned non-empty userData")
	}
}

type TestProvider struct {
	content  string
	failNext bool
}

func (p *TestProvider) GetUserData(ctx context.Context) ([]byte, error) {
	if p.failNext {
		p.failNext = false
		return []byte("%$#"), nil
	}
	return []byte(p.content), nil
}

func (p *TestProvider) GetRetryDelay() time.Duration {
	return 1 * time.Millisecond
}

// TestRetrieveCloudConfig tests retrieving and parsing of a daemon config
func TestRetrieveCloudConfig(t *testing.T) {
	var provider TestProvider

	provider = TestProvider{content: "write_files: []"}
	_, err := retrieveCloudConfig(context.TODO(), &provider)
	if err != nil {
		t.Fatalf("couldn't retrieve and parse empty cloud config: %v", err)
	}

	provider = TestProvider{failNext: true, content: "write_files: []"}
	_, err = retrieveCloudConfig(context.TODO(), &provider)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	provider = TestProvider{content: `#cloud-config
write_files:
- path: /test
  content: |
    test
    test`}
	_, err = retrieveCloudConfig(context.TODO(), &provider)
	if err != nil {
		t.Fatalf("couldn't retrieve valid cloud config: %v", err)
	}
}

func indentTextBlock(text string, by int) string {
	whiteSpace := strings.Repeat(" ", by)
	split := strings.Split(text, "\n")
	indented := ""
	for _, line := range split {
		indented += whiteSpace + line + "\n"
	}
	return indented
}

// TestProcessCloudConfig tests parsing and provisioning of a daemon config
func TestProcessCloudConfig(t *testing.T) {
	// create temporary agent config file
	tmpAgentConfigFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpAgentConfigFile.Name())

	// create temporary daemon config file
	tmpDaemonConfigFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpDaemonConfigFile.Name())

	// create temporary auth json file
	tmpAuthJsonFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpAuthJsonFile.Name())

	// create temporary cdh config file
	tmpCDHConfigFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpCDHConfigFile.Name())

	content := fmt.Sprintf(`#cloud-config
write_files:
- path: %s
  content: |
%s
- path: %s
  content: |
%s
- path: %s
  content: |
%s
- path: %s
  content: |
%s
`,
		tmpAgentConfigFile.Name(),
		indentTextBlock(testAgentConfig, 4),
		tmpDaemonConfigFile.Name(),
		indentTextBlock(testDaemonConfig, 4),
		tmpCDHConfigFile.Name(),
		indentTextBlock(testCDHConfig, 4),
		tmpAuthJsonFile.Name(),
		indentTextBlock(testAuthJson, 4))

	provider := TestProvider{content: content}

	cc, err := retrieveCloudConfig(context.TODO(), &provider)
	if err != nil {
		t.Fatalf("couldn't retrieve cloud config: %v", err)
	}

	cfg := Config{
		fetchTimeout: 180,
		paths: paths{
			aaConfig:     "",
			agentConfig:  tmpAgentConfigFile.Name(),
			authJson:     tmpAuthJsonFile.Name(),
			daemonConfig: tmpDaemonConfigFile.Name(),
			cdhConfig:    tmpCDHConfigFile.Name(),
		},
		digestPath:   "",
		initdataMeta: "",
		initdataToml: "",
		parentPath:   "",
		staticFiles:  nil,
	}
	if err := processCloudConfig(&cfg, cc); err != nil {
		t.Fatalf("failed to process cloud config file: %v", err)
	}

	// check if files have been written correctly
	data, _ := os.ReadFile(tmpAgentConfigFile.Name())
	fileContent := string(data)
	if fileContent != testAgentConfig {
		t.Fatalf("file content does not match daemon config fixture: got %q", fileContent)
	}

	data, _ = os.ReadFile(tmpDaemonConfigFile.Name())
	fileContent = string(data)
	if fileContent != testDaemonConfig {
		t.Fatalf("file content does not match daemon config fixture: got %q", fileContent)
	}

	data, _ = os.ReadFile(tmpCDHConfigFile.Name())
	fileContent = string(data)
	if fileContent != testCDHConfig {
		t.Fatalf("file content does not match cdh config fixture: got %q", fileContent)
	}

	data, _ = os.ReadFile(tmpAuthJsonFile.Name())
	fileContent = string(data)
	if fileContent != testAuthJson {
		t.Fatalf("file content does not match auth json fixture: got %q", fileContent)
	}
}

func TestProcessWithOptionalEntries(t *testing.T) {
	tmpAgentConfigFile, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpAgentConfigFile.Name())
	tmpDaemonConfigFile, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpDaemonConfigFile.Name())
	tmpAuthJsonFile, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpAuthJsonFile.Name())
	tmpCDHConfigFile, _ := os.CreateTemp("", "test")
	os.Remove(tmpCDHConfigFile.Name())

	content := fmt.Sprintf(`#cloud-config
write_files:
- path: %s
  content: |
%s
- path: %s
  content: |
%s
`,
		tmpAgentConfigFile.Name(),
		indentTextBlock(testAgentConfig, 4),
		tmpDaemonConfigFile.Name(),
		indentTextBlock(testDaemonConfig, 4))
	provider := TestProvider{content: content}

	cc, err := retrieveCloudConfig(context.TODO(), &provider)
	if err != nil {
		t.Fatalf("couldn't retrieve cloud config: %v", err)
	}

	cfg := Config{
		fetchTimeout: 180,
		paths: paths{
			aaConfig:     "",
			agentConfig:  tmpAgentConfigFile.Name(),
			authJson:     tmpAuthJsonFile.Name(),
			daemonConfig: tmpDaemonConfigFile.Name(),
			cdhConfig:    tmpCDHConfigFile.Name(),
		},
		digestPath:   "",
		initdataMeta: "",
		initdataToml: "",
		parentPath:   "",
		staticFiles:  nil,
	}
	if err := processCloudConfig(&cfg, cc); err != nil {
		t.Fatalf("failed to process cloud config file: %v", err)
	}

	_, err = os.Stat(tmpCDHConfigFile.Name())
	if !os.IsNotExist(err) {
		t.Fatalf("CDH config file shouldn't exist")
	}
}

// TestFailPlainTextUserData tests with plain text userData
func TestFailPlainTextUserData(t *testing.T) {
	// startTestServerPlainText
	srv := startTestServerPlainText()
	defer srv.Close()

	// Create a context
	ctx := context.Background()

	// Send request to srv.URL at path /metadata/instance/compute/userData

	reqPath := srv.URL + "/metadata/instance/compute/userData"
	// Call the getUserData function
	userData, _ := azure.GetUserData(ctx, reqPath)

	// Check that the userData is empty. Since plain text userData is not supported
	if userData != nil {
		t.Fatalf("getUserData returned userData")
	}

}

func TestConstructUserData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "userdata")
	if err != nil {
		fmt.Println("Error creating temporary directory:", err)
		return
	}
	defer os.RemoveAll(tempDir)

	aaFilePath := filepath.Join(tempDir, "aa.toml")
	cdhFilePath := filepath.Join(tempDir, "cdh.toml")
	repoFilePath := filepath.Join(tempDir, "policy.rego")

	tmpInitdataMeta, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpInitdataMeta.Name())
	tmpInitdataToml, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpInitdataToml.Name())

	var staticFiles = []string{aaFilePath, cdhFilePath, repoFilePath}
	_ = writeFile(tmpInitdataMeta.Name(), []byte(testInitdataMeta))
	_ = writeFile(aaFilePath, []byte(testAAConfig))
	_ = writeFile(cdhFilePath, []byte(testCDHConfig))
	_ = writeFile(repoFilePath, []byte(testPolicyConfig))

	cfg := Config{
		fetchTimeout: 180,
		paths: paths{
			aaConfig:     aaFilePath,
			agentConfig:  "",
			authJson:     "",
			daemonConfig: "",
			cdhConfig:    cdhFilePath,
		},
		digestPath:   "",
		initdataMeta: tmpInitdataMeta.Name(),
		initdataToml: tmpInitdataToml.Name(),
		parentPath:   "",
		staticFiles:  staticFiles,
	}

	err = constructUserData(&cfg)
	if err != nil {
		t.Fatalf("constructUserData returned err: %v", err)
	}

	bytes, _ := os.ReadFile(tmpInitdataToml.Name())
	content := string(bytes)
	if content != testInitdataMarshalled {
		t.Fatalf("constructUserData returned: %s does not match %s", content, testInitdataToml)
	}
}

func TestCalculateUserDataHash(t *testing.T) {
	tmpInitdataToml, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpInitdataToml.Name())
	tmpCheckSum, _ := os.CreateTemp("", "test")
	defer os.Remove(tmpCheckSum.Name())

	_ = writeFile(tmpInitdataToml.Name(), []byte(testInitdataToml))

	cfg := Config{
		fetchTimeout: 180,
		paths: paths{
			aaConfig:     "",
			agentConfig:  "",
			authJson:     "",
			daemonConfig: "",
			cdhConfig:    "",
		},
		digestPath:   tmpCheckSum.Name(),
		initdataMeta: "",
		initdataToml: tmpInitdataToml.Name(),
		parentPath:   "",
		staticFiles:  nil,
	}

	err := calculateUserDataHash(&cfg)
	if err != nil {
		t.Fatalf("calculateUserDataHash returned err: %v", err)
	}

	bytes, _ := os.ReadFile(tmpCheckSum.Name())
	sum := string(bytes)
	if testCheckSum != sum {
		t.Fatalf("calculateUserDataHash returned: %s does not match %s", sum, testCheckSum)
	}
}
