package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/output"
	"github.com/ffreis/dynamoctl/internal/store"
)

func newDeleteCmd() *cobra.Command {
	var flagForce bool

	cmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a key",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}

			if !flagForce {
				// Verify item exists before deleting.
				_, err := st.Get(ctx, flagNamespace, name)
				if err != nil {
					if errors.Is(err, store.ErrNotFound) {
						return fmt.Errorf("key %q not found in namespace %q", name, flagNamespace)
					}
					return fmt.Errorf("checking item: %w", err)
				}
			}

			if err := st.Delete(ctx, flagNamespace, name); err != nil {
				return fmt.Errorf("deleting item: %w", err)
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintDeleteResult(flagNamespace, name)
		},
	}

	cmd.Flags().BoolVar(&flagForce, "force", false, "Delete without verifying the key exists first")
	return cmd
}
