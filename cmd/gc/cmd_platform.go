package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/spf13/cobra"
)

func newPlatformCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage a versioned Gas City platform installation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPlatformInstallCmd(stdout, stderr))
	return cmd
}

func newPlatformInstallCmd(stdout, _ io.Writer) *cobra.Command {
	var manifestPath string
	var dryRun bool
	var apply bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Preflight or atomically apply a digest-pinned platform manifest",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if dryRun == apply {
				return fmt.Errorf("exactly one of --dry-run or --apply is required")
			}
			manifest, err := loadPlatformInstallManifest(manifestPath)
			if err != nil {
				return err
			}
			if dryRun {
				steps, err := platforminstall.Plan(manifest)
				if err != nil {
					return fmt.Errorf("plan platform install: %w", err)
				}
				fmt.Fprintf(stdout, "platform install plan release=%q manifest_sha256=%s\n", manifest.ReleaseID, manifest.ManifestSHA256) //nolint:errcheck // best-effort stdout
				for _, step := range steps {
					mode := "CHECK"
					if step.Mutates {
						mode = "MUTATE"
					}
					fields := []string{fmt.Sprintf("%02d", step.Order), mode, step.Action}
					if step.Path != "" {
						fields = append(fields, "path="+step.Path)
					}
					if step.SHA256 != "" {
						fields = append(fields, "sha256="+step.SHA256)
					}
					fmt.Fprintln(stdout, strings.Join(fields, " ")) //nolint:errcheck // best-effort stdout
				}
				return nil
			}

			receipt, err := platforminstall.Install(manifest)
			if err != nil {
				return fmt.Errorf("apply platform install: %w", err)
			}
			fmt.Fprintf(stdout, "platform install result=%s release=%q manifest_sha256=%s artifact_sha256=%s receipt=%s\n", receipt.Result, receipt.ReleaseID, receipt.ManifestSHA256, receipt.ArtifactSHA256, manifest.ReceiptPath) //nolint:errcheck // best-effort stdout
			return nil
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "absolute path to the signed/digest-pinned install manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run full preflight and print the ordered plan without mutation")
	cmd.Flags().BoolVar(&apply, "apply", false, "atomically apply the manifest (requires separate operator authorization)")
	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}

func loadPlatformInstallManifest(path string) (platforminstall.Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return platforminstall.Manifest{}, fmt.Errorf("inspect platform install manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return platforminstall.Manifest{}, fmt.Errorf("platform install manifest must be a regular file, got mode %s", info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return platforminstall.Manifest{}, fmt.Errorf("read platform install manifest: %w", err)
	}
	manifest, err := platforminstall.LoadManifest(data)
	if err != nil {
		return platforminstall.Manifest{}, fmt.Errorf("load platform install manifest: %w", err)
	}
	return manifest, nil
}
