package platforminstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/shellquote"
)

func TestNativeLaunchRequiresSignedChain(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		for _, fault := range []string{"valid", "unsigned", "helper unsigned", "parser unsigned", "helper bytes", "mode", "symlink"} {
			t.Run(provider+"/"+fault, func(t *testing.T) {
				root := t.TempDir()
				manifest, wrapper := attentionManifestFixture(t, root, provider)
				helper := filepath.Join(filepath.Dir(filepath.Dir(wrapper)), "lib/gct_attention_launch.py")
				switch fault {
				case "unsigned":
					manifest.Integrity.Files = manifest.Integrity.Files[1:]
				case "helper unsigned":
					manifest.Integrity.Files = append(manifest.Integrity.Files[:1], manifest.Integrity.Files[2:]...)
				case "parser unsigned":
					manifest.Integrity.Files = manifest.Integrity.Files[:2]
				case "helper bytes":
					if err := os.Remove(helper); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(helper, []byte("unapproved helper"), 0o444); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(helper, 0o644); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Rename(helper, helper+"-alias"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(helper+"-alias", helper); err != nil {
						t.Fatal(err)
					}
				}
				manifest = finalizeManifest(t, manifest)
				data, err := MarshalManifest(manifest)
				if err != nil {
					t.Fatal(err)
				}
				writeProviderFixture(t, DefaultManifestPath(manifest.CityPath), data)
				command := shellquote.Join([]string{filepath.Join(root, "bin", provider), "resume", "conversation with spaces", "$literal"})
				got, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, strings.Repeat("b", 64), command)
				if fault != "valid" {
					if err == nil {
						t.Fatalf("accepted %s native launcher: %q", fault, got)
					}
					return
				}
				want := shellquote.Join([]string{"/usr/bin/python3", "-I", helper, "native:" + provider, manifest.CityPath, "resume", "conversation with spaces", "$literal"})
				if err != nil || got != want {
					t.Fatalf("native chain = %q, %v; want %q", got, err, want)
				}
				if again, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", got); err != nil || again != got {
					t.Fatalf("canonical native resume = %q, %v", again, err)
				}
				wrongCity := shellquote.Join([]string{"/usr/bin/python3", "-I", helper, "native:" + provider, "/wrong-city"})
				if _, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", wrongCity); err == nil {
					t.Fatal("accepted another city's authority")
				}
			})
		}
	}
}

func TestNativeLaunchRejectsUnsupportedProviderChain(t *testing.T) {
	root := t.TempDir()
	manifest, _ := attentionManifestFixture(t, root, "custom")
	// An ordinary FilePin with a matching suffix is not a supported adapter.
	// Its helper could run before Python validates the provider argument.
	manifest.Integrity.Files = manifest.Integrity.Files[:1]
	manifest = finalizeManifest(t, manifest)
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeProviderFixture(t, DefaultManifestPath(manifest.CityPath), data)
	command := shellquote.Quote(filepath.Join(root, "bin/custom"))
	if got, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", command); err == nil {
		t.Fatalf("unsupported adapter selected unsigned helper: %q", got)
	}
}
