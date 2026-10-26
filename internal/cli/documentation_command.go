package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/docs"
)

const documentationCommandName = "documentation"

// documentationCommand prints the docs embedded in the binary, so a project
// that has only the installed muxt can read the rules without its source.
func documentationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     documentationCommandName + " [topic]",
		Aliases: []string{"docs"},
		Short:   "Print muxt's documentation (no topic lists them)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := checkFormat(cmd); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				return writeDocumentationIndex(cmd, out)
			}
			topic, _, err := docs.Find(args[0])
			if err != nil {
				return err
			}
			content, err := topic.Content()
			if err != nil {
				return err
			}
			_, err = out.Write(content)
			return err
		},
	}
	cmd.Flags().String("format", "text", "index output format (text or json)")
	return cmd
}

func writeDocumentationIndex(cmd *cobra.Command, w io.Writer) error {
	topics := docs.Topics()
	if format, _ := cmd.Flags().GetString("format"); format == "json" {
		buf, err := json.MarshalIndent(topics, "", "\t")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s\n", buf)
		return err
	}
	for _, section := range docs.Sections {
		_, _ = fmt.Fprintf(w, "%s\n", section)
		for _, t := range topics {
			if t.Section == section {
				_, _ = fmt.Fprintf(w, "  %s — %s\n", t.ID, t.Title)
			}
		}
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, "Run `muxt documentation <topic>` to print a page.")
	_, err := fmt.Fprintln(w, "Links inside pages are relative to the muxt repository's docs/ directory, not URLs; open them with `muxt documentation <topic>`.")
	return err
}
