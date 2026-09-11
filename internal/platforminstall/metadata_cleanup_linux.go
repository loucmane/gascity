package platforminstall

import (
	"errors"
	"fmt"
)

func metadataCloseOnce(closeResource func() error) func() error {
	closed := false
	return func() error {
		if closed {
			return nil
		}
		closed = true
		return closeResource()
	}
}

func metadataCommittedError(err error, committed bool) error {
	if err != nil && committed {
		return fmt.Errorf("committed_evidence_incomplete: %w", err)
	}
	return err
}

func metadataFinishChild(prior error, closeInput func() error, cancel func(), wait, terminal func() error) error {
	result := errors.Join(prior, closeInput())
	if result != nil {
		cancel()
	}
	return errors.Join(result, wait(), terminal())
}
