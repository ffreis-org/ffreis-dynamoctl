package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ffreis/dynamoctl/internal/backup"
	"github.com/ffreis/dynamoctl/internal/crypto"
	"github.com/ffreis/dynamoctl/internal/store"
)

type fakeStore struct {
	putFunc             func(ctx context.Context, item *store.Item) error
	getFunc             func(ctx context.Context, namespace, name string) (*store.Item, error)
	listFunc            func(ctx context.Context, namespace string) ([]store.Item, error)
	deleteFunc          func(ctx context.Context, namespace, name string) error
	scanNamespaceFunc   func(ctx context.Context, namespace string) ([]store.Item, error)
	scanAllFunc         func(ctx context.Context) ([]store.Item, error)
	updateEncryptedFunc func(ctx context.Context, namespace, name, newValue string, expectedVersion int) error
	restoreFunc         func(ctx context.Context, item *store.Item) error
}

func (f *fakeStore) Put(ctx context.Context, item *store.Item) error {
	if f.putFunc == nil {
		panic("Put not implemented")
	}
	return f.putFunc(ctx, item)
}

func (f *fakeStore) Get(ctx context.Context, namespace, name string) (*store.Item, error) {
	if f.getFunc == nil {
		panic("Get not implemented")
	}
	return f.getFunc(ctx, namespace, name)
}

func (f *fakeStore) List(ctx context.Context, namespace string) ([]store.Item, error) {
	if f.listFunc == nil {
		panic("List not implemented")
	}
	return f.listFunc(ctx, namespace)
}

func (f *fakeStore) Delete(ctx context.Context, namespace, name string) error {
	if f.deleteFunc == nil {
		panic("Delete not implemented")
	}
	return f.deleteFunc(ctx, namespace, name)
}

func (f *fakeStore) ScanNamespace(ctx context.Context, namespace string) ([]store.Item, error) {
	if f.scanNamespaceFunc == nil {
		panic("ScanNamespace not implemented")
	}
	return f.scanNamespaceFunc(ctx, namespace)
}

func (f *fakeStore) ScanAll(ctx context.Context) ([]store.Item, error) {
	if f.scanAllFunc == nil {
		panic("ScanAll not implemented")
	}
	return f.scanAllFunc(ctx)
}

func (f *fakeStore) UpdateEncrypted(ctx context.Context, namespace, name, newValue string, expectedVersion int) error {
	if f.updateEncryptedFunc == nil {
		panic("UpdateEncrypted not implemented")
	}
	return f.updateEncryptedFunc(ctx, namespace, name, newValue, expectedVersion)
}

func (f *fakeStore) Restore(ctx context.Context, item *store.Item) error {
	if f.restoreFunc == nil {
		panic("Restore not implemented")
	}
	return f.restoreFunc(ctx, item)
}

type memS3 struct {
	puts map[string][]byte
}

func newMemS3() *memS3 {
	return &memS3{puts: make(map[string][]byte)}
}

const (
	errFmtExecute       = "Execute: %v"
	errFmtUnexpectedOut = "unexpected output: %q"
)

func (m *memS3) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, _ := io.ReadAll(params.Body)
	m.puts[*params.Key] = body
	return &s3.PutObjectOutput{}, nil
}

func (m *memS3) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, ok := m.puts[*params.Key]
	if !ok {
		return nil, errors.New("no such key")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func TestStoreAndGetVersion(t *testing.T) {
	st := &fakeStore{
		putFunc: func(_ context.Context, item *store.Item) error {
			if item == nil {
				return errors.New("nil item")
			}
			return nil
		},
		getFunc: func(_ context.Context, namespace, name string) (*store.Item, error) {
			return &store.Item{Namespace: namespace, Name: name, Version: 7}, nil
		},
	}

	item := &store.Item{Namespace: "ns", Name: "k", Value: "v"}
	ver, err := storeAndGetVersion(context.Background(), st, item)
	if err != nil {
		t.Fatalf("storeAndGetVersion: %v", err)
	}
	if ver != 7 {
		t.Fatalf("version: want 7, got %d", ver)
	}
}

func TestListCmdExecute(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			listFunc: func(_ context.Context, namespace string) ([]store.Item, error) {
				if namespace != "prod" {
					return nil, errors.New("wrong namespace")
				}
				now := time.Unix(0, 0).UTC()
				return []store.Item{
					{Namespace: namespace, Name: "a", Version: 1, UpdatedAt: now},
					{Namespace: namespace, Name: "b", Version: 2, UpdatedAt: now},
				}, nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newListCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if !strings.Contains(out.String(), "NAMESPACE") {
		t.Fatalf("expected table output, got: %q", out.String())
	}
}

func TestDeleteCmdForceSkipsPrecheck(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	deleted := false
	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			deleteFunc: func(_ context.Context, namespace, name string) error {
				deleted = true
				if namespace != "prod" || name != "k" {
					return errors.New("wrong key")
				}
				return nil
			},
			getFunc: func(context.Context, string, string) (*store.Item, error) {
				t.Fatal("Get should not be called when --force is set")
				return nil, nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newDeleteCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--force", "k"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if !deleted {
		t.Fatal("expected Delete to be called")
	}
	if !strings.Contains(out.String(), "deleted prod/k") {
		t.Fatalf(errFmtUnexpectedOut, out.String())
	}
}

func TestGetCmdDecryptsEncryptedItem(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	k, _ := crypto.GenerateKey()
	keyHex := crypto.FormatKey(k)
	flagEncryptionKey = keyHex

	ciphertext, err := crypto.Encrypt([]byte("hello"), k)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			getFunc: func(_ context.Context, namespace, name string) (*store.Item, error) {
				return &store.Item{
					Namespace: namespace,
					Name:      name,
					Value:     ciphertext,
					Encrypted: true,
					Version:   1,
					UpdatedAt: time.Unix(0, 0).UTC(),
				}, nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newGetCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"api-key"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if strings.TrimSpace(out.String()) != "hello" {
		t.Fatalf("expected decrypted output, got: %q", out.String())
	}
}

func TestSetCmdNoEncrypt(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	var stored store.Item
	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			putFunc: func(_ context.Context, item *store.Item) error {
				stored = *item
				return nil
			},
			getFunc: func(_ context.Context, namespace, name string) (*store.Item, error) {
				return &store.Item{Namespace: namespace, Name: name, Version: 3}, nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newSetCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--no-encrypt", "k", "v"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if stored.Value != "v" || stored.Encrypted {
		t.Fatalf("unexpected stored item: %+v", stored)
	}
	if !strings.Contains(out.String(), "set prod/k (version 3)") {
		t.Fatalf(errFmtUnexpectedOut, out.String())
	}
}

func TestRotateCmdRotatesSingleItem(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	oldKey, _ := crypto.GenerateKey()
	newKey, _ := crypto.GenerateKey()
	flagEncryptionKey = crypto.FormatKey(oldKey)

	ciphertext, err := crypto.Encrypt([]byte("hello"), oldKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	updateCalls := 0
	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			listFunc: func(_ context.Context, namespace string) ([]store.Item, error) {
				return []store.Item{
					{Namespace: namespace, Name: "k", Value: ciphertext, Encrypted: true, Version: 1},
				}, nil
			},
			updateEncryptedFunc: func(_ context.Context, namespace, name, newValue string, expectedVersion int) error {
				updateCalls++
				if namespace != "prod" || name != "k" || expectedVersion != 1 {
					return errors.New("unexpected update args")
				}
				plain, err := crypto.Decrypt(newValue, newKey)
				if err != nil {
					return err
				}
				if string(plain) != "hello" {
					return errors.New("not re-encrypted correctly")
				}
				return nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newRotateCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--new-key", crypto.FormatKey(newKey)})

	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if updateCalls != 1 {
		t.Fatalf("expected 1 update call, got %d", updateCalls)
	}
	if !strings.Contains(out.String(), "rotated 1") {
		t.Fatalf(errFmtUnexpectedOut, out.String())
	}
}

func TestBackupAndRestoreCmdsSuccess(t *testing.T) {
	origStoreFactory := storeFactory
	origS3Factory := s3ClientFactory
	defer func() {
		storeFactory = origStoreFactory
		s3ClientFactory = origS3Factory
	}()

	flagTable = "tbl"
	flagNamespace = "prod"
	flagJSON = false

	s3c := newMemS3()

	var restored int
	st := &fakeStore{
		scanNamespaceFunc: func(_ context.Context, namespace string) ([]store.Item, error) {
			now := time.Unix(0, 0).UTC()
			return []store.Item{
				{Namespace: namespace, Name: "a", Value: "x", Version: 1, CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		scanAllFunc: func(context.Context) ([]store.Item, error) {
			return nil, errors.New("ScanAll should not be called when namespace is set")
		},
		restoreFunc: func(_ context.Context, item *store.Item) error {
			restored++
			return nil
		},
	}

	storeFactory = func(context.Context) (store.Store, error) { return st, nil }
	s3ClientFactory = func(context.Context) (backup.S3Client, error) { return s3c, nil }

	// Backup.
	var backupOut bytes.Buffer
	backupCmd := newBackupCmd()
	backupCmd.SetOut(&backupOut)
	backupCmd.SetErr(&backupOut)
	backupCmd.SetArgs([]string{"--bucket", "bkt"})

	if err := backupCmd.Execute(); err != nil {
		t.Fatalf("backup Execute: %v", err)
	}
	if !strings.Contains(backupOut.String(), "backup complete: s3://bkt/") {
		t.Fatalf(errFmtUnexpectedOut, backupOut.String())
	}

	// Find the generated key so restore can read it.
	var dumpedKey string
	for k := range s3c.puts {
		dumpedKey = k
	}
	if dumpedKey == "" {
		t.Fatal("expected S3 PutObject to be called")
	}

	// Restore.
	var restoreOut bytes.Buffer
	restoreCmd := newRestoreCmd()
	restoreCmd.SetOut(&restoreOut)
	restoreCmd.SetErr(&restoreOut)
	restoreCmd.SetArgs([]string{"--bucket", "bkt", "--key", dumpedKey, "--overwrite"})

	if err := restoreCmd.Execute(); err != nil {
		t.Fatalf("restore "+errFmtExecute, err)
	}
	if restored != 1 {
		t.Fatalf("expected 1 restored item, got %d", restored)
	}
	if !strings.Contains(restoreOut.String(), "restore complete: 1 restored") {
		t.Fatalf(errFmtUnexpectedOut, restoreOut.String())
	}
}

func TestVersionCmdTextAndJSON(t *testing.T) {
	flagJSON = false
	var out bytes.Buffer
	cmd := newVersionCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if !strings.Contains(out.String(), "commit=") {
		t.Fatalf(errFmtUnexpectedOut, out.String())
	}

	flagJSON = true
	out.Reset()
	cmd = newVersionCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf(errFmtExecute, err)
	}
	if !strings.Contains(out.String(), "\"version\"") {
		t.Fatalf("unexpected JSON output: %q", out.String())
	}
}
