package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/spf13/cobra"
)

var platformLifecycleFactory = func() platforminstall.Lifecycle {
	return newPlatformSupervisorLifecycle()
}

func newPlatformCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage a versioned Gas City platform installation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newPlatformInstallCmd(stdout, stderr),
		newPlatformManifestCmd(stdout, stderr),
		newPlatformRollbackCmd(stdout, stderr),
	)
	return cmd
}

func newPlatformManifestCmd(stdout, _ io.Writer) *cobra.Command {
	var inputPath string
	var outputPath string
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Finalize an unsigned platform manifest with its canonical digest",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !filepath.IsAbs(inputPath) {
				return fmt.Errorf("platform manifest input must be an absolute path: %q", inputPath)
			}
			if !filepath.IsAbs(outputPath) {
				return fmt.Errorf("platform manifest output must be an absolute path: %q", outputPath)
			}
			input, err := readRegularPlatformFile(inputPath, "unsigned platform manifest")
			if err != nil {
				return err
			}
			manifest, canonical, err := platforminstall.FinalizeManifest(input)
			if err != nil {
				return fmt.Errorf("finalize platform manifest: %w", err)
			}
			wrote, err := writeCanonicalPlatformManifest(outputPath, canonical)
			if err != nil {
				return err
			}
			result := "noop"
			if wrote {
				result = "written"
			}
			fmt.Fprintf(stdout, "platform manifest result=%s release=%q manifest_sha256=%s output=%s\n", result, manifest.ReleaseID, manifest.ManifestSHA256, outputPath) //nolint:errcheck // best-effort stdout
			return nil
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "absolute path to an unsigned manifest with an empty manifest_sha256")
	cmd.Flags().StringVar(&outputPath, "output", "", "absolute path for the canonical finalized manifest")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")
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
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			receipt, err := platforminstall.Apply(cmd.Context(), manifest, platformLifecycleFactory())
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

func newPlatformRollbackCmd(stdout, _ io.Writer) *cobra.Command {
	var manifestPath string
	var dryRun bool
	var apply bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Preflight or restore the manifest-pinned previous platform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun == apply {
				return fmt.Errorf("exactly one of --dry-run or --apply is required")
			}
			manifest, err := loadPlatformInstallManifest(manifestPath)
			if err != nil {
				return err
			}
			if dryRun {
				steps, err := platforminstall.RollbackPlan(manifest)
				if err != nil {
					return fmt.Errorf("plan platform rollback: %w", err)
				}
				fmt.Fprintf(stdout, "platform rollback plan release=%q manifest_sha256=%s\n", manifest.ReleaseID, manifest.ManifestSHA256) //nolint:errcheck // best-effort stdout
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

			proof, err := platforminstall.Revert(cmd.Context(), manifest, platformLifecycleFactory())
			if err != nil {
				return fmt.Errorf("apply platform rollback: %w", err)
			}
			fmt.Fprintf(stdout, "platform rollback result=restored release=%q manifest_sha256=%s artifact_sha256=%s commit=%s version=%q\n", manifest.ReleaseID, manifest.ManifestSHA256, proof.ExecutableSHA256, proof.Commit, proof.Version) //nolint:errcheck // best-effort stdout
			return nil
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "absolute path to the signed/digest-pinned install manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run full rollback preflight and print the ordered plan without mutation")
	cmd.Flags().BoolVar(&apply, "apply", false, "restore and verify the previous runtime (requires separate operator authorization)")
	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}

func loadPlatformInstallManifest(path string) (platforminstall.Manifest, error) {
	data, err := readRegularPlatformFile(path, "platform install manifest")
	if err != nil {
		return platforminstall.Manifest{}, err
	}
	manifest, err := platforminstall.LoadManifest(data)
	if err != nil {
		return platforminstall.Manifest{}, fmt.Errorf("load platform install manifest: %w", err)
	}
	return manifest, nil
}

func readRegularPlatformFile(path, name string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file, got mode %s", name, info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

func writeCanonicalPlatformManifest(path string, data []byte) (bool, error) {
	match, exists, err := platformManifestOutputMatches(path, data)
	if err != nil {
		return false, err
	}
	if exists {
		if match {
			return false, nil
		}
		return false, fmt.Errorf("platform manifest output already exists with different bytes: %s", path)
	}

	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return false, fmt.Errorf("inspect platform manifest output directory: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("platform manifest output parent must be a directory: %s", dir)
	}
	temp, err := os.CreateTemp(dir, ".platform-manifest-*.stage")
	if err != nil {
		return false, fmt.Errorf("create staged platform manifest: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return false, fmt.Errorf("chmod staged platform manifest: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return false, fmt.Errorf("write staged platform manifest: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return false, fmt.Errorf("fsync staged platform manifest: %w", err)
	}
	if err := temp.Close(); err != nil {
		return false, fmt.Errorf("close staged platform manifest: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			match, _, matchErr := platformManifestOutputMatches(path, data)
			if matchErr != nil {
				return false, matchErr
			}
			if match {
				return false, nil
			}
			return false, fmt.Errorf("platform manifest output already exists with different bytes: %s", path)
		}
		return false, fmt.Errorf("publish platform manifest without replacement: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return false, fmt.Errorf("remove staged platform manifest link: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return false, fmt.Errorf("open platform manifest output directory: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return false, fmt.Errorf("fsync platform manifest output directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return false, fmt.Errorf("close platform manifest output directory: %w", err)
	}
	return true, nil
}

func platformManifestOutputMatches(path string, data []byte) (match bool, exists bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect platform manifest output: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, true, fmt.Errorf("platform manifest output must be a regular file, got mode %s", info.Mode())
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, true, fmt.Errorf("read platform manifest output: %w", err)
	}
	return bytes.Equal(existing, data), true, nil
}
