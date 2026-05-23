// Package backup provides dump-to-S3 and restore-from-S3 operations.
// Encrypted values are preserved as-is in backups; the encryption key is
// NOT required for backup — only for restore + subsequent get.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appcfg "github.com/ffreis/dynamoctl/internal/config"
	"github.com/ffreis/dynamoctl/internal/store"
)

// FormatVersion is the backup file format version.
const FormatVersion = "1"

// S3Client is the subset of the S3 client used for backup operations.
type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Manifest is the top-level structure of a backup file.
type Manifest struct {
	FormatVersion string       `json:"format_version"`
	ExportedAt    time.Time    `json:"exported_at"`
	Table         string       `json:"table"`
	Namespace     string       `json:"namespace"` // empty = all namespaces
	ItemCount     int          `json:"item_count"`
	Items         []ItemRecord `json:"items"`
}

// ItemRecord is the per-item representation in a backup file.
type ItemRecord struct {
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Encrypted bool      `json:"encrypted"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DumpOptions configures a Dump call.
type DumpOptions struct {
	// Table is the source DynamoDB table.
	Table string
	// Namespace filters items to a single namespace. Empty = all.
	Namespace string
	// Bucket is the destination S3 bucket.
	Bucket string
	// Prefix is the S3 key prefix (default: "dynamoctl-backups").
	Prefix string
}

// Dump exports items from DynamoDB to a JSON file in S3.
// Returns the S3 URI of the created backup.
func Dump(ctx context.Context, st store.Store, s3c S3Client, opts DumpOptions) (s3URI string, count int, err error) {
	if opts.Prefix == "" {
		opts.Prefix = appcfg.DefaultBackupPrefix
	}

	// Collect items.
	var items []store.Item
	if opts.Namespace != "" {
		items, err = st.ScanNamespace(ctx, opts.Namespace)
	} else {
		items, err = st.ScanAll(ctx)
	}
	if err != nil {
		return "", 0, fmt.Errorf("scanning items: %w", err)
	}

	// Build manifest.
	records := make([]ItemRecord, len(items))
	for i, it := range items {
		records[i] = ItemRecord{
			Namespace: it.Namespace,
			Name:      it.Name,
			Value:     it.Value,
			Encrypted: it.Encrypted,
			Version:   it.Version,
			CreatedAt: it.CreatedAt,
			UpdatedAt: it.UpdatedAt,
		}
	}

	manifest := Manifest{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now().UTC(),
		Table:         opts.Table,
		Namespace:     opts.Namespace,
		ItemCount:     len(records),
		Items:         records,
	}

	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", 0, fmt.Errorf("marshalling backup manifest: %w", err)
	}

	// Build S3 key: prefix/table/YYYYMMDD-HHMMSS.json
	ts := manifest.ExportedAt.Format("20060102-150405")
	ns := opts.Namespace
	if ns == "" {
		ns = "_all"
	}
	s3Key := fmt.Sprintf("%s/%s/%s/%s.json", opts.Prefix, opts.Table, ns, ts)

	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      sdkaws.String(opts.Bucket),
		Key:         sdkaws.String(s3Key),
		Body:        bytes.NewReader(body),
		ContentType: sdkaws.String("application/json"),
	})
	if err != nil {
		return "", 0, fmt.Errorf("uploading backup to s3://%s/%s: %w", opts.Bucket, s3Key, err)
	}

	s3URI = fmt.Sprintf("s3://%s/%s", opts.Bucket, s3Key)
	slog.Info("backup complete", "s3_uri", s3URI, "items", len(records))
	return s3URI, len(records), nil
}

// RestoreOptions configures a Restore call.
type RestoreOptions struct {
	// Bucket + Key specify the S3 object to restore from.
	Bucket string
	Key    string
	// Overwrite controls whether existing items are overwritten.
	// When false, items that already exist in DynamoDB are skipped.
	Overwrite bool
}

// RestoreResult summarises a Restore operation.
type RestoreResult struct {
	Restored int
	Skipped  int
	Errors   []string
}

// Restore reads a backup manifest from S3 and writes items back to DynamoDB.
// Encrypted items are written as-is — the same encryption key must be used
// when retrieving values via get.
func Restore(ctx context.Context, st store.Store, s3c S3Client, opts RestoreOptions) (*RestoreResult, error) {
	out, err := s3c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: sdkaws.String(opts.Bucket),
		Key:    sdkaws.String(opts.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("downloading backup from s3://%s/%s: %w", opts.Bucket, opts.Key, err)
	}
	defer func() { _ = out.Body.Close() }() // close is best-effort after the body is fully read

	var manifest Manifest
	if err := json.NewDecoder(out.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding backup manifest: %w", err)
	}

	if manifest.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported backup format version %q (expected %q)",
			manifest.FormatVersion, FormatVersion)
	}

	result := &RestoreResult{}

	for _, rec := range manifest.Items {
		if !opts.Overwrite {
			_, err := st.Get(ctx, rec.Namespace, rec.Name)
			if err == nil {
				// Item exists and overwrite is disabled.
				slog.Debug("skipping existing item", "namespace", rec.Namespace, "name", rec.Name)
				result.Skipped++
				continue
			}
			if !errors.Is(err, store.ErrNotFound) {
				msg := fmt.Sprintf("checking %s/%s: %v", rec.Namespace, rec.Name, err)
				result.Errors = append(result.Errors, msg)
				slog.Warn("error checking item, skipping", "error", msg)
				continue
			}
		}

		it := store.Item{
			Namespace: rec.Namespace,
			Name:      rec.Name,
			Value:     rec.Value,
			Encrypted: rec.Encrypted,
			Version:   rec.Version,
			CreatedAt: rec.CreatedAt,
			UpdatedAt: rec.UpdatedAt,
		}
		if err := st.Restore(ctx, &it); err != nil {
			msg := fmt.Sprintf("restoring %s/%s: %v", rec.Namespace, rec.Name, err)
			result.Errors = append(result.Errors, msg)
			slog.Warn("failed to restore item", "error", msg)
			continue
		}
		result.Restored++
	}

	slog.Info("restore complete",
		"restored", result.Restored,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}
