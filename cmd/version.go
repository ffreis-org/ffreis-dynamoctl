package cmd

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := strings.TrimSpace(version)
			if v == "" {
				v = "dev"
			}
			c := strings.TrimSpace(commit)
			if c == "" {
				c = "unknown"
			}
			t := strings.TrimSpace(buildTime)
			if t == "" {
				t = "unknown"
			}

			if currentOutput() == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"version":    v,
					"commit":     c,
					"build_time": t,
				})
			}
			_, err := cmd.OutOrStdout().Write([]byte(v + " (commit=" + c + " built=" + t + ")\n"))
			return err
		},
	}
}
