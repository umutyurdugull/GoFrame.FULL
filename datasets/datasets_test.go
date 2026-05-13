package datasets

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

func TestRead(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds/USER.TEST.DATA" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("dataset content")),
		}, nil
	}))

	content, err := Read(client, "USER.TEST.DATA")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	if content != "dataset content" {
		t.Fatalf("unexpected content: got %q", content)
	}
}

func TestList(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.URL.Query().Get("dslevel"); got != "USER.TEST.*" {
			t.Fatalf("unexpected dslevel query: got %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"items":[{"dsname":"USER.TEST.DATA","dsorg":"PS","recfm":"FB","volser":"VOL001"},{"dsname":"USER.TEST.PDS","dsorg":"PO","recfm":"VB","volser":"VOL002"}]}`,
			)),
		}, nil
	}))

	items, err := List(client, "USER.TEST.*")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: got %d", len(items))
	}
	if items[0].Name != "USER.TEST.DATA" || items[0].Dsorg != "PS" {
		t.Fatalf("unexpected first dataset: %+v", items[0])
	}
	if items[1].Name != "USER.TEST.PDS" || items[1].VolSer != "VOL002" {
		t.Fatalf("unexpected second dataset: %+v", items[1])
	}
}

func TestListEncodesQueryValue(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.URL.RawQuery; got != "dslevel=USER.TEST.%2A%26SYS%3D1" {
			t.Fatalf("unexpected raw query: got %q", got)
		}
		if got := req.URL.Query().Get("dslevel"); got != "USER.TEST.*&SYS=1" {
			t.Fatalf("unexpected decoded dslevel query: got %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
		}, nil
	}))

	items, err := List(client, "USER.TEST.*&SYS=1")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("unexpected item count: got %d", len(items))
	}
}

func TestWrite(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds/USER.TEST.DATA" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.Header.Get("Content-Type"); got != "text/plain" {
			t.Fatalf("unexpected content type: got %q", got)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		if string(body) != "HELLO" {
			t.Fatalf("unexpected request body: got %q", string(body))
		}

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}))

	if err := Write(client, "USER.TEST.DATA", "HELLO"); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
}

func TestAllocate(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds/USER.TEST.DATA" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: got %q", got)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		want := `{"volser":"VOL001","unit":"3390","dsorg":"PS","alcunit":"TRK","primary":1,"secondary":1,"recfm":"FB","blksize":800,"lrecl":80}`
		if string(body) != want {
			t.Fatalf("unexpected request body: got %q want %q", string(body), want)
		}

		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}))

	err := Allocate(client, "USER.TEST.DATA", AllocateParams{
		Volser:    "VOL001",
		Unit:      "3390",
		Dsorg:     "PS",
		Alcunit:   "TRK",
		Primary:   1,
		Secondary: 1,
		Recfm:     "FB",
		Blksize:   800,
		Lrecl:     80,
	})
	if err != nil {
		t.Fatalf("Allocate returned error: %v", err)
	}
}

func TestDelete(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restfiles/ds/USER.TEST.DATA" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}))

	if err := Delete(client, "USER.TEST.DATA"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}

func TestWriteHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("write failed")),
		}, nil
	}))

	err := Write(client, "USER.TEST.DATA", "HELLO")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	httpErr, ok := err.(*core.HTTPError)
	if !ok {
		t.Fatalf("expected *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
}

func TestReadHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("read failed")),
		}, nil
	}))

	_, err := Read(client, "USER.TEST.DATA")
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

func TestReadEscapesPathSensitiveDatasetName(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if got := req.URL.EscapedPath(); got != "/zosmf/restfiles/ds/USER.TEST.DATA%2FALT" {
			t.Fatalf("unexpected escaped path: got %q", got)
		}
		if got := req.URL.Path; got != "/zosmf/restfiles/ds/USER.TEST.DATA/ALT" {
			t.Fatalf("unexpected decoded path: got %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("dataset content")),
		}, nil
	}))

	content, err := Read(client, "USER.TEST.DATA/ALT")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	if content != "dataset content" {
		t.Fatalf("unexpected content: got %q", content)
	}
}
