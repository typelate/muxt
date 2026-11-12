package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/muxtjson"
	"github.com/typelate/muxt/internal/scaffold"
)

// generateDefinitionCommand is muxt generate def NAME: it adds a route template
// to a template file of the package in the working directory. It never edits
// Go source.
func generateDefinitionCommand(workingDirectory *string, getEnv func(string) string, loadPackages packageLoader) *cobra.Command {
	var (
		templateFile string
		fragment     bool
		document     bool
	)
	cmd := &cobra.Command{
		Use:     "def NAME",
		Aliases: []string{"definition"},
		Short:   "Add a route template to a template file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			body := scaffold.Fragment
			if document {
				body = scaffold.Document
			}
			return scaffoldDefinition(cmd, *workingDirectory, args[0], templateFile, body, getEnv, loadPackages)
		},
	}
	cmd.Flags().StringVar(&templateFile, outputFile, "", "the .gohtml file in the working directory to add the template to")
	cmd.Flags().BoolVar(&fragment, outputFragment, false, "make the template an empty fragment (the default)")
	cmd.Flags().BoolVar(&document, outputDocument, false, "make the template a full HTML document")
	return cmd
}

// scaffoldDefinition appends the define for name to templateFile in wd and
// reports it on stderr. Everything is checked before anything is written.
func scaffoldDefinition(cmd *cobra.Command, wd, name, templateFile string, body scaffold.Body, getEnv func(string) string, loadPackages packageLoader) error {
	if err := checkTemplateFileName(templateFile); err != nil {
		return err
	}
	if err := muxt.CheckDefinitionName(name); err != nil {
		return err
	}
	if loadPackages != nil {
		if err := checkNotDefined(wd, name, getEnv, loadPackages); err != nil {
			return err
		}
	}
	path := filepath.Join(wd, templateFile)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", templateFile, err)
	}
	content := scaffold.AppendDefinition(existing, scaffold.Definition(name, body))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", templateFile, err)
	}
	_, err = fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s: %s\n", templateFile, name)
	return err
}

// checkNotDefined loads the package in wd and returns a
// *scaffold.DuplicateDefinitionError when its templates already define name.
// A package it cannot load, or one with no templates variable, is an error
// too: the check would otherwise pass for want of anything to check.
func checkNotDefined(wd, name string, getEnv func(string) string, loadPackages packageLoader) error {
	noVersion := func() (string, bool) { return "", false }
	config, err := entryConfiguration(muxtjson.Entry{}, wd, getEnv, noVersion)
	if err != nil {
		return err
	}
	cannotCheck := func(err error) error {
		return fmt.Errorf("cannot check whether %q is already defined: %w", name, err)
	}
	_, pl, err := loadPackages(wd, []string{wd}, nil)
	if err != nil {
		return cannotCheck(err)
	}
	pkg, err := load.Package(wd, pl, config.TemplatesVariables)
	if err != nil {
		return cannotCheck(err)
	}
	for _, variable := range pkg.Variables {
		if variable.Set.Lookup(name) == nil {
			continue
		}
		duplicate := &scaffold.DuplicateDefinitionError{Name: name}
		if position, ok := variable.NamePosition(name); ok {
			duplicate.File, duplicate.Line = relativeTo(wd, position.Filename), position.Line
		}
		return duplicate
	}
	return nil
}

// relativeTo returns file relative to dir. The go command may report a path
// with the symbolic links of a temporary directory resolved, so a file that
// does not read as below dir is tried against the resolved dir too.
func relativeTo(dir, file string) string {
	if rel, err := filepath.Rel(dir, file); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		if rel, err := filepath.Rel(real, file); err == nil && filepath.IsLocal(rel) {
			return rel
		}
	}
	return file
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
