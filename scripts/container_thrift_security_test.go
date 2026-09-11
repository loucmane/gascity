package scripts_test

import (
	"strings"
	"testing"
)

func TestBDRebuildPinsAndAssertsPatchedThrift(t *testing.T) {
	dockerfile := readFile(t, repoRoot(t), "contrib/k8s/Dockerfile.agent")
	for _, want := range []string{
		"ARG THRIFT_VERSION=0.24.0",
		`"github.com/apache/thrift@v${THRIFT_VERSION}"`,
		`go version -m /out/bd | tr '\t' ' ' | grep -Fq "dep github.com/apache/thrift v${THRIFT_VERSION} "`,
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("bd rebuild missing %q", want)
		}
	}
}
