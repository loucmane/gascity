//go:build !linux

package platforminstall

import (
	"context"
	"fmt"

	"github.com/gastownhall/gascity/internal/fsys"
)

func metadataImportFS(_ *MetadataLaunch) fsys.FS { return fsys.OSFS{} }
func VerifyMetadataRuntime(context.Context, Manifest) (RuntimeProof, error) {
	return RuntimeProof{}, fmt.Errorf("metadata verifier requires Linux")
}

func RunMetadataTransaction(context.Context, Manifest, bool) (Receipt, []PlanStep, error) {
	return Receipt{}, nil, fmt.Errorf("confined metadata transaction requires Linux")
}
func MetadataWriterEntrypoint() error { return fmt.Errorf("confined metadata writer requires Linux") }
