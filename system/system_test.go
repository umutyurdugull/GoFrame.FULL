package system

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/umutyurdugull/GoFrame.PROD/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(t *testing.T, transport http.RoundTripper) *core.Client {
	t.Helper()

	client, err := core.NewClient(
		"https://zosmf.example.com",
		core.WithAuthenticator(core.NewBasicAuth("USER", "PASS")),
		core.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	return client
}

func TestGetInfo(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/info" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"zosmf_version":"3.1","zos_version":"2.5","zosmf_hostname":"mf.example.com","zosmf_port":"443","api_version":"1.0"}`,
			)),
		}, nil
	}))

	info, err := GetInfo(client)
	if err != nil {
		t.Fatalf("GetInfo returned error: %v", err)
	}

	if info.ZosmfVersion != "3.1" || info.ZosVersion != "2.5" {
		t.Fatalf("unexpected info payload: %+v", info)
	}
}

func TestGetInfoHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("upstream failure")),
		}, nil
	}))

	_, err := GetInfo(client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	httpErr, ok := err.(*core.HTTPError)
	if !ok {
		t.Fatalf("expected *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
}
