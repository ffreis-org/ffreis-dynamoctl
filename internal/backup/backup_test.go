package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ffreis/dynamoctl/internal/store"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeStore satisfies store.Store in memory.
type fakeStore struct {
	items map[string]*store.Item
}

func newFakeStore() *fakeStore {
	return &fakeStore{items: make(map[string]*store.Item)}
}

func (f *fakeStore) key(ns, name string) string { return ns + "\x00" + name }

func (f *fakeStore) Put(_ context.Context, item *store.Item) error {
	if item == nil {
		return errors.New("nil item")
	}

	now := time.Now().UTC()
	k := f.key(item.Namespace, item.Name)
	existing := f.items[k]

	cp := *item
	if existing == nil {
		if cp.Version == 0 {
			cp.Version = 1
		}
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = now
		}
	} else {
		if cp.Version == 0 {
			cp.Version = existing.Version + 1
		}
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = existing.CreatedAt
		}
	}
	cp.UpdatedAt = now

	f.items[k] = &cp
	return nil
}

func (f *fakeStore) Get(_ context.Context, ns, name string) (*store.Item, error) {
	it, ok := f.items[f.key(ns, name)]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *it
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, ns string) ([]store.Item, error) {
	return f.scanNS(ns)
}

func (f *fakeStore) Delete(_ context.Context, ns, name string) error {
	delete(f.items, f.key(ns, name))
	return nil
}

func (f *fakeStore) ScanNamespace(ctx context.Context, ns string) ([]store.Item, error) {
	return f.List(ctx, ns)
}

func (f *fakeStore) ScanAll(_ context.Context) ([]store.Item, error) {
	out := make([]store.Item, 0, len(f.items))
	for _, it := range f.items {
		out = append(out, *it)
	}
	return out, nil
}

func (f *fakeStore) UpdateEncrypted(_ context.Context, ns, name, val string, ver int) error {
	it, ok := f.items[f.key(ns, name)]
	if !ok || it.Version != ver {
		return store.ErrConflict
	}
	it.Value = val
	it.Version++
	return nil
}

func (f *fakeStore) Restore(_ context.Context, item *store.Item) error {
	if item == nil {
		return errors.New("nil item")
	}
	cp := *item
	f.items[f.key(item.Namespace, item.Name)] = &cp
	return nil
}

func (f *fakeStore) scanNS(ns string) ([]store.Item, error) {
	var out []store.Item
	for _, it := range f.items {
		if it.Namespace == ns {
			out = append(out, *it)
		}
	}
	return out, nil
}

// fakeS3 captures PutObject calls and serves GetObject from a buffer.
type fakeS3 struct {
	puts   map[string][]byte // key → body
	putErr error
	getErr error
}

func newFakeS3() *fakeS3 {
	return &fakeS3{puts: make(map[string][]byte)}
}

func (f *fakeS3) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	body, _ := io.ReadAll(params.Body)
	f.puts[*params.Key] = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.puts[*params.Key]
	if !ok {
		return nil, errors.New("s3: no such key: " + *params.Key)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

// errStore fails on ScanAll.
type errStore struct{ fakeStore }

func (e *errStore) ScanAll(_ context.Context) ([]store.Item, error) {
	return nil, errors.New("dynamo unavailable")
}

// ---------------------------------------------------------------------------
// Tests: Dump
// ---------------------------------------------------------------------------

func TestDumpUploadsSingleNamespace(t *testing.T) {
	st := newFakeStore()
	s3c := newFakeS3()
	ctx := context.Background()

	now := time.Now().UTC()
	_ = st.Put(ctx, &store.Item{Namespace: "prod", Name: "api-key", Value: "enc-val", Encrypted: true, CreatedAt: now, UpdatedAt: now})
	_ = st.Put(ctx, &store.Item{Namespace: "prod", Name: "db-pass", Value: "enc-pass", Encrypted: true, CreatedAt: now, UpdatedAt: now})
	_ = st.Put(ctx, &store.Item{Namespace: "staging", Name: "other", Value: "x", CreatedAt: now, UpdatedAt: now})

	uri, count, err := Dump(ctx, st, s3c, DumpOptions{
		Table:     "dynamoctl",
		Namespace: "prod",
		Bucket:    "my-backups",
	})
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if count != 2 {
		t.Errorf("count: want 2, got %d", count)
	}
	if !strings.HasPrefix(uri, "s3://my-backups/") {
		t.Errorf("uri: unexpected prefix: %s", uri)
	}
}

func TestDumpAllNamespaces(t *testing.T) {
	st := newFakeStore()
	s3c := newFakeS3()
	ctx := context.Background()

	_ = st.Put(ctx, &store.Item{Namespace: "ns1", Name: "a"})
	_ = st.Put(ctx, &store.Item{Namespace: "ns2", Name: "b"})

	_, count, err := Dump(ctx, st, s3c, DumpOptions{Table: testTable, Bucket: testBucket})
	if err != nil {
		t.Fatalf("Dump all: %v", err)
	}
	if count != 2 {
		t.Errorf("count: want 2, got %d", count)
	}
}

func TestDumpManifestIsValidJSON(t *testing.T) {
	st := newFakeStore()
	s3c := newFakeS3()
	ctx := context.Background()

	_ = st.Put(ctx, &store.Item{Namespace: testNamespace, Name: testKey, Value: "v"})

	uri, _, err := Dump(ctx, st, s3c, DumpOptions{Table: testTable, Namespace: testNamespace, Bucket: testBucket})
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}

	// Extract key from URI: s3://bucket/key
	s3Key := strings.TrimPrefix(uri, "s3://"+testBucket+"/")
	body := s3c.puts[s3Key]
	if !json.Valid(body) {
		t.Error("backup body is not valid JSON")
	}

	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if m.FormatVersion != FormatVersion {
		t.Errorf("format_version: want %s, got %s", FormatVersion, m.FormatVersion)
	}
	if m.ItemCount != 1 {
		t.Errorf("item_count: want 1, got %d", m.ItemCount)
	}
}

func TestDumpPropagatesScanError(t *testing.T) {
	es := &errStore{}
	s3c := newFakeS3()

	_, _, err := Dump(context.Background(), es, s3c, DumpOptions{Bucket: testBucket})
	if err == nil {
		t.Error("expected error propagated from scan")
	}
}

func TestDumpPropagatesS3Error(t *testing.T) {
	st := newFakeStore()
	_ = st.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: testKey})
	s3c := newFakeS3()
	s3c.putErr = errors.New("s3 write error")

	_, _, err := Dump(context.Background(), st, s3c, DumpOptions{Bucket: testBucket})
	if err == nil {
		t.Error("expected error propagated from S3 PutObject")
	}
}

// ---------------------------------------------------------------------------
// Tests: Restore
// ---------------------------------------------------------------------------

const errFmtRestore = "Restore: %v"

func dumpAndGetKey(t *testing.T, st store.Store, s3c *fakeS3) string {
	t.Helper()
	ctx := context.Background()
	uri, _, err := Dump(ctx, st, s3c, DumpOptions{Table: testTable, Namespace: testNamespace, Bucket: testBucket})
	requireNoErr(t, err, "Dump")
	return strings.TrimPrefix(uri, "s3://"+testBucket+"/")
}

func TestRestoreRestoresItems(t *testing.T) {
	src := newFakeStore()
	_ = src.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: "a", Value: "enc-a", Encrypted: true})
	_ = src.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: "b", Value: "enc-b", Encrypted: true})

	s3c := newFakeS3()
	s3Key := dumpAndGetKey(t, src, s3c)

	dst := newFakeStore()
	result, err := Restore(context.Background(), dst, s3c, RestoreOptions{Bucket: testBucket, Key: s3Key})
	if err != nil {
		t.Fatalf(errFmtRestore, err)
	}
	if result.Restored != 2 {
		t.Errorf("restored: want 2, got %d", result.Restored)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped: want 0, got %d", result.Skipped)
	}
}

func TestRestoreSkipsExistingWithoutOverwrite(t *testing.T) {
	src := newFakeStore()
	_ = src.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: testKey, Value: "original"})

	s3c := newFakeS3()
	s3Key := dumpAndGetKey(t, src, s3c)

	// Pre-populate the destination with the same item.
	dst := newFakeStore()
	_ = dst.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: testKey, Value: "existing"})

	result, err := Restore(context.Background(), dst, s3c, RestoreOptions{
		Bucket:    testBucket,
		Key:       s3Key,
		Overwrite: false,
	})
	if err != nil {
		t.Fatalf(errFmtRestore, err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped: want 1, got %d", result.Skipped)
	}

	// Verify original value was preserved.
	got, _ := dst.Get(context.Background(), testNamespace, testKey)
	if got.Value != "existing" {
		t.Errorf("value should not have been overwritten: got %q", got.Value)
	}
}

func TestRestoreOverwritesWhenEnabled(t *testing.T) {
	src := newFakeStore()
	_ = src.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: testKey, Value: "backup-val"})

	s3c := newFakeS3()
	s3Key := dumpAndGetKey(t, src, s3c)

	dst := newFakeStore()
	_ = dst.Put(context.Background(), &store.Item{Namespace: testNamespace, Name: testKey, Value: "old"})

	result, err := Restore(context.Background(), dst, s3c, RestoreOptions{
		Bucket:    testBucket,
		Key:       s3Key,
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf(errFmtRestore, err)
	}
	if result.Restored != 1 {
		t.Errorf("restored: want 1, got %d", result.Restored)
	}
}

func TestRestoreRejectsUnknownFormatVersion(t *testing.T) {
	s3c := newFakeS3()
	badManifest := `{"format_version":"999","items":[]}`
	s3c.puts["backup.json"] = []byte(badManifest)

	_, err := Restore(context.Background(), newFakeStore(), s3c, RestoreOptions{Bucket: testBucket, Key: "backup.json"})
	if err == nil {
		t.Error("expected error for unknown format version")
	}
}

func TestRestorePropagatesS3Error(t *testing.T) {
	s3c := newFakeS3()
	s3c.getErr = errors.New("s3 unavailable")

	_, err := Restore(context.Background(), newFakeStore(), s3c, RestoreOptions{Bucket: testBucket, Key: testKey})
	if err == nil {
		t.Error("expected error from S3 GetObject failure")
	}
}

func TestRestorePreservesMetadata(t *testing.T) {
	src := newFakeStore()
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	original := store.Item{
		Namespace: testNamespace,
		Name:      testKey,
		Value:     "enc-val",
		Encrypted: true,
		Version:   5,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	_ = src.Restore(context.Background(), &original)

	s3c := newFakeS3()
	s3Key := dumpAndGetKey(t, src, s3c)

	dst := newFakeStore()
	result, err := Restore(context.Background(), dst, s3c, RestoreOptions{Bucket: testBucket, Key: s3Key})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Restored != 1 {
		t.Errorf("restored: want 1, got %d", result.Restored)
	}

	got, err := dst.Get(context.Background(), testNamespace, testKey)
	if err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
	if got.Version != original.Version {
		t.Errorf("version: want %d, got %d", original.Version, got.Version)
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("created_at: want %v, got %v", original.CreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("updated_at: want %v, got %v", original.UpdatedAt, got.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var _ store.Store = (*fakeStore)(nil)
var _ S3Client = (*fakeS3)(nil)
