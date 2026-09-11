package platforminstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataPublicationCleanupClassification(t *testing.T) {
	for _, phase := range []string{"before-rename", "after-rename"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			target, stage := filepath.Join(root, "receipt"), filepath.Join(root, "receipt.stage")
			installer := newInstaller()
			publication, err := installer.prepareMetadataPublication(target, stage, []byte("receipt"), "")
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected close failure")
			failClose := func() error { return errors.Join(injected, publication.parent.Close()) }
			if phase == "before-rename" {
				installer.rename = func(string, string) error { return failClose() }
			} else {
				installer.syncDir = func(string) error { return failClose() }
			}
			committed, err := publication.publish()
			err = metadataCommittedError(err, committed)
			if committed != (phase == "after-rename") || !errors.Is(err, injected) || strings.Contains(err.Error(), "committed_evidence_incomplete") != committed {
				t.Fatalf("phase=%s committed=%v error=%v", phase, committed, err)
			}
			retained := stage
			if committed {
				retained = target
			}
			if data, err := os.ReadFile(retained); err != nil || string(data) != "receipt" {
				t.Fatalf("lost phase evidence: %q %v", data, err)
			}
			if err := publication.close(); err != nil {
				t.Fatalf("duplicate cleanup caused false failure: %v", err)
			}
		})
	}
}

func TestMetadataPublicationCommitBoundary(t *testing.T) {
	for _, failure := range []string{"none", "rename", "directory-sync"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "receipt")
			stage := filepath.Join(root, "receipt.stage")
			installer := newInstaller()
			if failure == "rename" {
				installer.rename = func(string, string) error { return errors.New("rename injected") }
			}
			if failure == "directory-sync" {
				installer.syncDir = func(string) error { return errors.New("sync injected") }
			}
			publication, err := installer.prepareMetadataPublication(target, stage, []byte("new receipt"), "")
			if err != nil {
				t.Fatal(err)
			}
			committed, err := publication.publish()
			if committed != (failure != "rename") || (err != nil) != (failure != "none") {
				t.Fatalf("committed=%v error=%v", committed, err)
			}
			if failure == "rename" {
				if data, err := os.ReadFile(stage); err != nil || string(data) != "new receipt" {
					t.Fatalf("lost stage: %q %v", data, err)
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("target appeared: %v", err)
				}
			} else if data, err := os.ReadFile(target); err != nil || string(data) != "new receipt" {
				t.Fatalf("lost committed image: %q %v", data, err)
			}
			if _, err := publication.publish(); err == nil {
				t.Fatal("publication replay accepted")
			}
		})
	}
}

func TestMetadataPublicationRefusesDriftAndRetainsStage(t *testing.T) {
	for _, change := range []string{"preimage", "stage", "parent", "hardlink"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "metadata")
			if err := os.Mkdir(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			target, stage := filepath.Join(parent, "receipt"), filepath.Join(parent, "receipt.stage")
			if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			publication, err := newInstaller().prepareMetadataPublication(target, stage, []byte("new"), sha256Hex([]byte("old")))
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "preimage":
				err = os.WriteFile(target, []byte("drift"), 0o644)
			case "stage":
				err = os.WriteFile(stage, []byte("drift"), 0o644)
			case "parent":
				err = os.Rename(parent, parent+".retained")
			case "hardlink":
				err = os.Link(target, filepath.Join(root, "alias"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if committed, err := publication.publish(); err == nil || committed {
				t.Fatalf("drift committed=%v err=%v", committed, err)
			}
		})
	}
}

func TestMetadataPublicationRequiresAbsentSameDirectoryStage(t *testing.T) {
	root := t.TempDir()
	for _, stage := range []string{filepath.Join(root, "elsewhere", "stage"), filepath.Join(root, "receipt")} {
		if _, err := newInstaller().prepareMetadataPublication(filepath.Join(root, "receipt"), stage, []byte("new"), ""); err == nil {
			t.Fatal("unsafe stage accepted")
		}
	}
	stage := filepath.Join(root, "retained")
	if err := os.WriteFile(stage, []byte("original failure"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newInstaller().prepareMetadataPublication(filepath.Join(root, "receipt"), stage, []byte("new"), strings.Repeat("0", 64)); err == nil {
		t.Fatal("existing stage accepted")
	}
	data, err := os.ReadFile(stage)
	if err != nil || string(data) != "original failure" {
		t.Fatal("overwrote prior evidence")
	}
}
