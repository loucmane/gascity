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
	candidate            []byte
	previous             []byte
	previousSHA256       string
	manifest             []byte
	managedFiles         []managedFilePreflight
	noopReceipt          *Receipt
	reuseBackup          bool
	reuseManifest        bool
	coreAlreadyInstalled bool
	corePublished        bool
}

type managedFilePreflight struct {
	file             ManagedFile
	candidate        []byte
	previous         []byte
	previousPresent  bool
	reuseBackup      bool
	alreadyInstalled bool
	published        bool
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

	manifestPath := DefaultManifestPath(manifest.CityPath)
	directories := []string{
		filepath.Dir(manifest.ReceiptPath),
		filepath.Dir(manifestPath),
		filepath.Dir(manifest.BackupPath),
		filepath.Dir(manifest.Core.Destination),
	}
	for _, file := range state.managedFiles {
		directories = append(directories, filepath.Dir(file.file.Destination))
		if file.previousPresent {
			directories = append(directories, filepath.Dir(file.file.BackupPath))
		}
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return Receipt{}, fmt.Errorf("create install directory %q: %w", directory, err)
		}
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
	for index := range state.managedFiles {
		file := &state.managedFiles[index]
		if !file.previousPresent || file.reuseBackup {
			continue
		}
		if err := i.writeAtomic(file.file.BackupPath, file.previous, os.FileMode(file.file.Mode)); err != nil {
			return Receipt{}, fmt.Errorf("write exact managed-file backup %q: %w", file.file.Name, err)
		}
		file.reuseBackup = true
	}

	if !state.coreAlreadyInstalled {
		if err := i.writeAtomic(manifest.Core.Destination, state.candidate, os.FileMode(manifest.Core.Mode)); err != nil {
			if got, digestErr := digestRegularFile(manifest.Core.Destination); digestErr == nil && got == manifest.Core.SHA256 {
				state.corePublished = true
				if rollbackErr := i.rollbackTransaction(manifest, state); rollbackErr != nil {
					return Receipt{}, fmt.Errorf("publish candidate artifact: %w; rollback also failed: %w", err, rollbackErr)
				}
			}
			return Receipt{}, fmt.Errorf("publish candidate artifact: %w", err)
		}
		state.corePublished = true
	}
	for index := range state.managedFiles {
		file := &state.managedFiles[index]
		if file.alreadyInstalled {
			continue
		}
		if err := i.writeAtomic(file.file.Destination, file.candidate, os.FileMode(file.file.Mode)); err != nil {
			if got, digestErr := digestRegularFile(file.file.Destination); digestErr == nil && got == file.file.SHA256 {
				file.published = true
			}
			if rollbackErr := i.rollbackTransaction(manifest, state); rollbackErr != nil {
				return Receipt{}, fmt.Errorf("publish managed file %q: %w; rollback also failed: %w", file.file.Name, err, rollbackErr)
			}
			return Receipt{}, fmt.Errorf("publish managed file %q: %w", file.file.Name, err)
		}
		file.published = true
	}
	receipt := Receipt{
		Schema:         receiptSchemaV1,
		ReleaseID:      manifest.ReleaseID,
		ManifestSHA256: manifest.ManifestSHA256,
		ArtifactSHA256: manifest.Core.SHA256,
		PreviousSHA256: state.previousSHA256,
		ManagedFiles:   receiptManagedFiles(manifest.ManagedFiles),
		Result:         ResultInstalled,
	}
	if err := finalizeReceipt(&receipt); err != nil {
		if rollbackErr := i.rollbackTransaction(manifest, state); rollbackErr != nil {
			return Receipt{}, fmt.Errorf("finalize install receipt: %w; rollback also failed: %w", err, rollbackErr)
		}
		return Receipt{}, fmt.Errorf("finalize install receipt: %w", err)
	}
	manifestPublished := false
	if !state.reuseManifest {
		if err := i.writeAtomic(manifestPath, state.manifest, 0o644); err != nil {
			if rollbackErr := i.rollbackTransactionAndRemoveMetadata(manifest, state, true); rollbackErr != nil {
				return Receipt{}, fmt.Errorf("write install manifest: %w; rollback also failed: %w", err, rollbackErr)
			}
			return Receipt{}, fmt.Errorf("write install manifest: %w", err)
		}
		manifestPublished = true
	}
	if err := i.writeReceipt(manifest.ReceiptPath, receipt); err != nil {
		if rollbackErr := i.rollbackTransactionAndRemoveMetadata(manifest, state, manifestPublished); rollbackErr != nil {
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
	installed, err := readRegularFile(manifest.Core.Destination, "installed artifact")
	if err != nil {
		return nil, err
	}
	installedSHA := sha256Hex(installed)
	if installedSHA != manifest.PreviousSHA256 && installedSHA != manifest.Core.SHA256 {
		return nil, fmt.Errorf("installed artifact sha256 mismatch: got %s want current %s or candidate %s", installedSHA, manifest.PreviousSHA256, manifest.Core.SHA256)
	}
	state := &preflight{
		candidate:            candidate,
		previous:             installed,
		previousSHA256:       installedSHA,
		manifest:             manifestBytes,
		reuseManifest:        reuseManifest,
		coreAlreadyInstalled: installedSHA == manifest.Core.SHA256,
	}
	if state.coreAlreadyInstalled {
		backup, backupErr := readRegularFile(manifest.BackupPath, "rollback backup")
		if backupErr != nil {
			return nil, fmt.Errorf("installed artifact matches candidate: %w", backupErr)
		}
		if backupSHA := sha256Hex(backup); backupSHA != manifest.PreviousSHA256 {
			return nil, fmt.Errorf("rollback backup sha256 mismatch: got %s want %s", backupSHA, manifest.PreviousSHA256)
		}
		state.previous = backup
		state.previousSHA256 = manifest.PreviousSHA256
		state.reuseBackup = true
	}
	managedFiles, err := preflightManagedFiles(manifest.ManagedFiles)
	if err != nil {
		return nil, err
	}
	state.managedFiles = managedFiles

	receiptInfo, receiptStatErr := os.Lstat(manifest.ReceiptPath)
	if receiptStatErr != nil && !errors.Is(receiptStatErr, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect install receipt: %w", receiptStatErr)
	}
	receiptExists := receiptStatErr == nil
	if receiptExists && !receiptInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("receipt path must be a regular file, got mode %s", receiptInfo.Mode())
	}
	if state.coreAlreadyInstalled {
		if !allManagedFilesInstalled(state.managedFiles) {
			if receiptExists {
				return nil, errors.New("receipt exists but one or more managed files do not match the candidate")
			}
			return state, nil
		}
		if !receiptExists {
			return state, nil
		}
		receipt, loadErr := loadReceipt(manifest.ReceiptPath)
		if loadErr != nil {
			return nil, fmt.Errorf("installed artifacts match candidate but receipt is not valid: %w", loadErr)
		}
		if !receiptMatchesManifest(receipt, manifest) {
			return nil, errors.New("installed artifacts match candidate but receipt identifies a different release")
		}
		if !reuseManifest {
			return nil, errors.New("installed artifacts and receipt match candidate but canonical manifest is absent")
		}
		state.noopReceipt = &receipt
		return state, nil
	}

	backupExists, err := existingRegularFileMatches(manifest.BackupPath, installedSHA)
	if err != nil {
		return nil, fmt.Errorf("preflight backup: %w", err)
	}
	state.reuseBackup = backupExists
	if err := requireAbsentOrRegular(manifest.ReceiptPath, "receipt"); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(manifest.ReceiptPath); err == nil {
		return nil, errors.New("receipt path already exists for a different installed artifact")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect receipt path: %w", err)
	}
	return state, nil
}

func preflightManagedFiles(files []ManagedFile) ([]managedFilePreflight, error) {
	states := make([]managedFilePreflight, 0, len(files))
	for _, file := range files {
		state := managedFilePreflight{file: file}
		candidate, err := readRegularFile(file.Source, "managed file "+file.Name+" source")
		if err != nil {
			return nil, err
		}
		if got := sha256Hex(candidate); got != file.SHA256 {
			return nil, fmt.Errorf("managed file %q source sha256 mismatch: got %s want %s", file.Name, got, file.SHA256)
		}
		state.candidate = candidate

		info, err := os.Lstat(file.Destination)
		if errors.Is(err, os.ErrNotExist) {
			if file.PreviousSHA256 != "" {
				return nil, fmt.Errorf("managed file %q destination is missing, want prior sha256 %s", file.Name, file.PreviousSHA256)
			}
			states = append(states, state)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect managed file %q destination: %w", file.Name, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("managed file %q destination must be a regular file, got mode %s", file.Name, info.Mode())
		}
		installed, err := os.ReadFile(file.Destination)
		if err != nil {
			return nil, fmt.Errorf("read managed file %q destination: %w", file.Name, err)
		}
		installedSHA := sha256Hex(installed)
		if installedSHA == file.SHA256 {
			state.alreadyInstalled = true
			if file.PreviousSHA256 != "" {
				backup, backupErr := readRegularFile(file.BackupPath, "managed file "+file.Name+" backup")
				if backupErr != nil {
					return nil, fmt.Errorf("managed file %q matches candidate: %w", file.Name, backupErr)
				}
				if got := sha256Hex(backup); got != file.PreviousSHA256 {
					return nil, fmt.Errorf("managed file %q backup sha256 mismatch: got %s want %s", file.Name, got, file.PreviousSHA256)
				}
				state.previous = backup
				state.previousPresent = true
				state.reuseBackup = true
			}
			states = append(states, state)
			continue
		}
		if file.PreviousSHA256 == "" {
			return nil, fmt.Errorf("managed file %q destination exists with sha256 %s, want absent or candidate %s", file.Name, installedSHA, file.SHA256)
		}
		if installedSHA != file.PreviousSHA256 {
			return nil, fmt.Errorf("managed file %q destination sha256 mismatch: got %s want prior %s or candidate %s", file.Name, installedSHA, file.PreviousSHA256, file.SHA256)
		}
		state.previous = installed
		state.previousPresent = true
		backupExists, backupErr := existingRegularFileMatches(file.BackupPath, file.PreviousSHA256)
		if backupErr != nil {
			return nil, fmt.Errorf("preflight managed file %q backup: %w", file.Name, backupErr)
		}
		state.reuseBackup = backupExists
		states = append(states, state)
	}
	return states, nil
}

func allManagedFilesInstalled(files []managedFilePreflight) bool {
	for _, file := range files {
		if !file.alreadyInstalled {
			return false
		}
	}
	return true
}

func receiptManagedFiles(files []ManagedFile) []ReceiptManagedFile {
	result := make([]ReceiptManagedFile, 0, len(files))
	for _, file := range files {
		result = append(result, ReceiptManagedFile{Name: file.Name, SHA256: file.SHA256, PreviousSHA256: file.PreviousSHA256})
	}
	return result
}

func receiptMatchesManifest(receipt Receipt, manifest Manifest) bool {
	if receipt.ManifestSHA256 != manifest.ManifestSHA256 || receipt.ArtifactSHA256 != manifest.Core.SHA256 || receipt.PreviousSHA256 != manifest.PreviousSHA256 || receipt.ReleaseID != manifest.ReleaseID {
		return false
	}
	want := receiptManagedFiles(manifest.ManagedFiles)
	if len(receipt.ManagedFiles) != len(want) {
		return false
	}
	for index := range want {
		if receipt.ManagedFiles[index] != want[index] {
			return false
		}
	}
	return true
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

func (i *installer) rollbackTransaction(manifest Manifest, state *preflight) error {
	for index := len(state.managedFiles) - 1; index >= 0; index-- {
		file := state.managedFiles[index]
		if !file.alreadyInstalled && !file.published {
			continue
		}
		if !file.previousPresent {
			if err := i.removeRegularFileIfPresent(file.file.Destination, "new managed file "+file.file.Name); err != nil {
				return err
			}
			continue
		}
		if err := i.writeAtomic(file.file.Destination, file.previous, os.FileMode(file.file.Mode)); err != nil {
			return fmt.Errorf("restore managed file %q: %w", file.file.Name, err)
		}
		got, err := digestRegularFile(file.file.Destination)
		if err != nil {
			return fmt.Errorf("verify restored managed file %q: %w", file.file.Name, err)
		}
		if got != file.file.PreviousSHA256 {
			return fmt.Errorf("restore managed file %q sha256 got %s want %s", file.file.Name, got, file.file.PreviousSHA256)
		}
	}
	if !state.coreAlreadyInstalled && !state.corePublished {
		return nil
	}
	if err := i.writeAtomic(manifest.Core.Destination, state.previous, os.FileMode(manifest.Core.Mode)); err != nil {
		return err
	}
	got, err := digestRegularFile(manifest.Core.Destination)
	if err != nil {
		return err
	}
	if got != state.previousSHA256 {
		return fmt.Errorf("rollback sha256 got %s want %s", got, state.previousSHA256)
	}
	return nil
}

func (i *installer) rollbackTransactionAndRemoveMetadata(manifest Manifest, state *preflight, removeManifest bool) error {
	if err := i.rollbackTransaction(manifest, state); err != nil {
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
