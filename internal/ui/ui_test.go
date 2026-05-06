package ui

import (
	"testing"
)

func TestResolveMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		requested   string
		stdoutTTY   bool
		stderrTTY   bool
		noColor     bool
		wantMode    string
		wantInterac bool
		wantErr     bool
	}{
		// auto — stdout TTY, no stderr TTY, no NO_COLOR → rich+interactive
		{name: "auto stdout-tty rich", requested: "auto", stdoutTTY: true, stderrTTY: false, noColor: false, wantMode: ModeRich, wantInterac: true},
		// auto — no TTY → plain
		{name: "auto no-tty plain", requested: "auto", stdoutTTY: false, stderrTTY: false, noColor: false, wantMode: ModePlain, wantInterac: false},
		// auto — NO_COLOR set → plain even with TTY
		{name: "auto nocolor", requested: "auto", stdoutTTY: true, stderrTTY: true, noColor: true, wantMode: ModePlain, wantInterac: true},
		// auto — stderr TTY counts as interactive
		{name: "auto stderr tty", requested: "auto", stdoutTTY: false, stderrTTY: true, noColor: false, wantMode: ModeRich, wantInterac: true},
		// empty string treated as auto, no TTY → plain
		{name: "empty as auto", requested: "", stdoutTTY: false, stderrTTY: false, noColor: false, wantMode: ModePlain, wantInterac: false},
		// plain always returns plain
		{name: "plain explicit", requested: "plain", stdoutTTY: true, stderrTTY: true, noColor: false, wantMode: ModePlain, wantInterac: true},
		// rich — no NO_COLOR → rich; explicit mode always sets interactive=true regardless of TTY
		{name: "rich explicit", requested: "rich", stdoutTTY: false, stderrTTY: false, noColor: false, wantMode: ModeRich, wantInterac: true},
		// rich — NO_COLOR set → downgrade to plain
		{name: "rich nocolor", requested: "rich", stdoutTTY: false, stderrTTY: false, noColor: true, wantMode: ModePlain, wantInterac: true},
		// case-insensitive
		{name: "uppercase PLAIN", requested: "PLAIN", stdoutTTY: false, stderrTTY: false, noColor: false, wantMode: ModePlain, wantInterac: true},
		// unknown value → error
		{name: "unknown mode", requested: "fancy", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mode, interactive, err := ResolveMode(tc.requested, tc.stdoutTTY, tc.stderrTTY, tc.noColor)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got mode=%q interactive=%v", mode, interactive)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tc.wantMode {
				t.Errorf("mode: got %q, want %q", mode, tc.wantMode)
			}
			if interactive != tc.wantInterac {
				t.Errorf("interactive: got %v, want %v", interactive, tc.wantInterac)
			}
		})
	}
}

func TestNoColor(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty — color enabled", value: "", want: false},
		{name: "zero — color enabled", value: "0", want: false},
		{name: "false — color enabled", value: "false", want: false},
		{name: "FALSE — color enabled", value: "FALSE", want: false},
		{name: "1 — color disabled", value: "1", want: true},
		{name: "true — color disabled", value: "true", want: true},
		{name: "any non-empty non-zero — color disabled", value: "yes", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.value)
			got := noColor()
			if got != tc.want {
				t.Errorf("noColor() = %v, want %v (NO_COLOR=%q)", got, tc.want, tc.value)
			}
		})
	}
}
