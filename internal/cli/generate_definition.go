package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/scaffold"
)

// generateDefinitionCommand is muxt generate def NAME: it adds a route template
// to a template file of the package in the working directory. It never edits
// Go source.
func generateDefinitionCommand(workingDirectory *string) *cobra.Command {
	var templateFile string
	cmd := &cobra.Command{
		Use:     "def NAME",
		Aliases: []string{"definition"},
		Short:   "Add a route template to a template file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return scaffoldDefinition(cmd, *workingDirectory, args[0], templateFile)
		},
	}
	cmd.Flags().StringVar(&templateFile, outputFile, "", "the .gohtml file in the working directory to add the template to")
	return cmd
}

// scaffoldDefinition appends the define for name to templateFile in wd and
// reports it on stderr.
func scaffoldDefinition(cmd *cobra.Command, wd, name, templateFile string) error {
	if err := checkTemplateFileName(templateFile); err != nil {
		return err
	}
	path := filepath.Join(wd, templateFile)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", templateFile, err)
	}
	content := scaffold.AppendDefinition(existing, scaffold.Definition(name))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", templateFile, err)
	}
	_, err = fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s: %s\n", templateFile, name)
	return err
}

// checkTemplateFileName rejects an --output-file that is not a .gohtml file in
// the working directory.
func checkTemplateFileName(name string) error {
	const extension = ".gohtml"
	if name == "" {
		return fmt.Errorf("--%s is required: the %s file to add the template to", outputFile, extension)
	}
	if strings.ContainsAny(name, `/\`) || filepath.Dir(name) != "." {
		return fmt.Errorf("--%s must be a file name in the working directory: %s", outputFile, name)
	}
	if filepath.Ext(name) != extension || name == extension {
		return fmt.Errorf("--%s must be a file name ending in %s: %s", outputFile, extension, name)
	}
	return nil
}
