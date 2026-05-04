package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/crypto"
	"github.com/ffreis/dynamoctl/internal/output"
	"github.com/ffreis/dynamoctl/internal/store"
)

func newRotateCmd() *cobra.Command {
	var flagNewKey string

	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Re-encrypt all items in the namespace with a new key",
		Long: `Rotate decrypts every encrypted item in the namespace using the current
--encryption-key, then re-encrypts each one with --new-key and writes it
back atomically (using a conditional update to prevent lost writes).

Plaintext items are skipped.  Items that fail are counted but do not abort
the rotation — check the exit code and re-run if needed.

Example:
  export OLD_KEY=$DYNAMOCTL_KEY
  export NEW_KEY=$(openssl rand -hex 32)
  dynamoctl rotate --encryption-key $OLD_KEY --new-key $NEW_KEY
  export DYNAMOCTL_KEY=$NEW_KEY`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			oldKey, newKey, err := parseRotateKeys(flagEncryptionKey, flagNewKey)
			if err != nil {
				return err
			}

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}

			items, err := st.List(ctx, flagNamespace)
			if err != nil {
				return fmt.Errorf("listing items: %w", err)
			}

			result := rotateResult{}

			for _, item := range items {
				itemCopy := item
				log := slog.With(logKeyNamespace, itemCopy.Namespace, logKeyName, itemCopy.Name)

				if !itemCopy.Encrypted {
					log.Debug("skipping plaintext item")
					result.skipped++
					continue
				}

				if err := rotateEncryptedItem(ctx, st, &itemCopy, oldKey, newKey, log); err != nil {
					result.failed++
					continue
				}

				log.Debug("rotated item")
				result.rotated++
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintRotateResult(flagNamespace, result.rotated, result.skipped, result.failed)
		},
	}

	cmd.Flags().StringVar(&flagNewKey, flagNewKeyName, "", "New AES-256 key as 64-char hex string (required)")
	_ = cmd.MarkFlagRequired(flagNewKeyName)
	return cmd
}

type rotateResult struct {
	rotated int
	skipped int
	failed  int
}

func parseRotateKeys(currentKey, newKey string) (oldKey, parsedNewKey crypto.Key, err error) {
	if currentKey == "" {
		return crypto.Key{}, crypto.Key{}, fmt.Errorf("current encryption key required: set DYNAMOCTL_KEY or use --encryption-key")
	}
	if newKey == "" {
		return crypto.Key{}, crypto.Key{}, fmt.Errorf("new encryption key required: use --new-key")
	}
	if currentKey == newKey {
		return crypto.Key{}, crypto.Key{}, fmt.Errorf("--new-key must differ from the current encryption key")
	}

	oldKey, err = crypto.ParseKey(currentKey)
	if err != nil {
		return crypto.Key{}, crypto.Key{}, fmt.Errorf("invalid current encryption key: %w", err)
	}
	parsedNewKey, err = crypto.ParseKey(newKey)
	if err != nil {
		return crypto.Key{}, crypto.Key{}, fmt.Errorf("invalid new encryption key: %w", err)
	}
	return oldKey, parsedNewKey, nil
}

func rotateEncryptedItem(
	ctx context.Context,
	st store.Store,
	item *store.Item,
	oldKey, newKey crypto.Key,
	log *slog.Logger,
) error {
	plain, err := crypto.Decrypt(item.Value, oldKey)
	if err != nil {
		log.Warn("failed to decrypt item, skipping", logKeyError, err)
		return err
	}

	newCiphertext, err := crypto.Encrypt(plain, newKey)
	if err != nil {
		log.Warn("failed to re-encrypt item, skipping", logKeyError, err)
		return err
	}

	err = updateEncryptedWithRetry(ctx, st, item, oldKey, newKey, newCiphertext, log)
	if err != nil {
		log.Warn("failed to rotate item", logKeyError, err)
		return err
	}
	return nil
}

func updateEncryptedWithRetry(
	ctx context.Context,
	st store.Store,
	item *store.Item,
	oldKey, newKey crypto.Key,
	newCiphertext string,
	log *slog.Logger,
) error {
	// Conditional update — retry once on conflict (another writer incremented the version).
	for attempt := range 2 {
		err := st.UpdateEncrypted(ctx, item.Namespace, item.Name, newCiphertext, item.Version)
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrConflict) || attempt == 1 {
			return err
		}

		latest, err := st.Get(ctx, item.Namespace, item.Name)
		if err != nil {
			return err
		}
		*item = *latest

		plain, err := crypto.Decrypt(item.Value, oldKey)
		if err != nil {
			// Item may have already been rotated by a parallel run.
			log.Warn("conflict retry: decrypt failed (may be already rotated)", logKeyError, err)
			return err
		}
		newCiphertext, err = crypto.Encrypt(plain, newKey)
		if err != nil {
			return err
		}
	}

	return nil
}
