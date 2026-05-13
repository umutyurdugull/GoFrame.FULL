package core

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

type Authenticator interface {
	Apply(req *http.Request) error
}

type principalProvider interface {
	Principal() string
}

type BasicAuth struct {
	username string
	password string
}

func NewBasicAuth(username, password string) BasicAuth {
	return BasicAuth{
		username: username,
		password: password,
	}
}

func (a BasicAuth) Apply(req *http.Request) error {
	req.SetBasicAuth(a.username, a.password)
	return nil
}

func (a BasicAuth) Principal() string {
	return a.username
}

type ClientOption func(*Client) error

func WithAuthenticator(auth Authenticator) ClientOption {
	return func(client *Client) error {
		client.auth = auth
		return nil
	}
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(client *Client) error {
		if httpClient == nil {
			return fmt.Errorf("http client cannot be nil")
		}
		client.httpClient = httpClient
		return nil
	}
}

func WithInsecureTLS() ClientOption {
	return func(client *Client) error {
		client.insecureTLS = true
		return nil
	}
}

type Client struct {
	baseURL        *url.URL
	httpClient     *http.Client
	auth           Authenticator
	defaultHeaders http.Header
	insecureTLS    bool
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("%s %s returned status %d", e.Method, e.URL, e.StatusCode)
	}
	return fmt.Sprintf("%s %s returned status %d: %s", e.Method, e.URL, e.StatusCode, string(e.Body))
}

func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid base url: %q", baseURL)
	}
	normalizedURL := *parsedURL
	normalizedURL.Path = strings.TrimRight(normalizedURL.Path, "/")

	client := &Client{
		baseURL: &normalizedURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		defaultHeaders: http.Header{
			"X-CSRF-ZOSMF-HEADER": []string{"true"},
		},
	}

	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, err
		}
	}

	if client.insecureTLS {
		if err := configureInsecureTLS(client.httpClient); err != nil {
			return nil, err
		}
	}

	return client, nil
}

func MustNewClient(baseURL string, opts ...ClientOption) *Client {
	client, err := NewClient(baseURL, opts...)
	if err != nil {
		panic(err)
	}
	return client
}

func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

func (c *Client) Principal() string {
	if provider, ok := c.auth.(principalProvider); ok {
		return provider.Principal()
	}
	return ""
}

func (c *Client) NewRequest(method, path string, body io.Reader, headers http.Header) (*http.Request, error) {
	requestURL, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		return nil, err
	}

	applyHeaders(req.Header, c.defaultHeaders)
	applyHeaders(req.Header, headers)

	if c.auth != nil {
		if err := c.auth.Apply(req); err != nil {
			return nil, err
		}
	}

	return req, nil
}

func (c *Client) Do(method, path string, body io.Reader, headers http.Header, expectedStatus ...int) (*Response, error) {
	req, err := c.NewRequest(method, path, body, headers)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(expectedStatus) > 0 && !matchesStatus(resp.StatusCode, expectedStatus) {
		return nil, &HTTPError{
			Method:     method,
			URL:        req.URL.String(),
			StatusCode: resp.StatusCode,
			Body:       respBody,
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

func (c *Client) resolveURL(path string) (string, error) {
	if c == nil || c.baseURL == nil {
		return "", fmt.Errorf("client base url is not configured")
	}

	parsedPath, err := url.Parse(path)
	if err != nil {
		return "", err
	}

	return c.baseURL.ResolveReference(parsedPath).String(), nil
}

func applyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func matchesStatus(status int, expected []int) bool {
	for _, candidate := range expected {
		if status == candidate {
			return true
		}
	}
	return false
}

func configureInsecureTLS(httpClient *http.Client) error {
	if httpClient == nil {
		return fmt.Errorf("http client cannot be nil")
	}

	var baseTransport *http.Transport
	switch transport := httpClient.Transport.(type) {
	case nil:
		baseTransport = http.DefaultTransport.(*http.Transport).Clone()
	case *http.Transport:
		baseTransport = transport.Clone()
	default:
		return fmt.Errorf("http client transport must be *http.Transport to configure TLS")
	}

	if baseTransport.TLSClientConfig == nil {
		baseTransport.TLSClientConfig = &tls.Config{}
	} else {
		baseTransport.TLSClientConfig = baseTransport.TLSClientConfig.Clone()
	}
	baseTransport.TLSClientConfig.InsecureSkipVerify = true

	httpClient.Transport = baseTransport
	return nil
}
