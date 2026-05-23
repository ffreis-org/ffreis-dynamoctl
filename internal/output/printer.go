// Package output formats command results for human or machine consumption.
// All methods write to the provided io.Writer.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ffreis/dynamoctl/internal/store"
	"github.com/ffreis/dynamoctl/internal/ui"
)

// Printer writes command output in either JSON or human-readable text format.
type Printer struct {
	W      io.Writer
	Format string
	UI     *ui.Presenter
}

// New returns a Printer configured to write to w.
func New(w io.Writer, format string, presenter *ui.Presenter) *Printer {
	return &Printer{W: w, Format: format, UI: presenter}
}

func (p *Printer) isJSON() bool {
	return p != nil && p.Format == "json"
}

func (p *Printer) status(kind, label, detail string) error {
	if p != nil && p.UI != nil {
		_, err := fmt.Fprintln(p.W, p.UI.Status(kind, label, detail))
		return err
	}
	_, err := fmt.Fprintf(p.W, "[%s] %s\n", label, detail)
	return err
}

// --- set / delete / generic success ---

// PrintSetResult reports a successful set operation.
func (p *Printer) PrintSetResult(namespace, name string, version int) error {
	if p.isJSON() {
		return p.writeJSON(map[string]any{
			jsonKeyAction:    actionSet,
			jsonKeyNamespace: namespace,
			jsonKeyName:      name,
			jsonKeyVersion:   version,
		})
	}
	return p.status("ok", "ok", fmt.Sprintf("set %s/%s (version %d)", namespace, name, version))
}

// PrintDeleteResult reports a successful delete operation.
func (p *Printer) PrintDeleteResult(namespace, name string) error {
	if p.isJSON() {
		return p.writeJSON(map[string]any{
			jsonKeyAction:    actionDeleted,
			jsonKeyNamespace: namespace,
			jsonKeyName:      name,
		})
	}
	return p.status("ok", "ok", fmt.Sprintf("deleted %s/%s", namespace, name))
}

// --- get ---

// GetResult is the payload returned by PrintGetResult.
type GetResult struct {
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Encrypted bool      `json:"encrypted"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PrintGetResult prints a retrieved (and possibly decrypted) item.
func (p *Printer) PrintGetResult(item *store.Item, decryptedValue string) error {
	val := item.Value
	if decryptedValue != "" {
		val = decryptedValue
	}
	if p.isJSON() {
		return p.writeJSON(GetResult{
			Namespace: item.Namespace,
			Name:      item.Name,
			Value:     val,
			Encrypted: item.Encrypted,
			Version:   item.Version,
			UpdatedAt: item.UpdatedAt,
		})
	}
	_, err := fmt.Fprintln(p.W, val)
	return err
}

// --- list ---

// ItemView is the per-item representation in list output (value omitted).
type ItemView struct {
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Encrypted bool      `json:"encrypted"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PrintListResult prints a list of items.
func (p *Printer) PrintListResult(items []store.Item) error {
	if p.isJSON() {
		views := make([]ItemView, len(items))
		for i, it := range items {
			views[i] = ItemView{
				Namespace: it.Namespace,
				Name:      it.Name,
				Encrypted: it.Encrypted,
				Version:   it.Version,
				UpdatedAt: it.UpdatedAt,
			}
		}
		return p.writeJSON(views)
	}

	if len(items) == 0 {
		return p.status("muted", "skip", "no items")
	}

	tw := tabwriter.NewWriter(p.W, 0, 0, 2, ' ', 0)
	// Writes to a buffered tabwriter — errors surface on Flush at the end.
	_, _ = fmt.Fprintln(tw, "NAMESPACE\tNAME\tENCRYPTED\tVERSION\tUPDATED")
	for _, it := range items {
		enc := "no"
		if it.Encrypted {
			enc = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			it.Namespace, it.Name, enc, it.Version,
			it.UpdatedAt.Format(time.RFC3339),
		)
	}
	return tw.Flush()
}

// --- rotate ---

// PrintRotateResult reports the outcome of a key rotation.
func (p *Printer) PrintRotateResult(namespace string, rotated, skipped, failed int) error {
	if p.isJSON() {
		return p.writeJSON(map[string]any{
			jsonKeyAction:    actionRotate,
			jsonKeyNamespace: namespace,
			jsonKeyRotated:   rotated,
			jsonKeySkipped:   skipped,
			jsonKeyFailed:    failed,
		})
	}
	parts := []string{
		fmt.Sprintf("rotated %d", rotated),
		fmt.Sprintf("skipped %d plaintext", skipped),
		fmt.Sprintf("failed %d", failed),
	}
	return p.status("info", "ok", strings.Join(parts, "  "))
}

// --- backup ---

// PrintBackupResult reports a completed backup.
func (p *Printer) PrintBackupResult(s3URI string, count int) error {
	if p.isJSON() {
		return p.writeJSON(map[string]any{
			jsonKeyAction:    actionBackup,
			jsonKeyS3URI:     s3URI,
			jsonKeyItemCount: count,
		})
	}
	return p.status("ok", "ok", fmt.Sprintf("backup complete: %s (%d items)", s3URI, count))
}

// --- restore ---

// PrintRestoreResult reports a completed restore.
func (p *Printer) PrintRestoreResult(restored, skipped int, errs []string) error {
	if p.isJSON() {
		return p.writeJSON(map[string]any{
			jsonKeyAction:   actionRestore,
			jsonKeyRestored: restored,
			jsonKeySkipped:  skipped,
			jsonKeyErrors:   errs,
		})
	}
	return p.status("ok", "ok", fmt.Sprintf("restore complete: %d restored, %d skipped, %d errors",
		restored, skipped, len(errs)))
}

// --- helpers ---

func (p *Printer) writeJSON(v any) error {
	enc := json.NewEncoder(p.W)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
