package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/crypto"
	"github.com/ffreis/dynamoctl/internal/output"
	"github.com/ffreis/dynamoctl/internal/store"
)

func newSetCmd() *cobra.Command {
	var flagNoEncrypt bool
	var flagStdin bool

	cmd := &cobra.Command{
		Use:   "set <name> [value]",
		Short: "Set a key-value pair (encrypted by default)",
		Long: `Set stores a value under the given name in the current namespace.

By default the value is encrypted with AES-256-GCM using the key from
--encryption-key / DYNAMOCTL_KEY.  Pass --no-encrypt to store plaintext.

The value may be supplied as a positional argument or piped via stdin
(use --stdin or pass "-" as the value).`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			rawValue, err := resolveSetValue(os.Stdin, flagStdin, args)
			if err != nil {
				return err
			}

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}

			storeValue, encrypted, err := encryptValue(rawValue, flagNoEncrypt, flagEncryptionKey)
			if err != nil {
				return err
			}

			item := store.Item{
				Namespace: flagNamespace,
				Name:      name,
				Value:     storeValue,
				Encrypted: encrypted,
			}
			version, err := storeAndGetVersion(ctx, st, &item)
			if err != nil {
				return err
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintSetResult(flagNamespace, name, version)
		},
	}

	cmd.Flags().BoolVar(&flagNoEncrypt, "no-encrypt", false, "Store value without encryption")
	cmd.Flags().BoolVar(&flagStdin, "stdin", false, "Read value from stdin")
	return cmd
}

func resolveSetValue(stdin io.Reader, useStdin bool, args []string) (string, error) {
	switch {
	case useStdin:
		return readStdinValue(stdin)
	case len(args) == 2 && args[1] == "-":
		return readStdinValue(stdin)
	case len(args) == 2:
		return args[1], nil
	default:
		return "", fmt.Errorf("provide a value as argument or use --stdin")
	}
}

func readStdinValue(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func encryptValue(rawValue string, noEncrypt bool, encryptionKey string) (value string, encrypted bool, err error) {
	if noEncrypt {
		return rawValue, false, nil
	}

	if encryptionKey == "" {
		return "", false, fmt.Errorf("encryption key required: set DYNAMOCTL_KEY or use --encryption-key (or pass --no-encrypt)")
	}

	key, err := crypto.ParseKey(encryptionKey)
	if err != nil {
		return "", false, fmt.Errorf("invalid encryption key: %w", err)
	}

	storeValue, err := crypto.Encrypt([]byte(rawValue), key)
	if err != nil {
		return "", false, fmt.Errorf("encrypting value: %w", err)
	}
	return storeValue, true, nil
}

func storeAndGetVersion(ctx context.Context, st store.Store, item *store.Item) (version int, err error) {
	if err := st.Put(ctx, item); err != nil {
		return 0, fmt.Errorf("storing item: %w", err)
	}

	stored, err := st.Get(ctx, item.Namespace, item.Name)
	if err != nil {
		return 0, fmt.Errorf("confirming set: %w", err)
	}
	return stored.Version, nil
}
