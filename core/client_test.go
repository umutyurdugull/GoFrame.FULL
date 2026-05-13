package core

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestClientNewRequestBuildsURLHeadersAndAuth(t *testing.T) {
	client, err := NewClient("https://example.com/", WithAuthenticator(NewBasicAuth("USER", "PASS")))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	req, err := client.NewRequest(http.MethodGet, "zosmf/restjobs/jobs", nil, http.Header{
		"Content-Type": []string{"application/json"},
		"X-Test":       []string{"value"},
	})
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	if got, want := req.URL.String(), "https://example.com/zosmf/restjobs/jobs"; got != want {
		t.Fatalf("unexpected URL: got %q want %q", got, want)
	}

	if got := req.Header.Get("X-CSRF-ZOSMF-HEADER"); got != "true" {
		t.Fatalf("missing default header, got %q", got)
	}

	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected Content-Type: got %q", got)
	}

	if got := req.Header.Get("X-Test"); got != "value" {
		t.Fatalf("unexpected custom header: got %q", got)
	}

	user, pass, ok := req.BasicAuth()
	if !ok {
		t.Fatal("expected basic auth header to be set")
	}
	if user != "USER" || pass != "PASS" {
		t.Fatalf("unexpected basic auth credentials: got %q/%q", user, pass)
	}
}

func TestClientDoSendsRequestAndReturnsBody(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.URL.String(); got != "https://example.com/zosmf/info" {
				t.Fatalf("unexpected URL: got %q", got)
			}
			if got := req.Header.Get("X-CSRF-ZOSMF-HEADER"); got != "true" {
				t.Fatalf("missing default header: got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}

	client, err := NewClient("https://example.com", WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := client.Do(http.MethodGet, "/zosmf/info", nil, nil, http.StatusOK)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d", resp.StatusCode)
	}

	if got := string(resp.Body); got != `{"ok":true}` {
		t.Fatalf("unexpected body: got %q", got)
	}
}

func TestClientDoMapsHTTPError(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("upstream failed")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	client, err := NewClient("https://example.com", WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = client.Do(http.MethodGet, "/zosmf/restjobs/jobs", nil, nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}

	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}

	if got := string(httpErr.Body); got != "upstream failed" {
		t.Fatalf("unexpected error body: got %q", got)
	}

	if httpErr.Method != http.MethodGet {
		t.Fatalf("unexpected method: got %q", httpErr.Method)
	}

	if httpErr.URL != "https://example.com/zosmf/restjobs/jobs" {
		t.Fatalf("unexpected URL: got %q", httpErr.URL)
	}

	if got := httpErr.Error(); got != "GET https://example.com/zosmf/restjobs/jobs returned status 502: response body omitted (15 byte(s))" {
		t.Fatalf("unexpected error string: got %q", got)
	}
}

func TestHTTPErrorErrorRedactsBodyButPreservesBodyField(t *testing.T) {
	body := []byte("secret=top-secret-token\nrequest body with sensitive details")

	err := &HTTPError{
		Method:     http.MethodPost,
		URL:        "https://example.com/zosmf/restfiles/ds/USER.SECRET",
		StatusCode: http.StatusInternalServerError,
		Body:       body,
	}

	got := err.Error()
	if strings.Contains(got, "top-secret-token") || strings.Contains(got, "request body with sensitive details") {
		t.Fatalf("expected error string to redact body, got %q", got)
	}
	if !strings.Contains(got, "response body omitted") {
		t.Fatalf("expected redaction marker, got %q", got)
	}
	if string(err.Body) != string(body) {
		t.Fatalf("expected raw body to remain available to callers")
	}
}

func TestClientDoMapsHTTPErrorWithoutBody(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	client, err := NewClient("https://example.com", WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = client.Do(http.MethodGet, "/zosmf/info", nil, nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}

	if got := httpErr.Error(); got != "GET https://example.com/zosmf/info returned status 401" {
		t.Fatalf("unexpected error string: got %q", got)
	}
}

func TestNewClientRejectsInvalidBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{
			name:    "relative URL",
			baseURL: "/zosmf",
			wantErr: `invalid base url: "/zosmf"`,
		},
		{
			name:    "parse error",
			baseURL: "://bad",
			wantErr: "parse base url:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.baseURL)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: got %q want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestWithHTTPClientRejectsNil(t *testing.T) {
	_, err := NewClient("https://example.com", WithHTTPClient(nil))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := err.Error(); got != "http client cannot be nil" {
		t.Fatalf("unexpected error: got %q", got)
	}
}

func TestWithInsecureTLSConfiguresTransport(t *testing.T) {
	client, err := NewClient("https://example.com", WithInsecureTLS())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}

	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLS client config to be set")
	}

	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify to be true")
	}
}

func TestWithInsecureTLSRejectsUnsupportedTransport(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, nil
		}),
	}

	_, err := NewClient("https://example.com", WithHTTPClient(httpClient), WithInsecureTLS())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := err.Error(); got != "http client transport must be *http.Transport to configure TLS" {
		t.Fatalf("unexpected error: got %q", got)
	}
}
