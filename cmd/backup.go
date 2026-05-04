package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/backup"
	appcfg "github.com/ffreis/dynamoctl/internal/config"
	"github.com/ffreis/dynamoctl/internal/output"
)

func newBackupCmd() *cobra.Command {
	var flagBucket string
	var flagPrefix string
	var flagAllNamespaces bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Dump items to an S3 backup file",
		Long: `Backup exports items from DynamoDB to a JSON file in S3.

Encrypted values are preserved as-is — the encryption key is NOT required
for backup (only for restore + subsequent get).

The S3 key follows the pattern:
  <prefix>/<table>/<namespace>/<YYYYMMDD-HHMMSS>.json

Examples:
  dynamoctl backup --bucket my-backups
  dynamoctl backup --bucket my-backups --all
  dynamoctl backup --bucket my-backups --prefix custom-prefix`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flagBucket == "" {
				return fmt.Errorf("--bucket is required")
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

			ns := flagNamespace
			if flagAllNamespaces {
				ns = ""
			}

			opts := backup.DumpOptions{
				Table:     flagTable,
				Namespace: ns,
				Bucket:    flagBucket,
				Prefix:    flagPrefix,
			}

			s3URI, count, err := backup.Dump(ctx, st, s3c, opts)
			if err != nil {
				return fmt.Errorf("backup failed: %w", err)
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintBackupResult(s3URI, count)
		},
	}

	cmd.Flags().StringVar(&flagBucket, "bucket", "", "S3 bucket name (required)")
	cmd.Flags().StringVar(&flagPrefix, "prefix", "", "S3 key prefix (default: "+appcfg.DefaultBackupPrefix+")")
	cmd.Flags().BoolVar(&flagAllNamespaces, "all", false, "Back up all namespaces (ignores --namespace)")
	return cmd
}
