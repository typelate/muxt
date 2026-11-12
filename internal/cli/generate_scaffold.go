package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
	"github.com/typelate/muxt/internal/scaffold"
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
			return scaffoldPackage(*workingDirectory, args[0], cmd.Flags(), &config, deprecatedTemplatesVar, getEnv, version)
		},
	}
	addGenerateFlags(cmd.Flags(), &config, &deprecatedTemplatesVar)
	return cmd
}

// scaffoldPackage writes the package directory name and adds its entry to the
// muxt.json above wd. Everything is checked before anything is written: the
// merged entry must pass the validation generate applies to an entry.
func scaffoldPackage(wd, name string, flags *pflag.FlagSet, config *generate.RoutesFileConfiguration, deprecatedTemplatesVar string, getEnv func(string) string, version func() (string, bool)) error {
	if !token.IsIdentifier(name) {
		return fmt.Errorf("package name %q must be a Go identifier", name)
	}
	f, found, err := muxtjson.Find(wd, deprecatedMuxtJSONKeys())
	if err != nil {
		return err
	}
	if !found {
		return &muxtjson.NoFileError{Dir: wd}
	}
	dir := filepath.Join(wd, name)
	if _, err := os.Stat(dir); err == nil {
		return &muxtjson.ExistsError{Path: name}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", name, err)
	}
	key, err := f.KeyFor(dir)
	if err != nil {
		return err
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
	candidate := &muxtjson.File{Dir: f.Dir, Name: f.Name, Defaults: f.Defaults, Packages: maps.Clone(f.Packages)}
	if candidate.Packages == nil {
		candidate.Packages = map[string]muxtjson.Entry{}
	}
	candidate.Packages[key] = entry
	resolved, _ := candidate.Resolve(key)
	if _, err := entryConfiguration(resolved, dir, getEnv, version); err != nil {
		return &muxtjson.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", key), Err: err}
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	filename, src, err := scaffold.Package(scaffold.PackageSpec{Name: name, ReceiverType: receiver})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, filename), src, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(name, filename), err)
	}
	f.Packages = candidate.Packages
	return f.Save()
}

// entryFromFlags returns the entry that records each generate flag given, under
// its muxt.json key, with the value the flag holds. usesTemplatesVariable is
// true when the deprecated templates variable flag was given instead.
func entryFromFlags(flags *pflag.FlagSet, config generate.RoutesFileConfiguration, usesTemplatesVariable bool) (muxtjson.Entry, error) {
	values := map[string]any{}
	for _, field := range entryFields() {
		given := flags.Changed(field.Name) || (field.Name == useTemplatesVariable && usesTemplatesVariable)
		if given {
			values[muxtJSONKey(field.Name)] = field.Value(config)
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return muxtjson.Entry{}, fmt.Errorf("encoding generate flags: %w", err)
	}
	var entry muxtjson.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return muxtjson.Entry{}, fmt.Errorf("reading generate flags: %w", err)
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
			if err := muxtjson.Create(*workingDirectory); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "wrote "+muxtjson.FileName)
			return err
		},
	}
}
