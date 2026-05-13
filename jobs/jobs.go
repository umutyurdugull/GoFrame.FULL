package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/umutyurdugull/GoFrame.PROD/core"
)

const waitPollInterval = 3 * time.Second

type WaitOptions struct {
	PollInterval time.Duration
	MaxPolls     int
}

type Job struct {
	JobId   string `json:"jobid"`
	JobName string `json:"jobname"`
	Owner   string `json:"owner"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	RetCode string `json:"retcode"`
}

type JobResponse struct {
	JobId   string  `json:"jobid"`
	JobName string  `json:"jobname"`
	Status  string  `json:"status"`
	RetCode *string `json:"retcode"`
}

type JobFile struct {
	Id         int    `json:"id"`
	DdName     string `json:"ddname"`
	RecordsUrl string `json:"records-url"`
}

func Submit(client *core.Client, jcl string) (*JobResponse, error) {
	resp, err := client.Do(
		http.MethodPut,
		jobsPath(),
		bytes.NewBufferString(jcl),
		http.Header{"Content-Type": []string{"text/plain"}},
		http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}

	var jobResp JobResponse
	if err := json.Unmarshal(resp.Body, &jobResp); err != nil {
		return nil, fmt.Errorf("decode submitted job response: %w", err)
	}

	return &jobResp, nil
}

func Wait(client *core.Client, jobId, jobName string) (*JobResponse, error) {
	return waitForJob(client, jobId, jobName, waitPollInterval, 0, time.Sleep)
}

func WaitWithOptions(client *core.Client, jobId, jobName string, opts WaitOptions) (*JobResponse, error) {
	pollInterval := opts.PollInterval
	if pollInterval < 0 {
		pollInterval = 0
	}
	if pollInterval == 0 {
		pollInterval = waitPollInterval
	}

	return waitForJob(client, jobId, jobName, pollInterval, opts.MaxPolls, time.Sleep)
}

func GetOutput(client *core.Client, jobId, jobName string) (string, error) {
	files, err := listOutputFiles(client, jobName, jobId)
	if err != nil {
		return "", err
	}

	file, err := selectDefaultOutputFile(jobName, jobId, files)
	if err != nil {
		return "", err
	}

	return readOutputFile(client, jobName, jobId, file.Id)
}

func Cancel(client *core.Client, jobName, jobId string) error {
	_, err := client.Do(
		http.MethodPut,
		jobPath(jobName, jobId),
		bytes.NewBufferString(`{"status":"canceled"}`),
		http.Header{"Content-Type": []string{"application/json"}},
		http.StatusOK,
	)
	return err
}

func Purge(client *core.Client, jobName, jobId string) error {
	_, err := client.Do(
		http.MethodDelete,
		jobPath(jobName, jobId),
		nil,
		nil,
		http.StatusNoContent,
	)
	return err
}

func List(client *core.Client) ([]Job, error) {
	query := url.Values{}
	query.Set("owner", client.Principal())

	path := jobsListPath(query.Encode())
	resp, err := client.Do(http.MethodGet, path, nil, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}

	var jobList []Job
	if err := json.Unmarshal(resp.Body, &jobList); err != nil {
		return nil, fmt.Errorf("decode jobs list response: %w", err)
	}

	return jobList, nil
}

func GetAbendReason(output string) string {
	errorMarkers := []string{"ABEND", "SYSTEM COMPLETION CODE", "S0C4", "S806", "S0C1", "U4038"}

	for _, marker := range errorMarkers {
		if strings.Contains(output, marker) {
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.Contains(line, marker) {
					return strings.TrimSpace(line)
				}
			}
		}
	}
	return "no specific abend detected in common logs"
}

func waitForJob(client *core.Client, jobId, jobName string, pollInterval time.Duration, maxPolls int, sleep func(time.Duration)) (*JobResponse, error) {
	polls := 0

	for {
		jobResp, err := getJob(client, jobName, jobId)
		if err != nil {
			return nil, err
		}

		if normalizeJobStatus(jobResp.Status) == "OUTPUT" {
			return jobResp, nil
		}

		if err := terminalJobError(jobResp, jobName, jobId); err != nil {
			return nil, err
		}

		polls++
		if maxPolls > 0 && polls >= maxPolls {
			resolvedJobName, resolvedJobID := resolveJobIdentity(jobResp, jobName, jobId)
			return nil, fmt.Errorf("job %s [%s] did not reach OUTPUT after %d poll(s); last status: %s", resolvedJobName, resolvedJobID, polls, strings.TrimSpace(jobResp.Status))
		}

		if sleep != nil && pollInterval > 0 {
			sleep(pollInterval)
		}
	}
}

func getJob(client *core.Client, jobName, jobId string) (*JobResponse, error) {
	resp, err := client.Do(http.MethodGet, jobPath(jobName, jobId), nil, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("get job status for %s [%s]: %w", jobName, jobId, err)
	}

	var jobResp JobResponse
	if err := json.Unmarshal(resp.Body, &jobResp); err != nil {
		return nil, fmt.Errorf("decode job status response for %s [%s]: %w", jobName, jobId, err)
	}

	jobResp.JobName, jobResp.JobId = resolveJobIdentity(&jobResp, jobName, jobId)

	return &jobResp, nil
}

func terminalJobError(jobResp *JobResponse, fallbackJobName, fallbackJobID string) error {
	if jobResp == nil {
		return fmt.Errorf("job status response was nil")
	}

	jobName, jobID := resolveJobIdentity(jobResp, fallbackJobName, fallbackJobID)
	status := normalizeJobStatus(jobResp.Status)
	if isTerminalJobFailureStatus(status) {
		if jobResp.RetCode != nil && strings.TrimSpace(*jobResp.RetCode) != "" {
			return fmt.Errorf("job %s [%s] failed with status %s (%s)", jobName, jobID, strings.TrimSpace(jobResp.Status), strings.TrimSpace(*jobResp.RetCode))
		}
		return fmt.Errorf("job %s [%s] failed with status %s", jobName, jobID, strings.TrimSpace(jobResp.Status))
	}

	return nil
}

func listOutputFiles(client *core.Client, jobName, jobId string) ([]JobFile, error) {
	resp, err := client.Do(http.MethodGet, jobFilesPath(jobName, jobId), nil, nil, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("list job output files for %s [%s]: %w", jobName, jobId, err)
	}

	var files []JobFile
	if err := json.Unmarshal(resp.Body, &files); err != nil {
		return nil, fmt.Errorf("decode job output file list for %s [%s]: %w", jobName, jobId, err)
	}

	return files, nil
}

func readOutputFile(client *core.Client, jobName, jobId string, fileID int) (string, error) {
	recordsResp, err := client.Do(http.MethodGet, jobRecordsPath(jobName, jobId, fileID), nil, nil, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("read job output records for %s [%s] file %d: %w", jobName, jobId, fileID, err)
	}

	return string(recordsResp.Body), nil
}

func selectDefaultOutputFile(jobName, jobId string, files []JobFile) (*JobFile, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no output files found for job %s [%s]", jobName, jobId)
	}

	return &files[0], nil
}

func jobsPath() string {
	return "/zosmf/restjobs/jobs"
}

func jobsListPath(encodedQuery string) string {
	if encodedQuery == "" {
		return jobsPath()
	}
	return fmt.Sprintf("%s?%s", jobsPath(), encodedQuery)
}

func jobPath(jobName, jobId string) string {
	return fmt.Sprintf("%s/%s/%s", jobsPath(), jobName, jobId)
}

func jobFilesPath(jobName, jobId string) string {
	return fmt.Sprintf("%s/files", jobPath(jobName, jobId))
}

func jobRecordsPath(jobName, jobId string, fileID int) string {
	return fmt.Sprintf("%s/%d/records", jobFilesPath(jobName, jobId), fileID)
}

func normalizeJobStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

// The current jobs API treats a small curated set of statuses as terminal
// failures. This is intentionally conservative and not a complete z/OSMF
// status taxonomy.
func isTerminalJobFailureStatus(status string) bool {
	switch status {
	case "ABEND", "JCLERR", "JCL ERROR", "CANCELED", "CANCELLED":
		return true
	default:
		return false
	}
}

func resolveJobIdentity(jobResp *JobResponse, fallbackJobName, fallbackJobID string) (string, string) {
	jobName := fallbackJobName
	jobID := fallbackJobID

	if jobResp != nil {
		if strings.TrimSpace(jobResp.JobName) != "" {
			jobName = jobResp.JobName
		}
		if strings.TrimSpace(jobResp.JobId) != "" {
			jobID = jobResp.JobId
		}
	}

	return jobName, jobID
}
