package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/backup"
	"github.com/ffreis/dynamoctl/internal/output"
)

func newRestoreCmd() *cobra.Command {
	var flagBucket string
	var flagKey string
	var flagOverwrite bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore items from an S3 backup file",
		Long: `Restore reads a backup manifest from S3 and writes items back to DynamoDB.

Encrypted items are written as-is — supply the original encryption key at
get-time to decrypt.  Existing items are skipped by default; use --overwrite
to replace them.

Examples:
  dynamoctl restore --bucket my-backups --key dynamoctl-backups/mytable/prod/20240101-120000.json
  dynamoctl restore --bucket my-backups --key <s3-key> --overwrite`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flagBucket == "" {
				return fmt.Errorf("--bucket is required")
			}
			if flagKey == "" {
				return fmt.Errorf("--key is required")
			}

			ctx := cmd.Context()

			st, err := storeFactory(ctx)
			if err != nil {
				return err
			}
			s3c, err := s3ClientFactory(ctx)
			if err != nil {
				return err
			}

			opts := backup.RestoreOptions{
				Bucket:    flagBucket,
				Key:       flagKey,
				Overwrite: flagOverwrite,
			}

			result, err := backup.Restore(ctx, st, s3c, opts)
			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintRestoreResult(result.Restored, result.Skipped, result.Errors)
		},
	}

	cmd.Flags().StringVar(&flagBucket, "bucket", "", "S3 bucket containing the backup (required)")
	cmd.Flags().StringVar(&flagKey, "key", "", "S3 object key of the backup file (required)")
	cmd.Flags().BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing items (default: skip)")
	return cmd
}
