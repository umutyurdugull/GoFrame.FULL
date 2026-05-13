package jobs

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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

func TestSubmit(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.Header.Get("Content-Type"); got != "text/plain" {
			t.Fatalf("unexpected content type: got %q", got)
		}
		if got := req.Header.Get("X-CSRF-ZOSMF-HEADER"); got != "true" {
			t.Fatalf("missing csrf header: got %q", got)
		}
		user, pass, ok := req.BasicAuth()
		if !ok || user != "USER" || pass != "PASS" {
			t.Fatalf("unexpected basic auth credentials: %q/%q ok=%v", user, pass, ok)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		if string(body) != "//JOB ..." {
			t.Fatalf("unexpected request body: got %q", string(body))
		}

		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"ACTIVE"}`)),
		}, nil
	}))

	resp, err := Submit(client, "//JOB ...")
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	if resp.JobId != "JOB123" || resp.JobName != "TESTJOB" {
		t.Fatalf("unexpected job response: %+v", resp)
	}
}

func TestSubmitReturnsDecodeErrorForMalformedResponse(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123"`)),
		}, nil
	}))

	resp, err := Submit(client, "//JOB ...")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "decode submitted job response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}
		if got := req.URL.Query().Get("owner"); got != "USER" {
			t.Fatalf("unexpected owner query: got %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[{"jobid":"JOB123","jobname":"TESTJOB","owner":"USER","status":"OUTPUT","type":"JOB","retcode":"CC 0000"}]`)),
		}, nil
	}))

	jobList, err := List(client)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(jobList) != 1 {
		t.Fatalf("unexpected job count: got %d", len(jobList))
	}
	if jobList[0].Owner != "USER" || jobList[0].JobId != "JOB123" {
		t.Fatalf("unexpected job payload: %+v", jobList[0])
	}
}

func TestListReturnsDecodeErrorForMalformedResponse(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123"`)),
		}, nil
	}))

	jobList, err := List(client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if jobList != nil {
		t.Fatalf("expected nil job list, got %+v", jobList)
	}
	if !strings.Contains(err.Error(), "decode jobs list response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListEncodesOwnerQueryValue(t *testing.T) {
	client, err := core.NewClient(
		"https://zosmf.example.com",
		core.WithAuthenticator(core.NewBasicAuth("USER&OPS", "PASS")),
		core.WithHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs" {
					t.Fatalf("unexpected path: got %s", req.URL.Path)
				}
				if got := req.URL.RawQuery; got != "owner=USER%26OPS" {
					t.Fatalf("unexpected raw query: got %q", got)
				}
				if got := req.URL.Query().Get("owner"); got != "USER&OPS" {
					t.Fatalf("unexpected decoded owner query: got %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[]`)),
				}, nil
			}),
		}),
	)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	jobList, err := List(client)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(jobList) != 0 {
		t.Fatalf("unexpected job count: got %d", len(jobList))
	}
}

func TestGetOutput(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for files request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":7,"ddname":"JESMSGLG"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for records request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files/7/records" {
					t.Fatalf("unexpected records path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("spool output")),
				}, nil
			},
		},
	})

	output, err := GetOutput(client, "JOB123", "TESTJOB")
	if err != nil {
		t.Fatalf("GetOutput returned error: %v", err)
	}

	if output != "spool output" {
		t.Fatalf("unexpected output: got %q", output)
	}
}

func TestWaitReturnsOnImmediateOutputStatus(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"jobid":"JOB123","jobname":"TESTJOB","status":"OUTPUT","retcode":"CC 0000"}`,
			)),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}

	if resp.JobId != "JOB123" || resp.JobName != "TESTJOB" {
		t.Fatalf("unexpected job response: %+v", resp)
	}
	if resp.Status != "OUTPUT" {
		t.Fatalf("unexpected job status: got %q", resp.Status)
	}
	if resp.RetCode == nil || *resp.RetCode != "CC 0000" {
		t.Fatalf("unexpected retcode: %#v", resp.RetCode)
	}
}

func TestWaitTreatsOutputStatusCaseAndWhitespaceInsensitively(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"jobid":"JOB123","jobname":"TESTJOB","status":" output ","retcode":"CC 0000"}`,
			)),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Status != " output " {
		t.Fatalf("unexpected original status preservation: got %q", resp.Status)
	}
}

func TestWaitReturnsErrorForImmediateTerminalFailureStatus(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"jobid":"JOB123","jobname":"TESTJOB","status":"ABEND","retcode":"ABEND S806"}`,
			)),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on failure, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "failed with status ABEND") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "ABEND S806") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestWaitReturnsErrorForKnownTerminalStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "jclerr", status: "JCLERR"},
		{name: "jcl error", status: "JCL ERROR"},
		{name: "canceled", status: "CANCELED"},
		{name: "cancelled", status: "CANCELLED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"jobid":"JOB123","jobname":"TESTJOB","status":"` + tt.status + `"}`,
					)),
				}, nil
			}))

			resp, err := Wait(client, "JOB123", "TESTJOB")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if resp != nil {
				t.Fatalf("expected nil response on failure, got %+v", resp)
			}
			if !strings.Contains(err.Error(), "failed with status "+tt.status) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWaitWithOptionsReturnsBoundedError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"ACTIVE"}`)),
		}, nil
	}))

	resp, err := WaitWithOptions(client, "JOB123", "TESTJOB", WaitOptions{MaxPolls: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "did not reach OUTPUT after 1 poll(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "TESTJOB [JOB123]") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestWaitReturnsDecodeErrorForMalformedJobStatusJSON(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123"`)),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "decode job status response for TESTJOB [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitBackfillsMissingJobIdentityFromRequest(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"OUTPUT","retcode":"CC 0000"}`)),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.JobId != "JOB123" {
		t.Fatalf("unexpected job id: got %q", resp.JobId)
	}
	if resp.JobName != "TESTJOB" {
		t.Fatalf("unexpected job name: got %q", resp.JobName)
	}
}

func TestWaitPropagatesHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("wait failed")),
		}, nil
	}))

	resp, err := Wait(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "get job status for TESTJOB [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitDoesNotPrintToStdout(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"OUTPUT"}`)),
		}, nil
	}))

	output := captureStdout(t, func() {
		resp, err := Wait(client, "JOB123", "TESTJOB")
		if err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
	})

	if output != "" {
		t.Fatalf("expected no stdout output, got %q", output)
	}
}

func TestWaitForJobSupportsBoundedPollingWithoutSleeping(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"ACTIVE"}`)),
		}, nil
	}))

	resp, err := waitForJob(client, "JOB123", "TESTJOB", waitPollInterval, 1, func(time.Duration) {
		t.Fatal("sleep should not be called when bounded polling stops immediately")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "did not reach OUTPUT after 1 poll(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTerminalJobErrorNormalizesStatus(t *testing.T) {
	retCode := "ABEND S806"

	err := terminalJobError(&JobResponse{
		JobId:   "JOB123",
		JobName: "TESTJOB",
		Status:  " canceled ",
		RetCode: &retCode,
	}, "TESTJOB", "JOB123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed with status canceled") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "ABEND S806") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestWaitForJobSupportsMultipleBoundedPollsWithoutSleeping(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"ACTIVE"}`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"jobid":"JOB123","jobname":"TESTJOB","status":"INPUT"}`)),
				}, nil
			},
		},
	})

	sleepCalls := 0
	resp, err := waitForJob(client, "JOB123", "TESTJOB", waitPollInterval, 2, func(time.Duration) {
		sleepCalls++
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if sleepCalls != 1 {
		t.Fatalf("unexpected sleep call count: got %d want 1", sleepCalls)
	}
	if !strings.Contains(err.Error(), "did not reach OUTPUT after 2 poll(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "last status: INPUT") {
		t.Fatalf("unexpected error details: %v", err)
	}
}

func TestIsTerminalJobFailureStatusMatrix(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "abend", status: "ABEND", want: true},
		{name: "jclerr", status: "JCLERR", want: true},
		{name: "jcl error", status: "JCL ERROR", want: true},
		{name: "canceled", status: "CANCELED", want: true},
		{name: "cancelled", status: "CANCELLED", want: true},
		{name: "active", status: "ACTIVE", want: false},
		{name: "input", status: "INPUT", want: false},
		{name: "output", status: "OUTPUT", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminalJobFailureStatus(tt.status); got != tt.want {
				t.Fatalf("unexpected terminal status result: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestGetOutputUsesFirstSpoolFile(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for files request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`[{"id":9,"ddname":"JESJCL"},{"id":7,"ddname":"JESMSGLG"}]`,
					)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for records request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files/9/records" {
					t.Fatalf("unexpected records path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("first spool file output")),
				}, nil
			},
		},
	})

	output, err := GetOutput(client, "JOB123", "TESTJOB")
	if err != nil {
		t.Fatalf("GetOutput returned error: %v", err)
	}

	if output != "first spool file output" {
		t.Fatalf("unexpected output: got %q", output)
	}
}

func TestGetOutputReturnsErrorWhenSpoolListIsEmpty(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[]`)),
		}, nil
	}))

	_, err := GetOutput(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "no output files found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "TESTJOB [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetOutputReturnsMalformedSpoolListError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":7}`)),
		}, nil
	}))

	_, err := GetOutput(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode job output file list for TESTJOB [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetOutputPropagatesSpoolListHTTPError(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("spool list failed")),
		}, nil
	}))

	_, err := GetOutput(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if got := string(httpErr.Body); got != "spool list failed" {
		t.Fatalf("unexpected error body: got %q", got)
	}
	if !strings.Contains(err.Error(), "list job output files for TESTJOB [JOB123]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetOutputPropagatesSpoolContentHTTPError(t *testing.T) {
	client := newTestClient(t, &scriptedTransport{
		t: t,
		handlers: []roundTripFunc{
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for files request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files" {
					t.Fatalf("unexpected files path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`[{"id":7,"ddname":"JESMSGLG"}]`)),
				}, nil
			},
			func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method for records request: got %s", req.Method)
				}
				if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123/files/7/records" {
					t.Fatalf("unexpected records path: got %s", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("spool content failed")),
				}, nil
			},
		},
	})

	_, err := GetOutput(client, "JOB123", "TESTJOB")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var httpErr *core.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected wrapped *core.HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
	}
	if got := string(httpErr.Body); got != "spool content failed" {
		t.Fatalf("unexpected error body: got %q", got)
	}
	if !strings.Contains(err.Error(), "read job output records for TESTJOB [JOB123] file 7") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPurgeSuccess(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("unexpected method: got %s", req.Method)
		}
		if req.URL.Path != "/zosmf/restjobs/jobs/TESTJOB/JOB123" {
			t.Fatalf("unexpected path: got %s", req.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}))

	if err := Purge(client, "TESTJOB", "JOB123"); err != nil {
		t.Fatalf("Purge returned error: %v", err)
	}
}

func TestGetAbendReasonReturnsDefaultWhenNoMarkerExists(t *testing.T) {
	output := "IEF142I STEP1 - STEP WAS EXECUTED - COND CODE 0000\nJOB ENDED NORMALLY"

	got := GetAbendReason(output)

	if got != "no specific abend detected in common logs" {
		t.Fatalf("unexpected abend reason: got %q", got)
	}
}

func TestGetAbendReasonReturnsMatchingMarkerLine(t *testing.T) {
	output := strings.Join([]string{
		"IEF142I STEP1 - STEP WAS EXECUTED - COND CODE 0012",
		"CSV028I ABEND806-04 JOBNAME=TESTJOB STEPNAME=STEP1",
		"IEF450I TESTJOB STEP1 - ABEND=S806 U0000 REASON=00000000",
	}, "\n")

	got := GetAbendReason(output)

	if got != "CSV028I ABEND806-04 JOBNAME=TESTJOB STEPNAME=STEP1" {
		t.Fatalf("unexpected abend reason: got %q", got)
	}
}

func TestCancelAndPurgeReturnErrors(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("cancel failed")),
			}, nil
		}))

		err := Cancel(client, "TESTJOB", "JOB123")
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
	})

	t.Run("purge", func(t *testing.T) {
		client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("unexpected success code")),
			}, nil
		}))

		err := Purge(client, "TESTJOB", "JOB123")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		httpErr, ok := err.(*core.HTTPError)
		if !ok {
			t.Fatalf("expected *core.HTTPError, got %T", err)
		}
		if httpErr.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: got %d", httpErr.StatusCode)
		}
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe returned error: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	captured, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	return string(captured)
}
