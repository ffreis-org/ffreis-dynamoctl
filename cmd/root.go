// Package cmd contains all dynamoctl CLI commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	platformaws "github.com/ffreis/dynamoctl/internal/aws"
	"github.com/ffreis/dynamoctl/internal/backup"
	appcfg "github.com/ffreis/dynamoctl/internal/config"
	"github.com/ffreis/dynamoctl/internal/store"
	"github.com/ffreis/platform-cli/pkg/ui"
)

// Build-time variables injected via -ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// global flag values — set by cobra PersistentFlags.
var (
	flagTable         string
	flagNamespace     string
	flagRegion        string
	flagProfile       string
	flagEncryptionKey string
	flagOutput        string
	flagJSON          bool
	flagLogLevel      string
	flagUI            string
	cliUI             *ui.Presenter
)

const (
	exitOK       = 0
	exitError    = 1
	exitNotFound = 2
)

// rootCmd is the top-level command.
var rootCmd = &cobra.Command{
	Use:   "dynamoctl",
	Short: "Encrypted key-value store backed by DynamoDB",
	Long: `dynamoctl manages encrypted secrets and configuration values in DynamoDB.

Values are encrypted with AES-256-GCM before storage — no KMS or external
key management is required. Supply the same key at read time to decrypt.

Encryption key:
  Set DYNAMOCTL_KEY to a 64-character hex string (32 bytes):
    export DYNAMOCTL_KEY=$(openssl rand -hex 32)
  Or pass --encryption-key on every call.

Quick start:
  dynamoctl set  myapp/api-key ghp_xxxx
  dynamoctl get  myapp/api-key
  dynamoctl list
  dynamoctl backup  --bucket my-backups
  dynamoctl restore --bucket my-backups --key dynamoctl-backups/...
`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Called by main.
func Execute() int {
	return executeCommand(rootCmd, os.Stderr)
}

func executeCommand(cmd *cobra.Command, stderr io.Writer) int {
	if err := cmd.Execute(); err != nil {
		if message := err.Error(); message != "" {
			_, _ = io.WriteString(stderr, "error: "+message+"\n")
		}
		if errors.Is(err, store.ErrNotFound) {
			return exitNotFound
		}
		return exitError
	}
	return exitOK
}

// Factories used by commands; overridden in tests.
var (
	storeFactory    = func(ctx context.Context) (store.Store, error) { return newAWSStore(ctx) }
	s3ClientFactory = func(ctx context.Context) (backup.S3Client, error) { return newAWSS3Client(ctx) }
)

var loadDefaultConfig = config.LoadDefaultConfig

func init() {
	rootCmd.PersistentPreRunE = setupCLI

	f := rootCmd.PersistentFlags()
	f.StringVarP(&flagTable, "table", "t",
		envOrDefault(appcfg.EnvTable, appcfg.DefaultTableName),
		"DynamoDB table name (env: "+appcfg.EnvTable+")")
	f.StringVarP(&flagNamespace, "namespace", "n",
		envOrDefault(appcfg.EnvNamespace, appcfg.DefaultNamespace),
		"Key namespace (env: "+appcfg.EnvNamespace+")")
	f.StringVarP(&flagRegion, "region", "r",
		envOrDefault(appcfg.EnvAWSRegion, platformaws.DefaultRegionUSEast1),
		"AWS region (env: "+appcfg.EnvAWSRegion+")")
	f.StringVarP(&flagProfile, "profile", "p",
		os.Getenv("AWS_PROFILE"),
		"AWS CLI profile (env: AWS_PROFILE)")
	f.StringVar(&flagEncryptionKey, "encryption-key",
		os.Getenv(appcfg.EnvKey),
		"AES-256 key as 64-char hex string (env: "+appcfg.EnvKey+")")
	f.StringVar(&flagOutput, "output", "text", "Output format: text, json")
	f.BoolVar(&flagJSON, "json", false, "Deprecated: alias for --output=json")
	f.StringVar(&flagLogLevel, "log-level", "info",
		"Log level: debug, info, warn, error")
	f.StringVar(&flagUI, "ui", "auto", "UI mode: auto, plain, rich")

	rootCmd.AddCommand(
		newSetCmd(),
		newGetCmd(),
		newListCmd(),
		newDeleteCmd(),
		newRotateCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newVersionCmd(),
	)
}

func setupCLI(cmd *cobra.Command, args []string) error {
	if err := setupLogger(cmd, args); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(flagOutput)) {
	case "", "text", "json":
		// valid
	default:
		return fmt.Errorf("invalid output format %q: must be one of: text, json", flagOutput)
	}

	presenter, err := ui.New(flagUI)
	if err != nil {
		return err
	}
	cliUI = presenter
	cmd.SetContext(ui.WithPresenter(cmd.Context(), presenter))
	return nil
}

// setupLogger initialises the global slog logger from the --log-level flag.
func setupLogger(_ *cobra.Command, _ []string) error {
	var level slog.Level
	switch flagLogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	return nil
}

func currentOutput() string {
	if flagJSON {
		return "json"
	}
	switch output := strings.ToLower(strings.TrimSpace(flagOutput)); output {
	case "", "text", "table":
		return "text"
	case "json":
		return "json"
	default:
		return "text"
	}
}

// newAWSStore builds the DynamoDB store from global flags.
func newAWSStore(ctx context.Context) (*store.DynamoStore, error) {
	cfg, err := newAWSConfig(ctx)
	if err != nil {
		return nil, err
	}
	return store.New(awsdynamodb.NewFromConfig(cfg), flagTable), nil
}

// newAWSS3Client builds an S3 client from global flags.
func newAWSS3Client(ctx context.Context) (*awss3.Client, error) {
	cfg, err := newAWSConfig(ctx)
	if err != nil {
		return nil, err
	}
	return awss3.NewFromConfig(cfg), nil
}

// newAWSConfig builds an aws.Config from global flags.
func newAWSConfig(ctx context.Context) (awsconfig.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(flagRegion),
	}
	if flagProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(flagProfile))
	}
	cfg, err := loadDefaultConfig(ctx, opts...)
	if err != nil {
		return awsconfig.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
