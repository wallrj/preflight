package isito

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

const tempFilePrefix = "preflight-test-istio-datagatherer"

func TestNewDataGatherer(t *testing.T) {
	configString := ""
	config := Config{}
	err := yaml.Unmarshal([]byte(configString), &config)
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}
	_, err = config.NewDataGatherer(context.TODO())
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}
}

// TestFetchRealCluster is not used, but has been left here for development use to allow testing against a real cluster.
//func TestFetchRealCluster(t *testing.T) {
//	c := Config{}
//
//	dg, err := c.NewDataGatherer(context.TODO())
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	results, err := dg.Fetch()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	t.Log(results)
//}

// TestFetch
func TestFetch(t *testing.T) {
	// Local server to handle requests made by Kubernetes dynamic data gatherer.
	localServer := createLocalTestServer(t)

	// Parse the URL of the server to generate the kubeconfig file.
	parsedURL, err := url.Parse(localServer.URL)
	if err != nil {
		t.Fatalf("failed to parse test server url %s", localServer.URL)
	}

	// Ensure there is a valid kubeconfig in a temporary file for the dynamic data gatherer.
	kubeConfigPath, err := createKubeConfigWithServer(parsedURL.String())
	if err != nil {
		t.Fatalf("failed to create temp kubeconfig: %s", err)
	}
	defer os.Remove(kubeConfigPath)

	config := Config{}
	err = yaml.Unmarshal([]byte(fmt.Sprintf(configString, kubeConfigPath)), &config)
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}
	dg, err := config.NewDataGatherer(context.TODO())
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}

	results, err := dg.Fetch()
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}

	// TODO: Actually check the results.
	t.Log(results)
}

var configString = `
kubeconfig: %s
`

// createKubeConfigWithServer creates a kubeconfig file on disk with a reference to the local server.
func createKubeConfigWithServer(server string) (string, error) {
	content := fmt.Sprintf(kuebConfigString, server)
	tempFile, err := ioutil.TempFile("", tempFilePrefix)
	if err != nil {
		return "", fmt.Errorf("failed to create a tmpfile for kubeconfig")
	}

	if _, err := tempFile.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("failed to write to tmp kubeconfig file")
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close tmp kubeconfig file after writing")
	}

	return tempFile.Name(), nil
}

var kuebConfigString = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: example
contexts:
- context:
    cluster: example
    namespace: default
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    username: test
    password: test
`

// createLocalTestServer creates a local test server to respond to Kubernetes API requests from the dynamic data
// gatherer.
func createLocalTestServer(t *testing.T) *httptest.Server {
	var localServer *httptest.Server
	localServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var responseContent []byte

		switch r.URL.Path {
		case "/api/v1/namespaces":
			responseContent = []byte(testNamespaces)
		case "/api/v1/pods":
			responseContent = []byte(testPods)
		default:
			responseContent = []byte{}
		}

		w.Write(responseContent)
	}))

	return localServer
}

var testNamespaces = `
apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: Namespace
  metadata:
    name: no-istio-label
- apiVersion: v1
  kind: Namespace
  metadata:
    name: istio-label
    labels:
       istio-injection: enabled
`

var testPods = `
`
