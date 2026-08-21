package platforminstall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const receiptSchemaV1 = "gc.platform-install-receipt.v1"

type preflight struct {
	candidate      []byte
	previous       []byte
	previousSHA256 string
	manifest       []byte
	noopReceipt    *Receipt
	reuseBackup    bool
	reuseManifest  bool
	recoverReceipt bool
}

func (i *installer) install(manifest Manifest) (Receipt, error) {
	state, err := preflightManifest(manifest)
	if err != nil {
		return Receipt{}, err
	}
	if state.noopReceipt != nil {
		result := *state.noopReceipt
		result.Result = ResultNoop
		if err := finalizeReceipt(&result); err != nil {
			return Receipt{}, err
		}
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(manifest.ReceiptPath), 0o755); err != nil {
		return Receipt{}, fmt.Errorf("create receipt directory: %w", err)
	}
	manifestPath := DefaultManifestPath(manifest.CityPath)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return Receipt{}, fmt.Errorf("create manifest directory: %w", err)
	}
	if !state.recoverReceipt {
		if err := os.MkdirAll(filepath.Dir(manifest.BackupPath), 0o755); err != nil {
			return Receipt{}, fmt.Errorf("create backup directory: %w", err)
		}
		if !state.reuseBackup {
			if err := i.writeAtomic(manifest.BackupPath, state.previous, os.FileMode(manifest.Core.Mode)); err != nil {
				return Receipt{}, fmt.Errorf("write exact prior-binary backup: %w", err)
			}
		}
		if got, err := digestRegularFile(manifest.BackupPath); err != nil {
			return Receipt{}, fmt.Errorf("verify prior-binary backup: %w", err)
		} else if got != state.previousSHA256 {
			return Receipt{}, fmt.Errorf("verify prior-binary backup: sha256 got %s want %s", got, state.previousSHA256)
		}

		if err := i.writeAtomic(manifest.Core.Destination, state.candidate, os.FileMode(manifest.Core.Mode)); err != nil {
			if got, digestErr := digestRegularFile(manifest.Core.Destination); digestErr == nil && got == manifest.Core.SHA256 {
				if rollbackErr := i.rollback(manifest, state.previous, state.previousSHA256); rollbackErr != nil {
					return Receipt{}, fmt.Errorf("publish candidate artifact: %w; rollback also failed: %w", err, rollbackErr)
				}
			}
			return Receipt{}, fmt.Errorf("publish candidate artifact: %w", err)
		}
	}
	receipt := Receipt{
		Schema:         receiptSchemaV1,
		ReleaseID:      manifest.ReleaseID,
		ManifestSHA256: manifest.ManifestSHA256,
		ArtifactSHA256: manifest.Core.SHA256,
		PreviousSHA256: state.previousSHA256,
		Result:         ResultInstalled,
	}
	if err := finalizeReceipt(&receipt); err != nil {
		if rollbackErr := i.rollback(manifest, state.previous, state.previousSHA256); rollbackErr != nil {
			return Receipt{}, fmt.Errorf("finalize install receipt: %w; rollback also failed: %w", err, rollbackErr)
		}
		return Receipt{}, fmt.Errorf("finalize install receipt: %w", err)
	}
	manifestPublished := false
	if !state.reuseManifest {
		if err := i.writeAtomic(manifestPath, state.manifest, 0o644); err != nil {
			if state.recoverReceipt {
				return Receipt{}, fmt.Errorf("write recovered install manifest: %w", err)
			}
			if rollbackErr := i.rollbackAndRemoveMetadata(manifest, state.previous, state.previousSHA256, true); rollbackErr != nil {
				return Receipt{}, fmt.Errorf("write install manifest: %w; rollback also failed: %w", err, rollbackErr)
			}
			return Receipt{}, fmt.Errorf("write install manifest: %w", err)
		}
		manifestPublished = true
	}
	if err := i.writeReceipt(manifest.ReceiptPath, receipt); err != nil {
		if state.recoverReceipt {
			return Receipt{}, fmt.Errorf("write recovered install receipt: %w", err)
		}
		if rollbackErr := i.rollbackAndRemoveMetadata(manifest, state.previous, state.previousSHA256, manifestPublished); rollbackErr != nil {
			return Receipt{}, fmt.Errorf("write install receipt: %w; rollback also failed: %w", err, rollbackErr)
		}
		return Receipt{}, fmt.Errorf("write install receipt: %w", err)
	}
	return receipt, nil
}

func preflightManifest(manifest Manifest) (*preflight, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	want, err := ManifestDigest(manifest)
	if err != nil {
		return nil, err
	}
	if manifest.ManifestSHA256 != want {
		return nil, fmt.Errorf("manifest_sha256 mismatch: got %q want %q", manifest.ManifestSHA256, want)
	}
	manifestBytes, err := MarshalManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical install manifest: %w", err)
	}
	reuseManifest, err := existingRegularFileEquals(DefaultManifestPath(manifest.CityPath), manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("preflight canonical manifest: %w", err)
	}
	candidate, err := readRegularFile(manifest.Core.Source, "candidate artifact")
	if err != nil {
		return nil, err
	}
	if got := sha256Hex(candidate); got != manifest.Core.SHA256 {
		return nil, fmt.Errorf("candidate sha256 mismatch: got %s want %s", got, manifest.Core.SHA256)
	}
	previous, err := readRegularFile(manifest.Core.Destination, "installed artifact")
	if err != nil {
		return nil, err
	}
	previousSHA := sha256Hex(previous)
	if previousSHA != manifest.PreviousSHA256 && previousSHA != manifest.Core.SHA256 {
		return nil, fmt.Errorf("installed artifact sha256 mismatch: got %s want current %s or candidate %s", previousSHA, manifest.PreviousSHA256, manifest.Core.SHA256)
	}

	if previousSHA == manifest.Core.SHA256 {
		if _, err := os.Lstat(manifest.ReceiptPath); errors.Is(err, os.ErrNotExist) {
			backup, backupErr := readRegularFile(manifest.BackupPath, "rollback backup")
			if backupErr != nil {
				return nil, fmt.Errorf("installed artifact matches candidate without a receipt: %w", backupErr)
			}
			if backupSHA := sha256Hex(backup); backupSHA != manifest.PreviousSHA256 {
				return nil, fmt.Errorf("rollback backup sha256 mismatch: got %s want %s", backupSHA, manifest.PreviousSHA256)
			}
			return &preflight{
				candidate:      candidate,
				previous:       backup,
				previousSHA256: sha256Hex(backup),
				manifest:       manifestBytes,
				reuseBackup:    true,
				reuseManifest:  reuseManifest,
				recoverReceipt: true,
			}, nil
		} else if err != nil {
			return nil, fmt.Errorf("inspect install receipt: %w", err)
		}
		receipt, err := loadReceipt(manifest.ReceiptPath)
		if err != nil {
			return nil, fmt.Errorf("installed artifact matches candidate but receipt is not valid: %w", err)
		}
		if receipt.ManifestSHA256 != manifest.ManifestSHA256 || receipt.ArtifactSHA256 != manifest.Core.SHA256 || receipt.PreviousSHA256 != manifest.PreviousSHA256 || receipt.ReleaseID != manifest.ReleaseID {
			return nil, errors.New("installed artifact matches candidate but receipt identifies a different release")
		}
		backupMatches, err := existingRegularFileMatches(manifest.BackupPath, receipt.PreviousSHA256)
		if err != nil {
			return nil, fmt.Errorf("validate no-op backup: %w", err)
		}
		if !backupMatches {
			return nil, errors.New("installed artifact matches candidate but its rollback backup is absent")
		}
		if !reuseManifest {
			return nil, errors.New("installed artifact and receipt match candidate but canonical manifest is absent")
		}
		return &preflight{candidate: candidate, previous: previous, previousSHA256: previousSHA, manifest: manifestBytes, noopReceipt: &receipt, reuseManifest: true}, nil
	}
	backupExists, err := existingRegularFileMatches(manifest.BackupPath, previousSHA)
	if err != nil {
		return nil, fmt.Errorf("preflight backup: %w", err)
	}
	if err := requireAbsentOrRegular(manifest.ReceiptPath, "receipt"); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(manifest.ReceiptPath); err == nil {
		return nil, errors.New("receipt path already exists for a different installed artifact")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect receipt path: %w", err)
	}
	return &preflight{candidate: candidate, previous: previous, previousSHA256: previousSHA, manifest: manifestBytes, reuseBackup: backupExists, reuseManifest: reuseManifest}, nil
}

func existingRegularFileEquals(path string, want []byte) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("path %q is not a regular file", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(got, want) {
		return false, fmt.Errorf("path %q already exists with different canonical bytes", path)
	}
	return true, nil
}

func readRegularFile(path, name string) ([]byte, error) {
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

func existingRegularFileMatches(path, sha string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("path %q is not a regular file", path)
	}
	got, err := digestRegularFile(path)
	if err != nil {
		return false, err
	}
	if got != sha {
		return false, fmt.Errorf("path %q already exists with sha256 %s, want %s", path, got, sha)
	}
	return true, nil
}

func requireAbsentOrRegular(path, name string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s path: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s path must be absent or a regular file, got mode %s", name, info.Mode())
	}
	return nil
}

func digestRegularFile(path string) (string, error) {
	data, err := readRegularFile(path, path)
	if err != nil {
		return "", err
	}
	return sha256Hex(data), nil
}

func (i *installer) writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := requireAbsentOrRegular(path, "atomic destination"); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".stage-*")
	if err != nil {
		return fmt.Errorf("create staged file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	defer cleanup()
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write staged file: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod staged file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("fsync staged file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close staged file: %w", err)
	}
	if err := i.rename(tempPath, path); err != nil {
		return fmt.Errorf("rename staged file: %w", err)
	}
	if err := i.syncDir(dir); err != nil {
		return fmt.Errorf("fsync destination directory: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func (i *installer) rollback(manifest Manifest, previous []byte, wantSHA string) error {
	if err := i.writeAtomic(manifest.Core.Destination, previous, os.FileMode(manifest.Core.Mode)); err != nil {
		return err
	}
	got, err := digestRegularFile(manifest.Core.Destination)
	if err != nil {
		return err
	}
	if got != wantSHA {
		return fmt.Errorf("rollback sha256 got %s want %s", got, wantSHA)
	}
	return nil
}

func (i *installer) rollbackAndRemoveMetadata(manifest Manifest, previous []byte, wantSHA string, removeManifest bool) error {
	if err := i.rollback(manifest, previous, wantSHA); err != nil {
		return err
	}
	if err := i.removeRegularFileIfPresent(manifest.ReceiptPath, "failed receipt"); err != nil {
		return err
	}
	if removeManifest {
		if err := i.removeRegularFileIfPresent(DefaultManifestPath(manifest.CityPath), "failed manifest"); err != nil {
			return err
		}
	}
	return nil
}

func (i *installer) removeRegularFileIfPresent(path, name string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s path has unsafe mode %s", name, info.Mode())
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if err := i.syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("fsync %s directory after removal: %w", name, err)
	}
	return nil
}

func (i *installer) writeReceiptFile(path string, receipt Receipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}
	return i.writeAtomic(path, data, 0o644)
}

func finalizeReceipt(receipt *Receipt) error {
	digest, err := receiptDigest(*receipt)
	if err != nil {
		return err
	}
	receipt.ReceiptSHA256 = digest
	return nil
}

func receiptDigest(receipt Receipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("marshal receipt digest domain: %w", err)
	}
	return sha256Hex(data), nil
}

func loadReceipt(path string) (Receipt, error) {
	data, err := readRegularFile(path, "install receipt")
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Receipt{}, err
	}
	if receipt.Schema != receiptSchemaV1 {
		return Receipt{}, fmt.Errorf("receipt schema %q is unsupported", receipt.Schema)
	}
	want, err := receiptDigest(receipt)
	if err != nil {
		return Receipt{}, err
	}
	if receipt.ReceiptSHA256 != want {
		return Receipt{}, fmt.Errorf("receipt_sha256 mismatch: got %q want %q", receipt.ReceiptSHA256, want)
	}
	if receipt.Result != ResultInstalled {
		return Receipt{}, fmt.Errorf("receipt result %q is not a persisted installed result", receipt.Result)
	}
	return receipt, nil
}
