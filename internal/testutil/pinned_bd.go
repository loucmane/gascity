package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"golang.org/x/mod/modfile"
)

const beadsModule = "github.com/steveyegge/beads"

// PinnedBDCommand selects the bd source used by the calling test suite without
// executing it. Only the exact replacement in scripts/beads-fork-pin.json is
// accepted. BuildPinnedBD verifies that checkout before running the command.
func PinnedBDCommand(root, binDir, cgo string, command func(string, ...string) *exec.Cmd) (*exec.Cmd, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	binDir, err = filepath.Abs(binDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	version := ""
	for _, req := range mod.Require {
		if req.Mod.Path == beadsModule {
			version = req.Mod.Version
		}
	}
	if version == "" {
		return nil, fmt.Errorf("%s is not required by go.mod", beadsModule)
	}
	args := []string{"install", "-tags", "gms_pure_go", beadsModule + "/cmd/bd@" + version}
	dir := root
	if len(mod.Replace) != 0 {
		var pin struct {
			Module, Path, Commit, SHA256 string
		}
		data, err := os.ReadFile(filepath.Join(root, "scripts", "beads-fork-pin.json"))
		if err != nil {
			return nil, fmt.Errorf("read approved beads replacement: %w", err)
		}
		if err := json.Unmarshal(data, &pin); err != nil {
			return nil, fmt.Errorf("decode approved beads replacement: %w", err)
		}
		if pin.Module != beadsModule ||
			!regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(pin.Commit) ||
			!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(pin.SHA256) ||
			pin.Path != "../beads-vfug-"+pin.Commit {
			return nil, fmt.Errorf("invalid reviewed beads-fork-pin.json")
		}
		replacement := mod.Replace[0]
		if len(mod.Replace) != 1 || replacement.Old.Path != beadsModule ||
			replacement.Old.Version != "" || replacement.New.Path != pin.Path || replacement.New.Version != "" {
			return nil, fmt.Errorf("only the exact approved beads fork replacement is allowed")
		}
		dir = filepath.Clean(filepath.Join(root, pin.Path))
		args = []string{"build", "-tags", "gms_pure_go", "-o", filepath.Join(binDir, "bd"), "./cmd/bd"}
	}
	cmd := command("go", args...)
	cmd.Dir = dir
	// Preserve the caller's offline proxy, checksum policy and cache. Workspace
	// overrides must not redirect the source selected from this go.mod.
	cmd.Env = append(os.Environ(), "CGO_ENABLED="+cgo, "GOBIN="+binDir, "GOWORK=off")
	return cmd, nil
}

// BuildPinnedBD builds the same beads source as root/go.mod into binDir. A local
// fork must pass the existing commit/content/index gate before compilation.
// The unreplaced path uses go install so bd gets its own dependency closure.
// The caller supplies the process constructor, keeping process ownership in the
// test suite and allowing command recording without executing a build.
func BuildPinnedBD(root, binDir, cgo string, command func(string, ...string) *exec.Cmd) (string, error) {
	cmd, err := PinnedBDCommand(root, binDir, cgo, command)
	if err != nil {
		return "", fmt.Errorf("resolve pinned bd source: %w", err)
	}
	if cmd.Args[1] == "build" {
		gate := command("bash", "scripts/check-gomod-replace.sh", "go.mod")
		gate.Dir = root
		gate.Env = cmd.Env
		if out, err := gate.CombinedOutput(); err != nil {
			return "", fmt.Errorf("verify pinned bd checkout: %w\n%s", err, out)
		}
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create pinned bd directory: %w", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v in %s: %w\n%s", cmd.Args, cmd.Dir, err, out)
	}
	return filepath.Join(binDir, "bd"), nil
}
