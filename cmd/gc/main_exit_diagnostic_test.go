package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

type foreignDiagnosticExitError struct{}

func (foreignDiagnosticExitError) Error() string { return "writer fixture refused" }
func (foreignDiagnosticExitError) ExitCode() int { return 7 }

type diagnosticErrorWriter struct {
	err error
}

func (writer diagnosticErrorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func TestUnhandledForeignExitDiagnostics(t *testing.T) {
	processErr := &exec.ExitError{}
	tests := []struct {
		name    string
		err     error
		surface bool
		code    int
	}{
		{name: "nil", code: 0},
		{name: "ordinary", err: errors.New("diagnostic"), surface: true, code: 1},
		{name: "foreign", err: foreignDiagnosticExitError{}, surface: true, code: 7},
		{name: "wrapped foreign", err: fmt.Errorf("metadata writer: %w", foreignDiagnosticExitError{}), surface: true, code: 7},
		{name: "process type without child", err: processErr, surface: true, code: -1},
		{name: "wrapped process", err: fmt.Errorf("writer wait: %w", processErr), surface: true, code: -1},
		{name: "joined cleanup", err: errors.Join(fmt.Errorf("metadata writer: %w; stderr: fixture refusal", foreignDiagnosticExitError{}), errors.New("close writer pipe: fixture error")), surface: true, code: 7},
		{name: "sentinel", err: errExit, code: 1},
		{name: "wrapped sentinel", err: fmt.Errorf("reported: %w", errExit), code: 1},
		{name: "explicit sentinel", err: exitForCode(2), code: 2},
		{name: "wrapped explicit sentinel", err: fmt.Errorf("reported: %w", exitForCode(2)), code: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldSurfaceUnhandledCommandError(test.err); got != test.surface {
				t.Errorf("surface = %v, want %v", got, test.surface)
			}
			if got := commandExitCode(test.err); got != test.code {
				t.Errorf("exit code = %d, want %d", got, test.code)
			}
		})
	}
}

func TestRunSurfacesForeignExitDiagnostic(t *testing.T) {
	configureIsolatedRuntimeEnv(t)
	tests := []struct {
		name string
		err  error
		code int
		want string
	}{
		{name: "wrapped foreign", err: fmt.Errorf("metadata writer: %w", foreignDiagnosticExitError{}), code: 7, want: "metadata writer: writer fixture refused"},
		{name: "joined cleanup", err: errors.Join(fmt.Errorf("metadata writer: %w", foreignDiagnosticExitError{}), errors.New("close writer pipe: fixture error")), code: 7, want: "close writer pipe: fixture error"},
		{name: "reported sentinel", err: fmt.Errorf("already reported: %w", errExit), code: 1},
		{name: "reported explicit sentinel", err: fmt.Errorf("already reported: %w", exitForCode(2)), code: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runWithRootCommandOptions([]string{"completion", "bash"}, diagnosticErrorWriter{err: test.err}, &stderr, rootCommandOptions{})
			if code != test.code {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, test.code, stderr.String())
			}
			if test.want == "" {
				if stderr.Len() != 0 {
					t.Fatalf("already-reported sentinel duplicated: %q", stderr.String())
				}
			} else if !strings.HasPrefix(stderr.String(), "gc: ") || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("missing diagnostic %q in stderr %q", test.want, stderr.String())
			}
		})
	}
}
