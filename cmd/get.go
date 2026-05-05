package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/crypto"
	"github.com/ffreis/dynamoctl/internal/output"
	"github.com/ffreis/dynamoctl/internal/store"
)

func newGetCmd() *cobra.Command {
	var flagRaw bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Retrieve a value (decrypted by default)",
		Long: `Get retrieves the value stored under <name> in the current namespace.

If the item was stored encrypted, it is decrypted using --encryption-key /
DYNAMOCTL_KEY before printing.  Pass --raw to print the ciphertext as-is.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}

			item, err := getItemForGetCmd(ctx, st, flagNamespace, name)
			if err != nil {
				return err
			}

			decrypted, err := decryptForGetCmd(item, flagRaw, flagEncryptionKey)
			if err != nil {
				return err
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintGetResult(item, decrypted)
		},
	}

	cmd.Flags().BoolVar(&flagRaw, "raw", false, "Print raw (possibly encrypted) value without decryption")
	return cmd
}

func getItemForGetCmd(ctx context.Context, st store.Store, namespace, name string) (*store.Item, error) {
	item, err := st.Get(ctx, namespace, name)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("key %q not found in namespace %q: %w", name, namespace, store.ErrNotFound)
	}
	return nil, fmt.Errorf("retrieving item: %w", err)
}

func decryptForGetCmd(item *store.Item, raw bool, encryptionKey string) (string, error) {
	if item == nil || !item.Encrypted || raw {
		return "", nil
	}

	if encryptionKey == "" {
		return "", fmt.Errorf("item is encrypted; set DYNAMOCTL_KEY or use --encryption-key (or pass --raw)")
	}
	key, err := crypto.ParseKey(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key: %w", err)
	}
	plain, err := crypto.Decrypt(item.Value, key)
	if err != nil {
		return "", fmt.Errorf("decrypting value: %w", err)
	}
	return string(plain), nil
}
