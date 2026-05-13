package uss

import (
	_ "embed"
	"strings"
)

//go:embed bpxbatch.jcl
var bpxbatchJCLTemplate string

func buildBPXBATCHJCL(cmd string) string {
	return strings.ReplaceAll(bpxbatchJCLTemplate, "{{COMMAND}}", cmd)
}
