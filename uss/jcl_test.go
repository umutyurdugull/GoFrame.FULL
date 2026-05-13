package uss

import (
	"strings"
	"testing"
)

func TestBPXBATCHJCLTemplateLoaded(t *testing.T) {
	if !strings.Contains(bpxbatchJCLTemplate, "{{COMMAND}}") {
		t.Fatalf("expected command placeholder in embedded template, got %q", bpxbatchJCLTemplate)
	}

	if !strings.Contains(bpxbatchJCLTemplate, "EXEC PGM=BPXBATCH") {
		t.Fatalf("expected BPXBATCH step in embedded template, got %q", bpxbatchJCLTemplate)
	}
}

func TestBuildBPXBATCHJCL(t *testing.T) {
	jcl := buildBPXBATCHJCL("uname -a")

	if !strings.Contains(jcl, "//STEP1    EXEC PGM=BPXBATCH") {
		t.Fatalf("expected BPXBATCH step in JCL, got %q", jcl)
	}

	if !strings.Contains(jcl, "SH uname -a") {
		t.Fatalf("expected command in JCL, got %q", jcl)
	}

	if strings.Contains(jcl, "{{COMMAND}}") {
		t.Fatalf("expected command placeholder to be replaced, got %q", jcl)
	}

	if !strings.Contains(jcl, "//STDOUT   DD SYSOUT=*") {
		t.Fatalf("expected STDOUT DD in JCL, got %q", jcl)
	}

	if !strings.Contains(jcl, "//STDERR   DD SYSOUT=*") {
		t.Fatalf("expected STDERR DD in JCL, got %q", jcl)
	}
}
