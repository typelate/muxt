package cli

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/docs"
)

const documentationCommandName = "documentation"

// pseudoVersionCommit matches the 12-digit commit hash at the end of a Go
// pseudo-version, such as v0.22.1-0.20261105181332-775c5f71f012.
var pseudoVersionCommit = regexp.MustCompile(`\d{14}-([0-9a-f]{12})$`)

// releaseVersion matches a release or development version such as v1.2.3 or
// v0.22.0-dev.4.
var releaseVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// sourceRef returns the GitHub ref of the source a build with version was made
// from: the commit of a pseudo-version, the version itself for a release, or
// "main" when neither applies, such as for a dirty or untagged build.
func sourceRef(version string) string {
	if m := pseudoVersionCommit.FindStringSubmatch(version); m != nil {
		return m[1]
	}
	if releaseVersion.MatchString(version) {
		return version
	}
	return "main"
}

// documentationCommand prints the docs embedded in the binary, so a project
// that has only the installed muxt can read the rules without its source.
// Links that leave docs point to the source of the build's version on GitHub.
// With --query it prints the lines that hold the query instead.
func documentationCommand(version func() (string, bool)) *cobra.Command {
	var query string
	c := &cobra.Command{
		Use:     documentationCommandName + " [path]",
		Aliases: []string{"docs", "d"},
		Short:   "Print muxt's documentation by path (no path lists every file)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			var arg string
			if len(args) == 1 {
				arg = args[0]
			}
			entry, err := docs.Resolve(arg)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("query") {
				if query == "" {
					return errors.New("--query needs text to search for")
				}
				matches, err := docs.Search(entry, query)
				if err != nil {
					return err
				}
				if len(matches) == 0 {
					return fmt.Errorf("no match for %q under %s", query, entry.Path)
				}
				for _, m := range matches {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:%d: %s\n", m.Path, m.Line, m.Text); err != nil {
						return fmt.Errorf("writing match: %w", err)
					}
				}
				return nil
			}
			v, _ := version()
			content, err := entry.Render(sourceRef(v))
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}
	c.Flags().StringVarP(&query, "query", "q", "", "print the lines that hold this text, as path:line: text")
	return c
}
