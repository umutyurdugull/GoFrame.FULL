package uss

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/umutyurdugull/GoFrame.PROD/core"
	"github.com/umutyurdugull/GoFrame.PROD/jobs"
)

func ExecuteCmd(client *core.Client, cmd string) (string, error) {
	jobResp, err := submitCommandJob(client, cmd)
	if err != nil {
		return "", err
	}

	if err := waitForCommandJob(client, jobResp); err != nil {
		return "", err
	}

	output, err := collectCommandOutput(client, jobResp)
	if err != nil {
		return "", err
	}

	purgeJob(client, jobResp.JobName, jobResp.JobId)

	return strings.TrimSpace(output), nil
}

func submitCommandJob(client *core.Client, cmd string) (*jobs.JobResponse, error) {
	if err := validateCommand(cmd); err != nil {
		return nil, err
	}

	jcl := buildBPXBATCHJCL(cmd)

	jobResp, err := jobs.Submit(client, jcl)
	if err != nil {
		return nil, fmt.Errorf("submit uss command job: %w", err)
	}

	return jobResp, nil
}

func waitForCommandJob(client *core.Client, jobResp *jobs.JobResponse) error {
	if jobResp == nil {
		return fmt.Errorf("wait for uss command job: missing job response")
	}

	_, err := jobs.Wait(client, jobResp.JobId, jobResp.JobName)
	if err != nil {
		return fmt.Errorf("wait for uss command job %s [%s]: %w", jobResp.JobName, jobResp.JobId, err)
	}

	return nil
}

func collectCommandOutput(client *core.Client, jobResp *jobs.JobResponse) (string, error) {
	if jobResp == nil {
		return "", fmt.Errorf("collect uss command output: missing job response")
	}

	files, err := listSpoolFiles(client, jobResp.JobName, jobResp.JobId)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("collect uss command output for %s [%s]: no spool files found", jobResp.JobName, jobResp.JobId)
	}

	return selectCommandOutput(client, jobResp.JobName, jobResp.JobId, files)
}

func listSpoolFiles(client *core.Client, jobName, jobId string) ([]jobs.JobFile, error) {
	resp, err := client.Do(
		http.MethodGet,
		fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s/files", jobName, jobId),
		nil,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return nil, fmt.Errorf("list uss spool files for %s [%s]: %w", jobName, jobId, err)
	}

	var files []jobs.JobFile
	if err := json.Unmarshal(resp.Body, &files); err != nil {
		return nil, fmt.Errorf("list uss spool files for %s [%s]: decode response: %w", jobName, jobId, err)
	}

	return files, nil
}

func selectCommandOutput(client *core.Client, jobName, jobId string, files []jobs.JobFile) (string, error) {
	var output string
	var hasRealOutput bool

	for _, f := range files {
		if f.DdName == "STDOUT" || f.DdName == "STDERR" {
			content, err := readSpool(client, jobName, jobId, f.Id)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(content) != "" {
				output += fmt.Sprintf("--- %s ---\n%s\n", f.DdName, strings.TrimSpace(content))
				hasRealOutput = true
			}
		}
	}

	if !hasRealOutput && len(files) > 0 {
		fallbackLog, err := readSpool(client, jobName, jobId, files[0].Id)
		if err != nil {
			return "", err
		}
		return "no stdout/stderr found. system log:\n" + strings.TrimSpace(fallbackLog), nil
	}

	return output, nil
}

func readSpool(client *core.Client, jobName, jobId string, fileId int) (string, error) {
	resp, err := client.Do(
		http.MethodGet,
		fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s/files/%d/records", jobName, jobId, fileId),
		nil,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return "", fmt.Errorf("read uss spool file %d for %s [%s]: %w", fileId, jobName, jobId, err)
	}

	cleanContent := strings.ReplaceAll(string(resp.Body), "\x00", "")
	return cleanContent, nil
}

func purgeJob(client *core.Client, jobName, jobId string) {
	_ = jobs.Purge(client, jobName, jobId)
}

func validateCommand(cmd string) error {
	if strings.ContainsRune(cmd, '\n') || strings.ContainsRune(cmd, '\r') || strings.ContainsRune(cmd, '\x00') {
		return fmt.Errorf("submit uss command job: command contains unsupported control characters; use trusted single-line input")
	}

	return nil
}
