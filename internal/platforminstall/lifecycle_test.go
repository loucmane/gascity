package platforminstall

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

type fakeLifecycle struct {
	restarts   int
	verifies   int
	restartErr error
	verifyErr  error
	proof      RuntimeProof
}

func (lifecycle *fakeLifecycle) Restart(context.Context, Manifest) error {
	lifecycle.restarts++
	return lifecycle.restartErr
}

func (lifecycle *fakeLifecycle) Verify(context.Context, Manifest) (RuntimeProof, error) {
	lifecycle.verifies++
	return lifecycle.proof, lifecycle.verifyErr
}

func TestApplyRestartsExactlyOnceAndRecordsRuntimeProof(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	lifecycle := exactFakeLifecycle(manifest)

	receipt, err := Apply(context.Background(), manifest, lifecycle)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if lifecycle.restarts != 1 || lifecycle.verifies != 1 {
		t.Fatalf("lifecycle calls restart=%d verify=%d, want 1/1", lifecycle.restarts, lifecycle.verifies)
	}
	if receipt.Activation == nil || *receipt.Activation != lifecycle.proof {
		t.Fatalf("receipt activation = %+v, want %+v", receipt.Activation, lifecycle.proof)
	}
	persisted, err := loadReceipt(manifest.ReceiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Activation == nil || *persisted.Activation != lifecycle.proof {
		t.Fatalf("persisted activation = %+v, want %+v", persisted.Activation, lifecycle.proof)
	}
}

func TestApplyRestartFailureRestoresExactFilesystemAndMetadata(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	lifecycle := exactFakeLifecycle(manifest)
	lifecycle.restartErr = errors.New("injected restart failure")
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	rulesBefore := mustReadFile(t, manifest.ManagedFiles[0].Destination)

	_, err := Apply(context.Background(), manifest, lifecycle)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected restart failure")) {
		t.Fatalf("Apply() error = %v, want restart failure", err)
	}
	if lifecycle.restarts != 1 || lifecycle.verifies != 0 {
		t.Fatalf("lifecycle calls restart=%d verify=%d, want 1/0", lifecycle.restarts, lifecycle.verifies)
	}
	assertRestoredActivationFilesystem(t, manifest, coreBefore, rulesBefore)
}

func TestApplyPostcheckFailureRestoresExactFilesystemWithoutSecondRestart(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	lifecycle := exactFakeLifecycle(manifest)
	lifecycle.verifyErr = errors.New("injected postcheck failure")
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	rulesBefore := mustReadFile(t, manifest.ManagedFiles[0].Destination)

	_, err := Apply(context.Background(), manifest, lifecycle)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected postcheck failure")) {
		t.Fatalf("Apply() error = %v, want postcheck failure", err)
	}
	if lifecycle.restarts != 1 || lifecycle.verifies != 1 {
		t.Fatalf("lifecycle calls restart=%d verify=%d, want 1/1", lifecycle.restarts, lifecycle.verifies)
	}
	assertRestoredActivationFilesystem(t, manifest, coreBefore, rulesBefore)
}

func TestApplyRejectsMismatchedRuntimeProofAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	lifecycle := exactFakeLifecycle(manifest)
	lifecycle.proof.Commit = "0000000000000000000000000000000000000000"
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	rulesBefore := mustReadFile(t, manifest.ManagedFiles[0].Destination)

	_, err := Apply(context.Background(), manifest, lifecycle)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("runtime commit")) {
		t.Fatalf("Apply() error = %v, want runtime commit mismatch", err)
	}
	assertRestoredActivationFilesystem(t, manifest, coreBefore, rulesBefore)
}

func TestApplyIdenticalReplayVerifiesWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	first := exactFakeLifecycle(manifest)
	if _, err := Apply(context.Background(), manifest, first); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	coreBefore := mustStat(t, manifest.Core.Destination)
	second := exactFakeLifecycle(manifest)

	receipt, err := Apply(context.Background(), manifest, second)
	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if receipt.Result != ResultNoop {
		t.Fatalf("second result = %q, want noop", receipt.Result)
	}
	if second.restarts != 0 || second.verifies != 1 {
		t.Fatalf("second lifecycle calls restart=%d verify=%d, want 0/1", second.restarts, second.verifies)
	}
	assertSameFileIdentityAndTime(t, manifest.Core.Destination, coreBefore)
}

func TestApplyInterruptedBeforeRestartVerifiesThenRestartsOnce(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	lifecycle := exactFakeLifecycle(manifest)
	lifecycle.verifyErr = errors.New("old runtime still serving")
	verifyCalls := 0
	dynamic := &dynamicLifecycle{
		restart: func(context.Context, Manifest) error {
			lifecycle.restarts++
			return nil
		},
		verify: func(context.Context, Manifest) (RuntimeProof, error) {
			verifyCalls++
			if verifyCalls == 1 {
				return RuntimeProof{}, lifecycle.verifyErr
			}
			return lifecycle.proof, nil
		},
	}

	if _, err := Apply(context.Background(), manifest, dynamic); err != nil {
		t.Fatalf("Apply() recovery error = %v", err)
	}
	if lifecycle.restarts != 1 || verifyCalls != 2 {
		t.Fatalf("recovery calls restart=%d verify=%d, want 1/2", lifecycle.restarts, verifyCalls)
	}
}

func TestActivationPlanNamesPotentialRestartAndFinalVerification(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)

	steps, err := Plan(manifest)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	want := map[string]bool{
		"verify-runtime-or-restart":    false,
		"restart-supervisor-if-needed": true,
		"write-activation-receipt":     true,
		"verify-integrity":             false,
	}
	for _, step := range steps {
		if mutates, exists := want[step.Action]; exists {
			if step.Mutates != mutates {
				t.Errorf("Plan() step %q mutates=%t, want %t", step.Action, step.Mutates, mutates)
			}
			delete(want, step.Action)
		}
	}
	if len(want) != 0 {
		t.Fatalf("Plan() missing activation steps: %v", want)
	}
}

func TestLoadManifestRejectsInvalidActivationIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ActivationSpec)
		field  string
	}{
		{name: "expected commit", mutate: func(spec *ActivationSpec) { spec.ExpectedCommit = "short" }, field: "activation.expected_commit"},
		{name: "expected version", mutate: func(spec *ActivationSpec) { spec.ExpectedVersion = " " }, field: "activation.expected_version"},
		{name: "previous commit", mutate: func(spec *ActivationSpec) { spec.PreviousCommit = "short" }, field: "activation.previous_commit"},
		{name: "previous version", mutate: func(spec *ActivationSpec) { spec.PreviousVersion = " " }, field: "activation.previous_version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := activationManifest(t, t.TempDir())
			test.mutate(manifest.Activation)
			manifest = finalizeManifest(t, manifest)

			_, err := LoadManifest(marshalManifest(t, manifest))
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(test.field)) {
				t.Fatalf("LoadManifest() error = %v, want invalid %s", err, test.field)
			}
		})
	}
}

func TestRevertRestoresPriorPlatformAndVerifiesOneRestart(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	if _, err := Apply(context.Background(), manifest, exactFakeLifecycle(manifest)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	previous := previousFakeLifecycle(manifest)

	proof, err := Revert(context.Background(), manifest, previous)
	if err != nil {
		t.Fatalf("Revert() error = %v", err)
	}
	if proof != previous.proof {
		t.Fatalf("Revert() proof = %+v, want %+v", proof, previous.proof)
	}
	if previous.restarts != 1 || previous.verifies != 1 {
		t.Fatalf("rollback lifecycle calls restart=%d verify=%d, want 1/1", previous.restarts, previous.verifies)
	}
	if got := string(mustReadFile(t, manifest.Core.Destination)); got != "installed" {
		t.Fatalf("rolled-back core = %q, want installed", got)
	}
	if got := string(mustReadFile(t, manifest.ManagedFiles[0].Destination)); got != "rules-v1" {
		t.Fatalf("rolled-back rules = %q, want rules-v1", got)
	}
	assertPathAbsent(t, manifest.ManagedFiles[1].Destination)
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
}

func TestRevertRestartFailureDoesNotRetryAndLeavesPriorBytes(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	if _, err := Apply(context.Background(), manifest, exactFakeLifecycle(manifest)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	previous := previousFakeLifecycle(manifest)
	previous.restartErr = errors.New("injected rollback restart failure")

	_, err := Revert(context.Background(), manifest, previous)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected rollback restart failure")) {
		t.Fatalf("Revert() error = %v, want restart failure", err)
	}
	if previous.restarts != 1 || previous.verifies != 0 {
		t.Fatalf("rollback lifecycle calls restart=%d verify=%d, want 1/0", previous.restarts, previous.verifies)
	}
	if got := string(mustReadFile(t, manifest.Core.Destination)); got != "installed" {
		t.Fatalf("core after rollback restart failure = %q, want installed", got)
	}
}

func TestRollbackPlanNamesReverseRestorationWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := activationManifest(t, dir)
	if _, err := Apply(context.Background(), manifest, exactFakeLifecycle(manifest)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	coreBefore := mustStat(t, manifest.Core.Destination)

	steps, err := RollbackPlan(manifest)
	if err != nil {
		t.Fatalf("RollbackPlan() error = %v", err)
	}
	want := []string{
		"remove-managed-file:validator",
		"restore-managed-file:control-rules",
		"restore-core",
		"remove-receipt",
		"remove-manifest",
		"restart-supervisor",
		"verify-previous-runtime",
	}
	if len(steps) != len(want) {
		t.Fatalf("RollbackPlan() steps = %+v, want %d", steps, len(want))
	}
	for index := range want {
		if steps[index].Order != index+1 || steps[index].Action != want[index] {
			t.Errorf("RollbackPlan() step %d = %+v, want %q", index, steps[index], want[index])
		}
	}
	assertSameFileIdentityAndTime(t, manifest.Core.Destination, coreBefore)
}

type dynamicLifecycle struct {
	restart func(context.Context, Manifest) error
	verify  func(context.Context, Manifest) (RuntimeProof, error)
}

func (lifecycle *dynamicLifecycle) Restart(ctx context.Context, manifest Manifest) error {
	return lifecycle.restart(ctx, manifest)
}

func (lifecycle *dynamicLifecycle) Verify(ctx context.Context, manifest Manifest) (RuntimeProof, error) {
	return lifecycle.verify(ctx, manifest)
}

func activationManifest(t *testing.T, dir string) Manifest {
	t.Helper()
	manifest := testManifestWithManagedFiles(t, dir)
	manifest.Activation = &ActivationSpec{
		ExpectedCommit:  "0123456789abcdef0123456789abcdef01234567",
		ExpectedVersion: "gc version 1.4.1-test",
		PreviousCommit:  "89abcdef0123456789abcdef0123456789abcdef",
		PreviousVersion: "gc version previous",
	}
	return finalizeManifest(t, manifest)
}

func previousFakeLifecycle(manifest Manifest) *fakeLifecycle {
	return &fakeLifecycle{proof: RuntimeProof{
		ExecutableSHA256: manifest.PreviousSHA256,
		Commit:           manifest.Activation.PreviousCommit,
		Version:          manifest.Activation.PreviousVersion,
	}}
}

func exactFakeLifecycle(manifest Manifest) *fakeLifecycle {
	return &fakeLifecycle{proof: RuntimeProof{
		ExecutableSHA256: manifest.Core.SHA256,
		Commit:           manifest.Activation.ExpectedCommit,
		Version:          manifest.Activation.ExpectedVersion,
	}}
}

func assertRestoredActivationFilesystem(t *testing.T, manifest Manifest, coreBefore, rulesBefore []byte) {
	t.Helper()
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, coreBefore) {
		t.Fatalf("core after rollback = %q, want %q", got, coreBefore)
	}
	if got := mustReadFile(t, manifest.ManagedFiles[0].Destination); !bytes.Equal(got, rulesBefore) {
		t.Fatalf("rules after rollback = %q, want %q", got, rulesBefore)
	}
	assertPathAbsent(t, manifest.ManagedFiles[1].Destination)
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
	if _, err := os.Stat(manifest.BackupPath); err != nil {
		t.Fatalf("core rollback backup not retained: %v", err)
	}
}
