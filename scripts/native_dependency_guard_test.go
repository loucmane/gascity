package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeDependencyGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the native dependency shell guard runs on Unix")
	}
	root := repoRoot(t)
	reviewed := readFile(t, root, "scripts/testdata/native-dependencies/grpc-1.83.2.txt")
	if got := len(strings.FieldsFunc(reviewed, func(r rune) bool { return r == '\n' })); got != 730 {
		t.Fatalf("reviewed graph has %d modules, want 730", got)
	}
	type fixture struct {
		name, modules, symbol, literal, help, size, want string
		helpFails, beforeBuild                           bool
	}
	cases := []fixture{
		{name: "reviewed_graph", modules: reviewed},
		{name: "unrelated_growth", modules: reviewed + "example.invalid/unrelated v1.0.0\n", want: "module graph has 731 modules; max is 730", beforeBuild: true},
	}
	// Each family probe stays at 730 total: it must reach the family guard,
	// not accidentally pass because the total-count check rejected it first.
	for _, family := range []struct {
		name, prefix string
		limit        int
		want         string
	}{
		{"aws", "github.com/aws/aws-sdk-go-v2/probe", 25, "AWS SDK module count 26 exceeds 25"},
		{"azure", "github.com/Azure/azure-sdk-for-go/probe", 9, "Azure SDK module count 10 exceeds 9"},
		{"dolthub", "github.com/dolthub/probe", 15, "DoltHub module count 16 exceeds 15"},
	} {
		cases = append(cases, fixture{name: family.name + "_at_limit", modules: nativeGuardModules(family.prefix, family.limit, 1, 0)})
		cases = append(cases, fixture{name: family.name + "_over_limit", modules: nativeGuardModules(family.prefix, family.limit+1, 1, 0), want: family.want, beforeBuild: true})
	}
	cases = append(cases,
		fixture{name: "google_at_limit", modules: nativeGuardModules("", 0, 1, 1)},
		fixture{name: "google_over_limit", modules: nativeGuardModules("", 0, 1, 2), want: "google.golang.org/api count 2 exceeds 1", beforeBuild: true},
		fixture{name: "missing_beads", modules: nativeGuardModules("", 0, 0, 0), want: "expected exactly one github.com/steveyegge/beads module, got 0", beforeBuild: true},
		fixture{name: "duplicate_beads", modules: nativeGuardModules("", 0, 2, 0), want: "expected exactly one github.com/steveyegge/beads module, got 2", beforeBuild: true},
		fixture{name: "binary_at_limit", modules: reviewed, size: "270000000"},
		fixture{name: "binary_over_limit", modules: reviewed, size: "270000001", want: "gc binary is 270000001 bytes; max is 270000000"},
		fixture{name: "help_failure", modules: reviewed, helpFails: true, want: "normal gc metrics --help failed"},
		fixture{name: "help_testhook", modules: reviewed, help: "__testhook-record-help", want: "metrics --help exposes the product-metrics testhook command"},
	)
	for _, symbol := range []string{"main.runProductMetricsTesthookChild", "main.newProductMetricsTesthookRecordHelpCommand", "internal/productmetrics.OpenTesthook", "internal/productmetrics.testhookLoopbackHost"} {
		cases = append(cases, fixture{name: "symbol_" + symbol, modules: reviewed, symbol: symbol, want: "testhook symbol " + symbol})
	}
	for _, literal := range []string{"GC_PRODUCT_METRICS_TESTHOOK_ENDPOINT", "GC_PRODUCT_METRICS_TESTHOOK_CA_FILE", "__testhook-record-help"} {
		cases = append(cases, fixture{name: "literal_" + literal, modules: reviewed, literal: literal, want: "testhook literal " + literal})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("modules", tc.modules, 0o600)
			// go is a closed fixture protocol. No real build or installed gc
			// executes; only the production guard and this inert shell file run.
			write("go", `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GC_GUARD_FIXTURE_DIR/calls"
case "$*" in
  "list -m all") cat "$GC_GUARD_FIXTURE_DIR/modules" ;;
  "build -o "*" ./cmd/gc")
    test "$#" = 4
    cp "$GC_GUARD_FIXTURE_DIR/fake-gc" "$3"
    chmod 0755 "$3" ;;
  "tool nm "*) printf '%s\n' "$GC_GUARD_FIXTURE_SYMBOL" ;;
  *) exit 97 ;;
esac
`, 0o700)
			write("fake-gc", "#!/usr/bin/env bash\n# "+tc.literal+"\n"+
				"test \"$*\" = 'metrics --help' || exit 98\n"+
				"test \"$GC_GUARD_FIXTURE_HELP_FAILS\" = false || exit 1\n"+
				"printf '%s\\n' \"$GC_GUARD_FIXTURE_HELP\"\n", 0o700)
			realWC, err := exec.LookPath("wc")
			if err != nil {
				t.Fatal(err)
			}
			write("wc", `#!/usr/bin/env bash
if [ "$*" = "-c" ] && [ -n "$GC_GUARD_FIXTURE_SIZE" ]; then
  printf '%s\n' "$GC_GUARD_FIXTURE_SIZE"
else
  exec "$GC_GUARD_FIXTURE_WC" "$@"
fi
`, 0o700)
			cmd := exec.Command("bash", filepath.Join(root, "scripts/check-native-dependency-surface.sh"))
			cmd.Dir = dir
			for _, entry := range os.Environ() {
				if strings.HasPrefix(entry, "GC_NATIVE_DEP_") || strings.HasPrefix(entry, "GC_GUARD_FIXTURE_") || strings.HasPrefix(entry, "PATH=") || strings.HasPrefix(entry, "TMPDIR=") {
					continue
				}
				cmd.Env = append(cmd.Env, entry)
			}
			cmd.Env = append(cmd.Env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+dir,
				"GC_GUARD_FIXTURE_DIR="+dir, "GC_GUARD_FIXTURE_SYMBOL="+tc.symbol,
				"GC_GUARD_FIXTURE_HELP="+tc.help, fmt.Sprintf("GC_GUARD_FIXTURE_HELP_FAILS=%t", tc.helpFails),
				"GC_GUARD_FIXTURE_SIZE="+tc.size, "GC_GUARD_FIXTURE_WC="+realWC)
			out, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil || !strings.Contains(string(out), "native dependency guard: modules=730 ") {
					t.Fatalf("guard should pass: %v\n%s", err, out)
				}
			} else if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("expected refusal %q: %v\n%s", tc.want, err, out)
			}
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			built := strings.Contains(string(calls), "build -o ")
			if built == tc.beforeBuild {
				t.Fatalf("build reached=%t, want %t; calls:\n%s", built, !tc.beforeBuild, calls)
			}
		})
	}
}

// Synthetic family counts are deliberate (including malformed duplicate module
// output): the real guard must enforce its independent limits on what Go returns.
func nativeGuardModules(prefix string, count, beads, google int) string {
	lines := []string{"example.invalid/fixture"}
	for i := 0; i < beads; i++ {
		lines = append(lines, "github.com/steveyegge/beads v1.2.2")
	}
	for i := 0; i < google; i++ {
		lines = append(lines, "google.golang.org/api v0.264.0")
	}
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf("%s%d v1.0.0", prefix, i))
	}
	for i := len(lines); i < 730; i++ {
		lines = append(lines, fmt.Sprintf("example.invalid/neutral%d v1.0.0", i))
	}
	return strings.Join(lines, "\n") + "\n"
}
