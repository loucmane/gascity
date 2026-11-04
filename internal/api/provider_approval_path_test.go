package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedReadinessRequiresPhysicalApprovalFiles(t *testing.T) {
	for _, kind := range []string{"receipt", "authority"} {
		for _, alias := range []string{"file", "parent"} {
			t.Run(kind+"/"+alias, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				marker := filepath.Join(home, "ran")
				cli := []byte("#!/bin/sh\necho ran > " + marker + "\necho '{\"loggedIn\":true}'\n")
				fixture := providerlaunch.NewWithCLI(t, "claude", cli)
				path := providerallowlist.ReceiptPath(fixture.City)
				if kind == "authority" {
					path = providerallowlist.Path(fixture.City)
				}
				if alias == "parent" {
					path = filepath.Dir(path)
				}
				target := filepath.Join(home, "redirected")
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				result, err := ProbeCityProvider(context.Background(), fixture.City, "", "claude")
				if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
					t.Fatalf("provider executed through %s %s alias: result=%+v error=%v marker=%v", kind, alias, result, err, statErr)
				}
				if err == nil {
					t.Fatalf("accepted redirected approval: %+v", result)
				}
			})
		}
	}
}
