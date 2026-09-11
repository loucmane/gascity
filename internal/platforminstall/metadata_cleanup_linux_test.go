package platforminstall

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMetadataCleanupFailureClassification(t *testing.T) {
	for _, committed := range []bool{false, true} {
		for _, resource := range []string{"publication", "lock", "pipe", "session"} {
			t.Run(resource+"/"+map[bool]string{false: "before-rename", true: "after-rename"}[committed], func(t *testing.T) {
				closeErr := errors.New("injected close")
				calls := 0
				closeOnce := metadataCloseOnce(func() error { calls++; return closeErr })
				finish := func() (returnErr error) {
					defer func() { returnErr = metadataCommittedError(returnErr, committed) }()
					defer func() { returnErr = errors.Join(returnErr, closeOnce()) }()
					return nil
				}
				err := finish()
				if !errors.Is(err, closeErr) || strings.Contains(err.Error(), "committed_evidence_incomplete") != committed || calls != 1 {
					t.Fatalf("cleanup classification: %v calls=%d", err, calls)
				}
				if err := closeOnce(); err != nil || calls != 1 {
					t.Fatalf("duplicate close: %v calls=%d", err, calls)
				}
			})
		}
	}
}

func TestMetadataChildCleanupFailureStillReaps(t *testing.T) {
	for _, failure := range []string{"none", "child-end", "request-end", "wait", "terminal"} {
		t.Run(failure, func(t *testing.T) {
			injected := errors.New(failure)
			var events []string
			operation := func(name string) func() error {
				return func() error {
					events = append(events, name)
					if name == failure {
						return injected
					}
					return nil
				}
			}
			childClose := metadataCloseOnce(operation("child-end"))
			requestClose := metadataCloseOnce(operation("request-end"))
			prior := childClose()
			err := metadataFinishChild(prior, requestClose, func() { events = append(events, "cancel") }, operation("wait"), operation("terminal"))
			if (err != nil) != (failure != "none") || failure != "none" && !errors.Is(err, injected) {
				t.Fatalf("lost cleanup failure: %v", err)
			}
			wanted := []string{"child-end", "request-end", "wait", "terminal"}
			if failure == "child-end" || failure == "request-end" {
				wanted = []string{"child-end", "request-end", "cancel", "wait", "terminal"}
			}
			if !reflect.DeepEqual(events, wanted) {
				t.Fatalf("cleanup order %v want %v", events, wanted)
			}
			if err := errors.Join(childClose(), requestClose()); err != nil || !reflect.DeepEqual(events, wanted) {
				t.Fatalf("duplicate resource cleanup: %v %v", err, events)
			}
		})
	}
}

func TestMetadataCleanupClassifiesAtOuterBoundary(t *testing.T) {
	data, err := os.ReadFile("metadata_transaction_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, name := range []string{"MetadataWriterEntrypoint", "RunMetadataTransaction"} {
		function := strings.Index(source, "func "+name)
		if function < 0 {
			t.Fatalf("missing entrypoint %s", name)
		}
		body := source[function:]
		firstDefer := strings.Index(body, "defer func()")
		classification := strings.Index(body, "metadataCommittedError")
		if firstDefer < 0 || classification < firstDefer || classification-firstDefer > 700 {
			t.Fatalf("%s does not classify after outer resource cleanup", name)
		}
	}
	start := strings.Index(source, "if err := command.Start()")
	if start < 0 {
		t.Fatal("missing writer start")
	}
	finalizer := strings.Index(source[start:], "metadataFinishChild")
	closeChild := strings.Index(source[start:], "closeChildInput()")
	if finalizer < 0 || closeChild < 0 || finalizer > closeChild {
		t.Fatal("successful Start can reach child-end cleanup before guaranteed reaping")
	}
}
