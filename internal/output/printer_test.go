package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ffreis/dynamoctl/internal/store"
)

const (
	testKeyAPIKey     = "api-key"
	actionWantGotFmt  = "action: want %s, got %v"
	wantProdAPIKeyFmt = "prod/" + testKeyAPIKey
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func textPrinter(buf *bytes.Buffer) *Printer { return New(buf, "text", nil) }
func jsonPrinter(buf *bytes.Buffer) *Printer { return New(buf, "json", nil) }

func decodeJSON(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	if err := json.Unmarshal(buf.Bytes(), v); err != nil {
		t.Fatalf("decode JSON %q: %v", buf.String(), err)
	}
}

// ---------------------------------------------------------------------------
// PrintSetResult
// ---------------------------------------------------------------------------

func TestPrintSetResultText(t *testing.T) {
	var buf bytes.Buffer
	if err := textPrinter(&buf).PrintSetResult(testNamespaceProd, testKeyAPIKey, 3); err != nil {
		t.Fatalf("PrintSetResult: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, wantProdAPIKeyFmt) {
		t.Errorf("want %q in output, got %q", wantProdAPIKeyFmt, got)
	}
	if !strings.Contains(got, "3") {
		t.Errorf("want version 3 in output, got %q", got)
	}
}

func TestPrintSetResultJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := jsonPrinter(&buf).PrintSetResult(testNamespaceProd, testKeyAPIKey, 3); err != nil {
		t.Fatalf("PrintSetResult JSON: %v", err)
	}
	var m map[string]any
	decodeJSON(t, &buf, &m)
	if m[jsonKeyAction] != actionSet {
		t.Errorf(actionWantGotFmt, actionSet, m[jsonKeyAction])
	}
	if m[jsonKeyNamespace] != testNamespaceProd {
		t.Errorf("namespace: want %s, got %v", testNamespaceProd, m[jsonKeyNamespace])
	}
}

// ---------------------------------------------------------------------------
// PrintDeleteResult
// ---------------------------------------------------------------------------

func TestPrintDeleteResultText(t *testing.T) {
	var buf bytes.Buffer
	_ = textPrinter(&buf).PrintDeleteResult("default", "mykey")
	if !strings.Contains(buf.String(), "default/mykey") {
		t.Errorf("want 'default/mykey', got %q", buf.String())
	}
}

func TestPrintDeleteResultJSON(t *testing.T) {
	var buf bytes.Buffer
	_ = jsonPrinter(&buf).PrintDeleteResult("default", "mykey")
	var m map[string]any
	decodeJSON(t, &buf, &m)
	if m[jsonKeyAction] != actionDeleted {
		t.Errorf(actionWantGotFmt, actionDeleted, m[jsonKeyAction])
	}
}

// ---------------------------------------------------------------------------
// PrintGetResult
// ---------------------------------------------------------------------------

func TestPrintGetResultTextPrintsValue(t *testing.T) {
	var buf bytes.Buffer
	item := &store.Item{Namespace: "ns", Name: "key", Value: "encrypted-blob", Encrypted: true}
	_ = textPrinter(&buf).PrintGetResult(item, "decrypted-secret")

	// Text mode should print only the decrypted value.
	got := strings.TrimSpace(buf.String())
	if got != "decrypted-secret" {
		t.Errorf("want 'decrypted-secret', got %q", got)
	}
}

func TestPrintGetResultTextUsesRawValueWhenNoDecryption(t *testing.T) {
	var buf bytes.Buffer
	item := &store.Item{Namespace: "ns", Name: "key", Value: "plaintext"}
	_ = textPrinter(&buf).PrintGetResult(item, "")

	got := strings.TrimSpace(buf.String())
	if got != "plaintext" {
		t.Errorf("want 'plaintext', got %q", got)
	}
}

func TestPrintGetResultJSON(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now().UTC()
	item := &store.Item{
		Namespace: "ns", Name: "key", Value: "enc", Encrypted: true, Version: 2, UpdatedAt: now,
	}
	_ = jsonPrinter(&buf).PrintGetResult(item, "plain")

	var r GetResult
	decodeJSON(t, &buf, &r)
	if r.Value != "plain" {
		t.Errorf("value: want plain, got %q", r.Value)
	}
	if r.Version != 2 {
		t.Errorf("version: want 2, got %d", r.Version)
	}
	if r.Encrypted != true {
		t.Error("encrypted: want true")
	}
}

// ---------------------------------------------------------------------------
// PrintListResult
// ---------------------------------------------------------------------------

func TestPrintListResultTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	_ = textPrinter(&buf).PrintListResult(nil)
	if !strings.Contains(buf.String(), "no items") {
		t.Errorf("want 'no items', got %q", buf.String())
	}
}

func TestPrintListResultTextShowsHeader(t *testing.T) {
	var buf bytes.Buffer
	items := []store.Item{
		{Namespace: testNamespaceProd, Name: "db-pass", Encrypted: true, Version: 5},
		{Namespace: testNamespaceProd, Name: testKeyAPIKey, Encrypted: false, Version: 1},
	}
	_ = textPrinter(&buf).PrintListResult(items)
	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Errorf("want header 'NAME', got %q", out)
	}
	if !strings.Contains(out, "db-pass") {
		t.Errorf("want 'db-pass', got %q", out)
	}
}

func TestPrintListResultJSONReturnsArray(t *testing.T) {
	var buf bytes.Buffer
	items := []store.Item{
		{Namespace: "ns", Name: "a", Encrypted: true, Version: 1},
		{Namespace: "ns", Name: "b", Encrypted: false, Version: 2},
	}
	_ = jsonPrinter(&buf).PrintListResult(items)

	var arr []ItemView
	decodeJSON(t, &buf, &arr)
	if len(arr) != 2 {
		t.Errorf("want 2 items, got %d", len(arr))
	}
	// Values must NOT appear in list output for security.
	for _, v := range arr {
		if v.Name == "" {
			t.Error("name should not be empty")
		}
	}
}

// ---------------------------------------------------------------------------
// PrintRotateResult
// ---------------------------------------------------------------------------

func TestPrintRotateResultText(t *testing.T) {
	var buf bytes.Buffer
	_ = textPrinter(&buf).PrintRotateResult(testNamespaceProd, 10, 3, 1)
	out := buf.String()
	if !strings.Contains(out, "10") {
		t.Errorf("want '10' rotated, got %q", out)
	}
}

func TestPrintRotateResultJSON(t *testing.T) {
	var buf bytes.Buffer
	_ = jsonPrinter(&buf).PrintRotateResult(testNamespaceProd, 10, 3, 1)
	var m map[string]any
	decodeJSON(t, &buf, &m)
	if m[jsonKeyAction] != actionRotate {
		t.Errorf(actionWantGotFmt, actionRotate, m[jsonKeyAction])
	}
	if int(m[jsonKeyRotated].(float64)) != 10 {
		t.Errorf("rotated: want 10, got %v", m[jsonKeyRotated])
	}
}

// ---------------------------------------------------------------------------
// PrintBackupResult
// ---------------------------------------------------------------------------

func TestPrintBackupResultText(t *testing.T) {
	var buf bytes.Buffer
	_ = textPrinter(&buf).PrintBackupResult("s3://bucket/key.json", 42)
	out := buf.String()
	if !strings.Contains(out, "s3://bucket/key.json") {
		t.Errorf("want s3 URI in output, got %q", out)
	}
}

func TestPrintBackupResultJSON(t *testing.T) {
	var buf bytes.Buffer
	_ = jsonPrinter(&buf).PrintBackupResult("s3://b/k", 7)
	var m map[string]any
	decodeJSON(t, &buf, &m)
	if m[jsonKeyS3URI] != "s3://b/k" {
		t.Errorf("s3_uri: want s3://b/k, got %v", m[jsonKeyS3URI])
	}
}

// ---------------------------------------------------------------------------
// PrintRestoreResult
// ---------------------------------------------------------------------------

func TestPrintRestoreResultText(t *testing.T) {
	var buf bytes.Buffer
	_ = textPrinter(&buf).PrintRestoreResult(5, 2, []string{"err1"})
	out := buf.String()
	if !strings.Contains(out, "5 restored") {
		t.Errorf("want '5 restored', got %q", out)
	}
}

func TestPrintRestoreResultJSONIncludesErrors(t *testing.T) {
	var buf bytes.Buffer
	_ = jsonPrinter(&buf).PrintRestoreResult(3, 0, []string{"failed x", "failed y"})
	var m map[string]any
	decodeJSON(t, &buf, &m)
	errs, ok := m[jsonKeyErrors].([]any)
	if !ok || len(errs) != 2 {
		t.Errorf("want 2 errors in JSON, got %v", m[jsonKeyErrors])
	}
}
