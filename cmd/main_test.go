package main

import (
	"strings"
	"testing"
)

func TestSampleJCLUsesDefaults(t *testing.T) {
	t.Setenv("GOFRAME_JOB_ACCOUNT", "")
	t.Setenv("GOFRAME_JOB_CLASS", "")
	t.Setenv("GOFRAME_JOB_MSGCLASS", "")

	jcl := sampleJCL()

	for _, expected := range []string{
		"//GOFRAME JOB (ACCT),'GOFRAME',CLASS=A,MSGCLASS=X",
		"//STEP1   EXEC PGM=IEFBR14",
	} {
		if !strings.Contains(jcl, expected) {
			t.Fatalf("expected sample JCL to contain %q, got %q", expected, jcl)
		}
	}
}

func TestSampleJCLUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("GOFRAME_JOB_ACCOUNT", "TESTACCT")
	t.Setenv("GOFRAME_JOB_CLASS", "B")
	t.Setenv("GOFRAME_JOB_MSGCLASS", "H")

	jcl := sampleJCL()

	expected := "//GOFRAME JOB (TESTACCT),'GOFRAME',CLASS=B,MSGCLASS=H"
	if !strings.Contains(jcl, expected) {
		t.Fatalf("expected sample JCL to contain %q, got %q", expected, jcl)
	}
}

func TestFirstLines(t *testing.T) {
	got := firstLines("line1\nline2\nline3", 2)
	want := "line1\nline2\n..."
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	got = firstLines("line1\nline2", 2)
	want = "line1\nline2"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOptionalEnv(t *testing.T) {
	t.Setenv("GOFRAME_OPTIONAL_TEST", "")
	if got := optionalEnv("GOFRAME_OPTIONAL_TEST", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}

	t.Setenv("GOFRAME_OPTIONAL_TEST", "configured")
	if got := optionalEnv("GOFRAME_OPTIONAL_TEST", "fallback"); got != "configured" {
		t.Fatalf("expected configured value, got %q", got)
	}
}
