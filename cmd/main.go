package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/umutyurdugull/GoFrame.PROD/core"
	"github.com/umutyurdugull/GoFrame.PROD/datasets"
	"github.com/umutyurdugull/GoFrame.PROD/jobs"
	"github.com/umutyurdugull/GoFrame.PROD/system"
	"github.com/umutyurdugull/GoFrame.PROD/uss"
)

func main() {
	host := requireEnv("GOFRAME_HOST")
	username := requireEnv("GOFRAME_USERNAME")
	password := requireEnv("GOFRAME_PASSWORD")

	auth := core.NewBasicAuth(username, password)
	opts := []core.ClientOption{
		core.WithAuthenticator(auth),
	}
	if insecureTLSRequested() {
		fmt.Println("WARNING: GOFRAME_INSECURE_TLS=true disables TLS certificate verification. Use only for lab or self-signed environments.")
		opts = append(opts, core.WithInsecureTLS())
	}

	client, err := core.NewClient(
		host,
		opts...,
	)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	fmt.Println("========================================")
	fmt.Println("        GOFRAME EXAMPLE")
	fmt.Println("========================================")
	fmt.Printf("base url:  %s\n", client.BaseURL())
	fmt.Printf("principal: %s\n", auth.Principal())
	fmt.Printf("client principal: %s\n", client.Principal())

	runSystemExample(client)
	runJobsExample(client)
	runDatasetsExample(client)
	runUSSExample(client)
}

func runSystemExample(client *core.Client) {
	fmt.Println("\n========================================")
	fmt.Println("       SYSTEM")
	fmt.Println("========================================")

	sysInfo, err := system.GetInfo(client)
	if err != nil {
		fmt.Printf("system info error: %v\n", err)
		return
	}

	fmt.Printf("z/os version: %s\n", sysInfo.ZosVersion)
	fmt.Printf("z/osmf ver:   %s (api: %s)\n", sysInfo.ZosmfVersion, sysInfo.ApiVersion)
	fmt.Printf("hostname:     %s:%s\n", sysInfo.ZosmfHostname, sysInfo.ZosmfPort)
}

func runJobsExample(client *core.Client) {
	fmt.Println("\n========================================")
	fmt.Println("        JOBS")
	fmt.Println("========================================")

	myJobs, err := jobs.List(client)
	if err != nil {
		fmt.Printf("job list error: %v\n", err)
	} else {
		fmt.Printf("listed %d job(s) for %s\n", len(myJobs), client.Principal())
		for _, j := range myJobs {
			fmt.Printf("- %s [%s] status: %s, rc: %s\n", j.JobName, j.JobId, j.Status, j.RetCode)
		}
	}

	cancelJobName := strings.TrimSpace(os.Getenv("GOFRAME_CANCEL_JOB_NAME"))
	cancelJobID := strings.TrimSpace(os.Getenv("GOFRAME_CANCEL_JOB_ID"))
	if cancelJobName != "" || cancelJobID != "" {
		if cancelJobName == "" || cancelJobID == "" {
			fmt.Println("skipping optional cancel example: set both GOFRAME_CANCEL_JOB_NAME and GOFRAME_CANCEL_JOB_ID")
		} else if err := jobs.Cancel(client, cancelJobName, cancelJobID); err != nil {
			fmt.Printf("cancel job error for %s [%s]: %v\n", cancelJobName, cancelJobID, err)
		} else {
			fmt.Printf("requested cancel for job %s [%s]\n", cancelJobName, cancelJobID)
		}
	}

	jcl := sampleJCL()
	jobResp, err := jobs.Submit(client, jcl)
	if err != nil {
		fmt.Printf("sample job submit error: %v\n", err)
		return
	}
	fmt.Printf("submitted sample job %s [%s]\n", jobResp.JobName, jobResp.JobId)

	finalJob, err := jobs.Wait(client, jobResp.JobId, jobResp.JobName)
	if err != nil {
		fmt.Printf("sample job wait error: %v\n", err)
		return
	}
	fmt.Printf("sample job reached status %s\n", finalJob.Status)

	boundedJob, err := jobs.WaitWithOptions(client, finalJob.JobId, finalJob.JobName, jobs.WaitOptions{MaxPolls: 1})
	if err != nil {
		fmt.Printf("bounded wait verification error: %v\n", err)
		return
	}
	fmt.Printf("bounded wait verification status: %s\n", boundedJob.Status)

	output, err := jobs.GetOutput(client, finalJob.JobId, finalJob.JobName)
	if err != nil {
		fmt.Printf("sample job output error: %v\n", err)
		return
	}

	fmt.Println("--- sample job output preview ---")
	fmt.Println(firstLines(output, 12))
	fmt.Printf("abend check: %s\n", jobs.GetAbendReason(output))
	if boolEnv("GOFRAME_PURGE_SAMPLE_JOB") {
		if err := jobs.Purge(client, finalJob.JobName, finalJob.JobId); err != nil {
			fmt.Printf("sample job purge error: %v\n", err)
			return
		}
		fmt.Printf("purged sample job %s [%s]\n", finalJob.JobName, finalJob.JobId)
		return
	}

	fmt.Printf("sample job %s [%s] is intentionally left for spool inspection. Set GOFRAME_PURGE_SAMPLE_JOB=true to purge it automatically.\n", finalJob.JobName, finalJob.JobId)
}

func runDatasetsExample(client *core.Client) {
	fmt.Println("\n========================================")
	fmt.Println("      DATASETS")
	fmt.Println("========================================")

	dsPrefix := client.Principal() + ".GOFRAME"
	testDsName := dsPrefix + ".TEST"

	found, err := datasets.List(client, dsPrefix+".*")
	if err != nil {
		fmt.Printf("dataset list error: %v\n", err)
	} else {
		fmt.Printf("listed %d dataset(s) matching %s.*\n", len(found), dsPrefix)
	}

	allocParams := datasets.AllocateParams{
		Dsorg:     "PS",
		Alcunit:   "TRK",
		Primary:   1,
		Secondary: 1,
		Recfm:     "FB",
		Lrecl:     80,
		Blksize:   800,
	}

	fmt.Printf("allocating dataset: %s\n", testDsName)
	if err := datasets.Allocate(client, testDsName, allocParams); err != nil {
		fmt.Printf("dataset allocate error: %v\n", err)
		return
	}

	content := "HELLO FROM GOFRAME!"
	if err := datasets.Write(client, testDsName, content); err != nil {
		fmt.Printf("dataset write error: %v\n", err)
		return
	}

	readBack, err := datasets.Read(client, testDsName)
	if err != nil {
		fmt.Printf("dataset read error: %v\n", err)
		return
	}

	fmt.Printf("dataset readback: %s\n", strings.TrimSpace(readBack))
	if boolEnv("GOFRAME_DELETE_SAMPLE_DATASET") {
		if err := datasets.Delete(client, testDsName); err != nil {
			fmt.Printf("dataset delete error: %v\n", err)
			return
		}
		fmt.Printf("deleted dataset: %s\n", testDsName)
		return
	}

	fmt.Printf("dataset %s is intentionally left in place for manual inspection on the mainframe. Set GOFRAME_DELETE_SAMPLE_DATASET=true to delete it automatically.\n", testDsName)
}

func runUSSExample(client *core.Client) {
	fmt.Println("\n========================================")
	fmt.Println("        USS")
	fmt.Println("========================================")

	unixCommand := optionalEnv("GOFRAME_USS_COMMAND", "uname -a ; pwd")
	fmt.Printf("executing uss command: %s\n", unixCommand)

	ussOutput, err := uss.ExecuteCmd(client, unixCommand)
	if err != nil {
		fmt.Printf("uss execution error: %v\n", err)
		return
	}

	fmt.Printf("--- command output ---\n%s\n", ussOutput)
}

func sampleJCL() string {
	account := optionalEnv("GOFRAME_JOB_ACCOUNT", "ACCT")
	class := optionalEnv("GOFRAME_JOB_CLASS", "A")
	msgClass := optionalEnv("GOFRAME_JOB_MSGCLASS", "X")

	return fmt.Sprintf("//GOFRAME JOB (%s),'GOFRAME',CLASS=%s,MSGCLASS=%s\n//STEP1   EXEC PGM=IEFBR14\n", account, class, msgClass)
}

func firstLines(value string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) <= maxLines {
		return strings.TrimSpace(value)
	}

	return strings.Join(lines[:maxLines], "\n") + "\n..."
}

func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return value
}

func optionalEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func insecureTLSRequested() bool {
	return boolEnv("GOFRAME_INSECURE_TLS")
}

func boolEnv(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes"
}
