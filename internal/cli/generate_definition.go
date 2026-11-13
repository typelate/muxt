package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ettle/strcase"
	"github.com/spf13/cobra"
	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/template/html"
)

// generateDefinitionCommand is muxt generate def NAME: it adds a route template
// to a template file of the package in the working directory. It never edits
// Go source; a method the route calls and the receiver lacks is printed to
// stdout as a stub for the person to place.
func generateDefinitionCommand(workingDirectory *string, getEnv func(string) string, loadPackages packageLoader) *cobra.Command {
	var (
		templateFile string
		receiverType string
		fragment     bool
		document     bool
	)
	cmd := &cobra.Command{
		Use:     "def NAME",
		Aliases: []string{"definition"},
		Short:   "Add a route template to a template file",
		Long: `Add a route template to a template file.

The method stub it prints is a placeholder to rewrite: its result type is any, its body returns nil, and its receiver form follows what muxt infers. Imports are not added. Without --output-file it prints a txtar archive and writes nothing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if fragment && document {
				return fmt.Errorf("--%s and --%s are mutually exclusive", outputFragment, outputDocument)
			}
			body := generate.FragmentBody
			if document {
				body = generate.DocumentBody
			}
			spec := jobSpec{
				Name:  generateCommandName + " def",
				Scope: itsEntry,
				// The receiver type flag is the only setting def reads from the
				// command line, and what it gives an unconfigured package.
				Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
					config, err := defaultConfiguration(dir, getEnv)
					if err != nil {
						return generate.RoutesFileConfiguration{}, err
					}
					config.ReceiverType = receiverType
					return config, nil
				},
			}
			sel, err := selectJobs(spec, *workingDirectory, nil, getEnv)
			if err != nil {
				return err
			}
			job := sel.Jobs[0]
			config := job.Config
			if job.File != "" && cmd.Flags().Changed(useReceiverType) {
				// The flag is a second source for what an entry already says, so
				// in a configured directory it only fills a receiver type the
				// resolved entry, package_defaults included, leaves unnamed.
				if config.ReceiverType != "" {
					return &composite.ConfiguredDirectoryError{Path: job.File, Command: spec.Name, Flags: []string{useReceiverType}}
				}
				config.ReceiverType = receiverType
			}
			target := definitionTarget{Dir: *workingDirectory, Name: args[0], File: templateFile}
			return scaffoldDefinition(cmd, target, body, config, getEnv, loadPackages)
		},
	}
	cmd.Flags().StringVar(&templateFile, outputFile, "", "the .gohtml file in the working directory to add the template to")
	cmd.Flags().StringVar(&receiverType, useReceiverType, "", "the receiver type to print a method stub for, when no muxt.json entry names it")
	cmd.Flags().BoolVar(&fragment, outputFragment, false, "make the template an empty fragment (the default)")
	cmd.Flags().BoolVar(&document, outputDocument, false, "make the template a full HTML document")
	return cmd
}

// definitionTarget is where generate def puts a route template. The three
// fields are strings of one kind, so a struct keeps a call from swapping them.
type definitionTarget struct {
	// Dir is the package directory, the working directory of the command.
	Dir string
	// Name is the template name, the route the define declares.
	Name string
	// File is the .gohtml file name in Dir to append to; empty prints an
	// archive instead.
	File string
}

// scaffoldDefinition appends the define for target.Name to target.File in
// target.Dir, reports it on stderr, and prints the method stub, if any, to
// stdout. Everything is checked before anything is written. With no File it
// prints a txtar archive of the files it would write instead, and writes
// nothing.
func scaffoldDefinition(cmd *cobra.Command, target definitionTarget, body generate.DefineBody, config generate.RoutesFileConfiguration, getEnv func(string) string, loadPackages packageLoader) error {
	wd, name, templateFile := target.Dir, target.Name, target.File
	if templateFile != "" {
		if err := checkTemplateFileName(templateFile); err != nil {
			return err
		}
	}
	if err := muxt.CheckDefinitionName(name); err != nil {
		return err
	}
	var stub string
	if loadPackages != nil {
		var err error
		if stub, err = inspectPackage(target, config, loadPackages); err != nil {
			return err
		}
	}
	define := generate.DefineBlock(name, body)
	if templateFile == "" {
		return writeDefinitionArchive(cmd.OutOrStdout(), name, define, stub)
	}
	path := filepath.Join(wd, templateFile)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", templateFile, err)
	}
	content := generate.AppendDefinition(existing, define)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", templateFile, err)
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s: %s\n", templateFile, name); err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), stub)
	return err
}

// inspectPackage loads the package in target.Dir and returns a
// *generate.DuplicateDefinitionError when its templates already define
// target.Name, and a *load.UnreadFileError when target.File is one no templates variable
// reads. Otherwise it returns the stubs of the methods the route calls that the
// receiver type does not define, empty when no receiver type is configured. A
// package it cannot load, or one with no templates variable, is an error too:
// the check would otherwise pass for want of anything to check.
func inspectPackage(target definitionTarget, config generate.RoutesFileConfiguration, loadPackages packageLoader) (string, error) {
	wd, name, templateFile := target.Dir, target.Name, target.File
	cannotCheck := func(err error) error {
		return fmt.Errorf("cannot check whether %q is already defined: %w", name, err)
	}
	l, err := loadPackages(wd, []string{wd}, []string{config.ReceiverPackage})
	if err != nil {
		return "", cannotCheck(err)
	}
	pkg, err := l.Package(wd, config.TemplatesVariables)
	if err != nil {
		return "", cannotCheck(err)
	}
	for _, variable := range pkg.Variables {
		if variable.Set.Lookup(name) == nil {
			continue
		}
		duplicate := &generate.DuplicateDefinitionError{Name: name}
		if position, ok := variable.NamePosition(name); ok {
			duplicate.File, duplicate.Line = relativeTo(wd, position.Filename), position.Line
		}
		return "", duplicate
	}
	if templateFile != "" {
		if err := l.CheckFileRead(wd, config.TemplatesVariables, templateFile); err != nil {
			return "", err
		}
	}
	if config.ReceiverType == "" {
		return "", nil
	}
	receiver, err := l.Receiver(wd, config.ReceiverPackage, config.ReceiverType)
	if err != nil {
		return "", err
	}
	return generate.MethodStubs(pkg, receiver, name, l.StandardLibrary())
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

// writeDefinitionArchive writes a txtar archive to w holding the template file
// define would go in, and the method stub when there is one. The file names are
// the route's identifier in snake case, so the person can place each section
// where it belongs without renaming it.
func writeDefinitionArchive(w io.Writer, name, define, stub string) error {
	base, err := routeFileBase(name)
	if err != nil {
		return err
	}
	archive := &txtar.Archive{Files: []txtar.File{{Name: base + ".gohtml", Data: generate.AppendDefinition(nil, define)}}}
	if stub != "" {
		archive.Files = append(archive.Files, txtar.File{Name: base + ".go", Data: []byte(stub)})
	}
	_, err = w.Write(txtar.Format(archive))
	return err
}

// routeFileBase returns the identifier muxt gives the route name, in snake case.
// The identifier is the one muxt uses for the route's generated names, so it is
// asked of muxt rather than derived from the name here.
func routeFileBase(name string) (string, error) {
	templates, err := html.New("").Parse(generate.DefineBlock(name, generate.FragmentBody))
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", name, err)
	}
	defs, err := muxt.Definitions(source.Variable{Name: "probe", Set: templates})
	if err != nil {
		return "", err
	}
	if len(defs) != 1 {
		return "", fmt.Errorf("%q is not one route definition", name)
	}
	return strcase.ToSnake(defs[0].Identifier()), nil
}

// checkTemplateFileName rejects an --output-file that is not a .gohtml file in
// the working directory.
func checkTemplateFileName(name string) error {
	const extension = ".gohtml"
	if strings.ContainsAny(name, `/\`) || filepath.Dir(name) != "." {
		return fmt.Errorf("--%s must be a file name in the working directory: %s", outputFile, name)
	}
	if filepath.Ext(name) != extension || name == extension {
		return fmt.Errorf("--%s must be a file name ending in %s: %s", outputFile, extension, name)
	}
	return nil
}
