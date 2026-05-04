package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestExecuteRunsVersionAndSetsLogger(t *testing.T) {
	prevLogLevel := flagLogLevel
	prevJSON := flagJSON
	prevOutput := flagOutput
	defer func() {
		flagLogLevel = prevLogLevel
		flagJSON = prevJSON
		flagOutput = prevOutput
	}()

	flagLogLevel = "debug"
	flagJSON = false
	flagOutput = "text"

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"version"})

	if code := Execute(); code != exitOK {
		t.Fatalf("Execute() code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "commit=") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestEnvOrDefault(t *testing.T) {
	const k = "DYNAMOCTL_TEST_ENV"
	_ = os.Unsetenv(k)
	if got := envOrDefault(k, "def"); got != "def" {
		t.Fatalf("want default, got %q", got)
	}

	t.Setenv(k, "x")
	if got := envOrDefault(k, "def"); got != "x" {
		t.Fatalf("want env value, got %q", got)
	}
}
