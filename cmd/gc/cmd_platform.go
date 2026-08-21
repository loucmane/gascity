package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

var errPlatformInstallDisabled = errors.New("platform install command is disabled")

func newPlatformCmd(_ io.Writer, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "platform",
		Short: "Manage a versioned Gas City platform installation",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errPlatformInstallDisabled
		},
	}
}
