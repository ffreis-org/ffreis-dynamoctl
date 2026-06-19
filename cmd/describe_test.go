package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestDescribeCmdFactoryErrorPropagates(t *testing.T) {
	orig := ddbClientFactory
	defer func() { ddbClientFactory = orig }()

	flagTable = "my-table"
	flagJSON = false

	wantErr := errors.New("no credentials")
	ddbClientFactory = func(_ context.Context) (*awsdynamodb.Client, error) {
		return nil, wantErr
	}

	var out bytes.Buffer
	cmd := newDescribeCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from factory failure, got nil")
	}
	if !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDescribeCmdUsesGlobalFlagTable(t *testing.T) {
	orig := ddbClientFactory
	defer func() { ddbClientFactory = orig }()

	// flagTable is a persistent flag on root; set it directly as all other
	// cmd tests do when testing subcommands in isolation.
	flagTable = "target-table"
	flagJSON = false

	var capturedTable string
	ddbClientFactory = func(_ context.Context) (*awsdynamodb.Client, error) {
		capturedTable = flagTable
		return nil, errors.New("stop early")
	}

	var out bytes.Buffer
	cmd := newDescribeCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	_ = cmd.Execute()

	if capturedTable != "target-table" {
		t.Fatalf("flagTable: want %q, got %q", "target-table", capturedTable)
	}
}
