package platforminstall

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func writerEnvironmentFixture(launch *MetadataLaunch) []string {
	return []string{"GODEBUG=containermaxprocs=0", "GC_HOME=" + launch.GCHome, "HOME=/nonexistent", "PATH=/usr/bin:/bin", "PWD=" + launch.Evidence}
}

// The pinned Bubblewrap transport adds PWD; the writer must accept exactly that
// five-entry environment, not count-only variants or an environment-supplied cwd.
func TestMetadataWriterEnvironmentExactContract(t *testing.T) {
	launch := &MetadataLaunch{GCHome: "/fixture/home", Evidence: "/fixture/evidence"}
	valid := writerEnvironmentFixture(launch)
	type environmentCase struct {
		name string
		env  []string
		ok   bool
	}
	tests := []environmentCase{
		{"transport-environment", valid, true},
		{"reordered", []string{valid[4], valid[3], valid[2], valid[1], valid[0]}, true},
		{"old-four-entry-environment", valid[:4], false},
		{"extra", append(slices.Clone(valid), "EXTRA=unexpected"), false},
		{"empty", nil, false},
	}
	for i, entry := range valid {
		key, _, _ := strings.Cut(entry, "=")
		missing := append(slices.Clone(valid[:i]), valid[i+1:]...)
		duplicate := slices.Clone(valid)
		duplicate[(i+1)%len(valid)] = entry
		wrong := slices.Clone(valid)
		wrong[i] = key + "=wrong"
		malformed := slices.Clone(valid)
		malformed[i] = key
		tests = append(tests,
			environmentCase{"missing-" + key, missing, false},
			environmentCase{"duplicate-" + key, duplicate, false},
			environmentCase{"wrong-" + key, wrong, false},
			environmentCase{"malformed-" + key, malformed, false},
		)
	}
	unknown := slices.Clone(valid)
	unknown[4] = "UNKNOWN=" + launch.Evidence
	tests = append(tests, environmentCase{"unknown-same-count", unknown, false})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := slices.Clone(tc.env)
			calls := 0
			err := metadataWriterEnvironment(launch, tc.env, func() (string, error) {
				calls++
				return launch.Evidence, nil
			})
			if (err == nil) != tc.ok {
				t.Fatalf("environment acceptance: got %v, want success=%v", err, tc.ok)
			}
			if !tc.ok && !strings.Contains(err.Error(), "writer environment") {
				t.Fatalf("wrong failure boundary: %v", err)
			}
			wantCalls := 0
			if tc.ok {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("cwd reads=%d, want %d", calls, wantCalls)
			}
			if !slices.Equal(before, tc.env) {
				t.Fatal("validator mutated environment input")
			}
		})
	}
}

func TestMetadataWriterEnvironmentWorkingDirectory(t *testing.T) {
	launch := &MetadataLaunch{GCHome: "/fixture/home", Evidence: "/fixture/evidence"}
	readFailure := errors.New("kernel cwd unavailable")
	for _, tc := range []struct {
		name, cwd string
		err       error
	}{
		{"different", "/fixture/other", nil},
		{"empty", "", nil},
		{"relative", "fixture/evidence", nil},
		{"unclean", "/fixture/./evidence", nil},
		{"deleted", "/fixture/evidence (deleted)", nil},
		{"read-failure", launch.Evidence, readFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := metadataWriterEnvironment(launch, writerEnvironmentFixture(launch), func() (string, error) {
				calls++
				return tc.cwd, tc.err
			})
			if err == nil || !strings.Contains(err.Error(), "working directory") {
				t.Fatalf("cwd failure not preserved: %v", err)
			}
			if calls != 1 {
				t.Fatalf("cwd reads=%d, want 1", calls)
			}
			if tc.err != nil && !errors.Is(err, readFailure) {
				t.Fatalf("kernel error lost: %v", err)
			}
		})
	}
}

func TestMetadataWriterWorkingDirectoryIgnoresPWD(t *testing.T) {
	root, spoof := t.TempDir(), t.TempDir()
	t.Chdir(root)
	t.Setenv("PWD", spoof)
	actual, err := metadataWriterWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if actual != root || actual == os.Getenv("PWD") {
		t.Fatalf("cwd observation trusted PWD: got %q", actual)
	}
	launch := &MetadataLaunch{GCHome: "/fixture/home", Evidence: root}
	if err := metadataWriterEnvironment(launch, writerEnvironmentFixture(launch), metadataWriterWorkingDirectory); err != nil {
		t.Fatal(err)
	}
	launch.Evidence = spoof
	if err := metadataWriterEnvironment(launch, writerEnvironmentFixture(launch), metadataWriterWorkingDirectory); err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("spoofed PWD satisfied cwd check: %v", err)
	}
}

func TestMetadataWriterEnvironmentMatchesSandboxArgv(t *testing.T) {
	manifest := metadataContractFixture(t)
	args, err := metadataSandboxArgv(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var environment []string
	var cwd string
	cleared := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--clearenv":
			cleared = true
		case "--setenv":
			if !cleared || i+2 >= len(args) {
				t.Fatal("unbound environment arguments")
			}
			environment = append(environment, args[i+1]+"="+args[i+2])
			i += 2
		case "--chdir":
			if cwd != "" || i+1 >= len(args) {
				t.Fatal("unbound working directory")
			}
			cwd = args[i+1]
			i++
		}
	}
	if !cleared || len(environment) != 4 || cwd != manifest.Metadata.Evidence {
		t.Fatal("launcher contract drift")
	}
	// Source-backed model of pinned Bubblewrap's PWD insertion, not a runtime proof.
	environment = append(environment, "PWD="+cwd)
	if err := metadataWriterEnvironment(manifest.Metadata, environment, func() (string, error) { return cwd, nil }); err != nil {
		t.Fatal(err)
	}
}
