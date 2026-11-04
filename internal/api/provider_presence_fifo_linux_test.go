package api

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedPresenceFIFORefusesWithoutBlocking(t *testing.T) {
	for _, kind := range []string{"manifest", "receipt"} {
		for _, alias := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "-direct", true: "-alias"}[alias], func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				fixture := providerlaunch.New(t, "claude")
				path := platforminstall.DefaultManifestPath(fixture.City)
				if kind == "receipt" {
					path = providerallowlist.ReceiptPath(fixture.City)
					if err := os.Remove(providerallowlist.Path(fixture.City)); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				fifo := path
				if alias {
					fifo = filepath.Join(t.TempDir(), "input")
					if err := os.Symlink(fifo, path); err != nil {
						t.Fatal(err)
					}
				}
				if err := syscall.Mkfifo(fifo, 0o600); err != nil {
					t.Fatal(err)
				}
				result := make(chan error, 1)
				go func() {
					_, err := ProbeCityProvider(context.Background(), fixture.City, "", "claude")
					result <- err
				}()
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("nonregular approval input restored provider execution")
					}
				case <-time.After(time.Second):
					t.Fatal("readiness blocked on approval presence FIFO")
				}
			})
		}
	}
}
