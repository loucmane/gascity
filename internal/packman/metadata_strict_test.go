package packman

import (
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestMetadataStrictLocalClosureCannotTreatMissingPackAsEmpty(t *testing.T) {
	root := t.TempDir()
	imports := map[string]config.Import{"missing": {Source: "./missing"}}
	readGit := func(string, ...string) (string, error) {
		t.Fatal("Git must not run for absent local fixture")
		return "", nil
	}
	lock := func(check func() error) error { return check() }
	ordinary, err := checkInstalledWithReaders(root, root, imports, lock, readGit)
	if err != nil {
		t.Fatal(err)
	}
	strict, err := checkInstalledWithReaders(root, root, imports, lock, readGit, true)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.HasIssues() {
		t.Fatal("legacy local behavior changed")
	}
	if !strict.HasIssues() {
		t.Fatal("strict missing local closure accepted")
	}
}
