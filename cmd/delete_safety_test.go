package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ffreis/dynamoctl/internal/store"
)

// TestDeleteCmdWithoutForce_BlocksOnMissingKey is the missing guardrail test
// for dynamoctl's most dangerous operation. Per AGENTS.md this tool is used
// to clear stale Terraform lock items; deleting the wrong key (or a key that
// has already been cleared by someone else) at the wrong time can interfere
// with a concurrent `terraform apply`.
//
// The default codepath (no --force) is supposed to call Get first and refuse
// to call Delete when the key does not exist. This test asserts both halves
// of that contract: Delete is NOT called, and the user gets a clear "not
// found" message rather than a silent success.
func TestDeleteCmdWithoutForceBlocksOnMissingKey(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	deleteCalled := false
	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			getFunc: func(_ context.Context, _, _ string) (*store.Item, error) {
				return nil, store.ErrNotFound
			},
			deleteFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newDeleteCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"missing-lock-id"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute on missing key without --force: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not mention 'not found'", err.Error())
	}
	if deleteCalled {
		t.Fatal("Delete was called even though the key did not exist (without --force)")
	}
}

// TestDeleteCmdWithoutForce_DeletesWhenPresent ensures the precheck path also
// works in the happy case: a key that exists is deleted, and the user gets
// the standard success output.
func TestDeleteCmdWithoutForceDeletesWhenPresent(t *testing.T) {
	origStoreFactory := storeFactory
	defer func() { storeFactory = origStoreFactory }()

	flagNamespace = "prod"
	flagJSON = false

	getCalled, deleteCalled := false, false
	storeFactory = func(context.Context) (store.Store, error) {
		return &fakeStore{
			getFunc: func(_ context.Context, namespace, name string) (*store.Item, error) {
				getCalled = true
				return &store.Item{
					Namespace: namespace,
					Name:      name,
					Version:   1,
					UpdatedAt: time.Unix(0, 0).UTC(),
				}, nil
			},
			deleteFunc: func(_ context.Context, namespace, name string) error {
				deleteCalled = true
				if namespace != "prod" || name != "real-lock" {
					t.Errorf("Delete called with namespace=%q name=%q, want prod/real-lock", namespace, name)
				}
				return nil
			},
		}, nil
	}

	var out bytes.Buffer
	cmd := newDeleteCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"real-lock"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !getCalled {
		t.Error("precheck (Get) was not called without --force")
	}
	if !deleteCalled {
		t.Error("Delete was not called after successful precheck")
	}
	if !strings.Contains(out.String(), "deleted") {
		t.Errorf("expected success output containing 'deleted', got %q", out.String())
	}
}
