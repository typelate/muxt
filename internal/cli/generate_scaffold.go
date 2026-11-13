package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// generatePackageCommand is muxt generate package NAME: it creates ./NAME and
// adds its entry to the nearest muxt.json, with the generate flags given.
func generatePackageCommand(workingDirectory *string, getEnv func(string) string, version func() (string, bool)) *cobra.Command {
	var (
		config                 generate.RoutesFileConfiguration
		deprecatedTemplatesVar string
	)
	cmd := &cobra.Command{
		Use:   "package NAME",
		Short: "Create a package directory and its muxt.json entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return scaffoldPackage(cmd.OutOrStdout(), *workingDirectory, args[0], cmd.Flags(), &config, deprecatedTemplatesVar, getEnv, version)
		},
	}
	addGenerateFlags(cmd.Flags(), &config, &deprecatedTemplatesVar)
	return cmd
}

// scaffoldPackage writes the package directory name and adds its entry to the
// muxt.json above wd. Everything is checked before anything is written: the
// merged entry must pass the validation generate applies to an entry.
func scaffoldPackage(out io.Writer, wd, name string, flags *pflag.FlagSet, config *generate.RoutesFileConfiguration, deprecatedTemplatesVar string, getEnv func(string) string, version func() (string, bool)) error {
	switch {
	case strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator):
		return &composite.PackageNameError{Name: name, Reason: "must be one directory name, created in the working directory"}
	case name == "main":
		// A package main with no func main breaks go build ./..., and a
		// route package is imported, which package main cannot be.
		return &composite.PackageNameError{Name: name, Reason: "is the main package; a route package is a library package the main package imports"}
	case !token.IsIdentifier(name):
		return &composite.PackageNameError{Name: name, Reason: "must be a Go identifier"}
	}
	f, found, err := composite.Find(wd, muxtJSONKeys())
	if err != nil {
		return err
	}
	if !found {
		return &composite.NoFileError{Dir: wd}
	}
	dir := filepath.Join(wd, name)
	if _, err := os.Stat(dir); err == nil {
		return &composite.ExistsError{Path: name}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", name, err)
	}
	key, err := f.KeyFor(dir)
	if err != nil {
		return err
	}
	if _, exists := f.Packages[key]; exists {
		return &composite.ExistsError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", key)}
	}
	if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
		return err
	}
	entry, err := entryFromFlags(flags, *config, deprecatedTemplatesVar != "")
	if err != nil {
		return err
	}
	receiver := "T"
	if f.Defaults.UseReceiverType != nil {
		receiver = *f.Defaults.UseReceiverType
	}
	if flags.Changed(useReceiverType) {
		receiver = config.ReceiverType
	}
	if f.Defaults.UseReceiverType == nil || *f.Defaults.UseReceiverType != receiver {
		entry.UseReceiverType = &receiver
	}
	candidate := &composite.File{Dir: f.Dir, Name: f.Name, Defaults: f.Defaults, Packages: maps.Clone(f.Packages)}
	if candidate.Packages == nil {
		candidate.Packages = map[string]composite.Entry{}
	}
	candidate.Packages[key] = entry
	resolved, _ := candidate.Resolve(key)
	if _, err := entryConfiguration(resolved, dir, getEnv, version); err != nil {
		return &composite.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", key), Err: err}
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	// The entry names the variable the next generate looks for, so the file
	// declares the first one the resolved entry names.
	var templatesVariable string
	if len(resolved.UseTemplatesVariable) > 0 {
		templatesVariable = resolved.UseTemplatesVariable[0]
	}
	filename, src, err := generate.PackageFile(generate.PackageFileSpec{Name: name, ReceiverType: receiver, TemplatesVariable: templatesVariable})
	if err != nil {
		return err
	}
	// The embed in the Go file needs a match, so the template file exists
	// even though it is empty.
	written := []string{filepath.Join(name, filename), filepath.Join(name, name+".gohtml")}
	if err := os.WriteFile(filepath.Join(dir, filename), src, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", written[0], err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".gohtml"), nil, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", written[1], err)
	}
	f.Packages = candidate.Packages
	if err := f.Save(); err != nil {
		return err
	}
	for _, path := range written {
		if _, err := fmt.Fprintf(out, "wrote %s\n", path); err != nil {
			return err
		}
	}
	// A status line names files relative to the working directory, like the
	// package files above; only errors carry the absolute path.
	absWD, err := filepath.Abs(wd)
	if err != nil {
		return fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	shown, err := filepath.Rel(absWD, f.Name)
	if err != nil {
		return fmt.Errorf("path of %s from %s: %w", f.Name, absWD, err)
	}
	_, err = fmt.Fprintf(out, "wrote %s: packages[%q]\n", shown, key)
	return err
}

// entryFromFlags returns the entry that records each generate flag given, under
// its muxt.json key, with the value the flag holds. usesTemplatesVariable is
// true when the deprecated templates variable flag was given instead.
func entryFromFlags(flags *pflag.FlagSet, config generate.RoutesFileConfiguration, usesTemplatesVariable bool) (composite.Entry, error) {
	values := map[string]any{}
	for _, field := range entryFields() {
		given := flags.Changed(field.Name) || (field.Name == useTemplatesVariable && usesTemplatesVariable)
		if given {
			values[composite.Key(field.Name)] = field.Value(config)
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return composite.Entry{}, fmt.Errorf("encoding generate flags: %w", err)
	}
	var entry composite.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return composite.Entry{}, fmt.Errorf("reading generate flags: %w", err)
	}
	return entry, nil
}

// generateConfigCommand is muxt generate config: it writes the empty muxt.json
// that generate package and a flagless generate then add entries to.
func generateConfigCommand(workingDirectory *string) *cobra.Command {
	return &cobra.Command{
		Use:     "config",
		Aliases: []string{"configuration"},
		Short:   "Write an empty muxt.json in the working directory",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			if err := composite.Create(*workingDirectory, muxtJSONKeys()); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "wrote "+composite.FileName)
			return err
		},
	}
}
