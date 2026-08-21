package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeBuildUsesCommitDerivedReproducibleInputs(t *testing.T) {
	makefilePath := filepath.Join(repoRoot(t), "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %s: %v", makefilePath, err)
	}
	makefile := string(data)

	for _, required := range []string{
		`COMMIT     := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")`,
		`BUILD_TIME := $(shell git show -s --format=%cI HEAD 2>/dev/null || echo "unknown")`,
		`go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gc`,
	} {
		if !strings.Contains(makefile, required) {
			t.Errorf("Makefile is missing reproducible build contract %q", required)
		}
	}

	if strings.Contains(makefile, `BUILD_TIME := $(shell date -u`) {
		t.Error("Makefile BUILD_TIME still depends on wall-clock time instead of the source commit")
	}
}
