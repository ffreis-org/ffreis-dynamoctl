package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/output"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all keys in the current namespace",
		Long: `List prints every key in the namespace (metadata only — values are never shown).

Use --namespace / -n to target a different namespace.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}

			items, err := st.List(ctx, flagNamespace)
			if err != nil {
				return fmt.Errorf("listing items: %w", err)
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintListResult(items)
		},
	}
}
