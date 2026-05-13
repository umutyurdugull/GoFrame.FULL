package uss

import (
	"errors"
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

type scriptedTransport struct {
	t        *testing.T
	handlers []roundTripFunc
	index    int
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.t.Helper()
	if s.index >= len(s.handlers) {
		s.t.Fatalf("unexpected extra request: %s %s", req.Method, req.URL.String())
	}

	handler := s.handlers[s.index]
	s.index++
	return handler(req)
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

func TestExecuteCmdSuccess(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPut {
					t.Fatalf("unexpected submit method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs" {
					t.Fatalf("unexpected submit path: got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll returned error: %v", err)
				}
				if !strings.Contains(string(body), "SH uname -a") {
					t.Fatalf("unexpected JCL body: %q", string(body))
				}

				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected wait method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected wait path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":1,"ddname":"STDOUT"},{"id":2,"ddname":"STDERR"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files/1/records" {
					t.Fatalf("unexpected stdout path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("hello\x00 stdout")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files/2/records" {
					t.Fatalf("unexpected stderr path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("error output")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodDelete {
					t.Fatalf("unexpected purge method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected purge path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err != nil {
		t.Fatalf("ExecuteCmd returned error: %v", err)
	}

	want := "--- STDOUT ---\nhello stdout\n--- STDERR ---\nerror output"
	if output != want {
		t.Fatalf("unexpected output: got %q want %q", output, want)
	}
}

func TestExecuteCmdSuccessIncludesStdoutStderrAndStripsNUL(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":1,"ddname":"STDOUT"},{"id":2,"ddname":"STDERR"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("\x00hello stdout\x00")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("error output")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err != nil {
		t.Fatalf("ExecuteCmd returned error: %v", err)
	}
	if !strings.Contains(output, "--- STDOUT ---") {
		t.Fatalf("missing stdout section: %q", output)
	}
	if !strings.Contains(output, "--- STDERR ---") {
		t.Fatalf("missing stderr section: %q", output)
	}
	if strings.Contains(output, "\x00") {
		t.Fatalf("expected NUL bytes to be stripped, got %q", output)
	}
	if strings.HasSuffix(output, "\n") {
		t.Fatalf("expected final output to be trimmed, got %q", output)
	}
}

func TestExecuteCmdSubmitHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("submit failed")),
		}, nil
	}))

	_, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "submit uss command job") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "returned status 500") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestExecuteCmdRejectsCommandWithNewline(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	output, err := ExecuteCmd(client, "uname -a\nwhoami")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}
	if !strings.Contains(err.Error(), "unsupported control characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCmdRejectsCommandWithNUL(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	output, err := ExecuteCmd(client, "uname -a\x00")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}
	if !strings.Contains(err.Error(), "unsupported control characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCmdWaitErrorIncludesJobContext(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected wait path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ABEND","retcode":"ABEND S806"}`)),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}
	if !strings.Contains(err.Error(), "wait for uss command job USSCMD [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "failed with status ABEND") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestExecuteCmdIgnoresPurgeError(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":1,"ddname":"STDOUT"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("purge failed")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err != nil {
		t.Fatalf("ExecuteCmd returned error: %v", err)
	}

	want := "--- STDOUT ---\nok"
	if output != want {
		t.Fatalf("unexpected output: got %q want %q", output, want)
	}
}

func TestExecuteCmdFallsBackToFirstFileWhenStdoutAndStderrAreAbsent(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPut {
					t.Fatalf("unexpected submit method: got %s", req.Method)
				}

				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected wait method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected wait path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":5,"ddname":"JESMSGLG"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files/5/records" {
					t.Fatalf("unexpected fallback records path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("fallback system log")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodDelete {
					t.Fatalf("unexpected purge method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected purge path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err != nil {
		t.Fatalf("ExecuteCmd returned error: %v", err)
	}

	want := "no stdout/stderr found. system log:\nfallback system log"
	if output != want {
		t.Fatalf("unexpected output: got %q want %q", output, want)
	}
}

func TestExecuteCmdReturnsErrorWhenSpoolListIsMalformed(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"id":1,"ddname":"STDOUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodDelete {
					t.Fatalf("unexpected purge method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123" {
					t.Fatalf("unexpected purge path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}

	if !strings.Contains(err.Error(), "list uss spool files for USSCMD [JOB123]") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestExecuteCmdReturnsErrorWhenSpoolListIsEmpty(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[]`)),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}
	if !strings.Contains(err.Error(), "collect uss command output for USSCMD [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "no spool files found") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestExecuteCmdReturnsErrorWhenSpoolListRequestFails(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("spool list failed")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "list uss spool files for USSCMD [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCmdReturnsErrorWhenStdoutReadFails(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":1,"ddname":"STDOUT"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files/1/records" {
					t.Fatalf("unexpected stdout path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("stdout read failed")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "read uss spool file 1 for USSCMD [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCmdReturnsErrorWhenFallbackReadFails(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"USSCMD","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":5,"ddname":"JESMSGLG"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/zosmf/restjobs/jobs/USSCMD/JOB123/files/5/records" {
					t.Fatalf("unexpected fallback records path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("fallback read failed")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Fatalf("unexpected output: got %q want empty string", output)
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "read uss spool file 5 for USSCMD [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCmdEscapesReturnedJobIdentityInSpoolAndPurgePaths(t *testing.T) {
	escapedBase := "/zosmf/restjobs/jobs/USS%2FCMD%3FA%23B%20C%252F/JOB%2F123%3FA%23B%20C%252F"

	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB/123?A#B C%2F","jobname":"USS/CMD?A#B C%2F","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if got := req.URL.EscapedPath(); got != escapedBase {
					t.Fatalf("unexpected wait escaped path: got %q want %q", got, escapedBase)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB/123?A#B C%2F","jobname":"USS/CMD?A#B C%2F","status":"OUTPUT"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				want := escapedBase + "/files"
				if got := req.URL.EscapedPath(); got != want {
					t.Fatalf("unexpected files escaped path: got %q want %q", got, want)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":1,"ddname":"STDOUT"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				want := escapedBase + "/files/1/records"
				if got := req.URL.EscapedPath(); got != want {
					t.Fatalf("unexpected records escaped path: got %q want %q", got, want)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if got := req.URL.EscapedPath(); got != escapedBase {
					t.Fatalf("unexpected purge escaped path: got %q want %q", got, escapedBase)
				}

				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			},
		},
	})

	output, err := ExecuteCmd(client, "uname -a")
	if err != nil {
		t.Fatalf("ExecuteCmd returned error: %v", err)
	}
	if output != "--- STDOUT ---\nok" {
		t.Fatalf("unexpected output: got %q", output)
	}
}
