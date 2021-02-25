package cmd

import (
	"context"
	"fmt"

	"github.com/qri-io/ioes"
	"github.com/qri-io/qri/lib"
	"github.com/spf13/cobra"
)

// NewWhatChangedCommand creates a new `qri whatchanged` command that shows what changed at a commit
func NewWhatChangedCommand(f Factory, ioStreams ioes.IOStreams) *cobra.Command {
	o := &WhatChangedOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:    "whatchanged DATASET",
		Hidden: true,
		Short:  "shows what changed at a particular commit",
		Long: `Shows what changed for components at a particular commit, that is, which
were added, modified or removed. This is analogous to the status command,
except only available for dataset versions in history.`,
		Example: `  # Show what changed for the head commit
  $ qri whatchanged me/dataset_name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(f, args); err != nil {
				return err
			}
			return o.Run()
		},
	}

	return cmd
}

// WhatChangedOptions encapsulates state for the whatchanged command
type WhatChangedOptions struct {
	ioes.IOStreams

	Refs       *RefSelect

	Instance *lib.Instance
}

// Complete adds any missing configuration that can only be added just before calling Run
func (o *WhatChangedOptions) Complete(f Factory, args []string) (err error) {
	fsiMethods, _ := f.FSIMethods()
	o.Instance = f.Instance()
	o.Refs, err = GetCurrentRefSelect(f, args, 1, EnsureFSIAgrees(fsiMethods))
	return nil
}

// Run executes the whatchanged command
func (o *WhatChangedOptions) Run() (err error) {
	printRefSelect(o.ErrOut, o.Refs)

	ref := o.Refs.Ref()

	ctx := context.Background()
	inst := o.Instance

	params := lib.WhatChangedParams{Refstr: ref}

	res, err := inst.Filesys().WhatChanged(ctx, &params)
	if err != nil {
		printErr(o.ErrOut, err)
		return nil
	}

	for _, si := range res {
		printInfo(o.Out, fmt.Sprintf("  %s: %s", si.Component, si.Type))
	}

	return nil
}
