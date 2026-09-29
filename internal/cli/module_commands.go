package cli

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/fakeserver"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
)

// parseModuleHeader reads the arguments recorded in a generated file's
// header into what analysis.NewModule reports about the package. It reports
// false when they do not parse.
func parseModuleHeader(args []string) (analysis.PackageConfig, bool) {
	var (
		config                 generate.RoutesFileConfiguration
		deprecatedTemplatesVar string
	)
	set := pflag.NewFlagSet("parse-header", pflag.ContinueOnError)
	set.SetOutput(io.Discard)
	addGenerateFlags(set, &config, &deprecatedTemplatesVar)
	if err := set.Parse(args); err != nil {
		return analysis.PackageConfig{}, false
	}
	return analysis.PackageConfig{
		RoutesFunction:         cmp.Or(config.RoutesFunction, generate.DefaultRoutesFunctionName),
		ReceiverInterface:      cmp.Or(config.ReceiverInterface, generate.DefaultReceiverInterfaceName),
		ReceiverType:           config.ReceiverType,
		ReceiverPackage:        config.ReceiverPackage,
		TemplateRoutePathsType: cmp.Or(config.TemplateRoutePathsTypeName, generate.DefaultTemplateRoutePathsTypeName),
		OutputHTMX:             config.OutputHTMX,
		OutputDatastar:         config.OutputDatastar,
		Logger:                 config.Logger,
		PathPrefix:             config.PathPrefix,
		Middleware:             config.Middleware,
	}, true
}

func exploreModuleCommand(workingDirectory *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     exploreModuleCommandName,
		Aliases: []string{"explore"},
		Short:   "Explore all muxt packages in the module",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			result, err := analysis.NewModule(*workingDirectory, parseModuleHeader)
			if err != nil {
				return err
			}
			return writeResult(cmd, cmd.OutOrStdout(), result)
		},
	}

	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}

func generateFakeServerCommand(workingDirectory *string) *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   generateFakeServerCommandName + " [package-dirs...]",
		Short: "Generate a fake server main.go for exploring routes (unstable -- do not depend on the fake interface)",
		Long: `Generate a main.go and internal/fake/receiver.go containing a counterfeiter fake
and an httptest server for interactively exploring routes. The target package
must be a library (not main).

WARNING: The generated fake interface is unstable and should not be relied upon.
This command is intended for exploratory use only.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			mod, err := analysis.NewModule(*workingDirectory, parseModuleHeader)
			if err != nil {
				return err
			}

			if len(args) == 0 {
				args = []string{*workingDirectory}
			}
			outDir := absoluteDir(*workingDirectory, cmp.Or(outputDir, filepath.Join("cmd", "explore-goland")))
			importPath, err := fakeImportPath(mod, outDir)
			if err != nil {
				return err
			}
			relOut, err := filepath.Rel(*workingDirectory, outDir)
			if err != nil {
				relOut = outDir
			}

			for _, arg := range args {
				pkg, err := packageInDirectory(mod, absoluteDir(*workingDirectory, arg))
				if err != nil {
					return err
				}
				_, pl, err := load.Packages(pkg.Dir)
				if err != nil {
					return err
				}
				files, err := fakeserver.Generate(newFakeServerConfig(pkg, importPath), pl)
				if err != nil {
					return err
				}
				if err := writeFakeServer(outDir, files); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run: go run ./%s\n", relOut)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "output directory (default: ./cmd/explore-goland)")

	return cmd
}

func absoluteDir(wd, dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(wd, dir)
}

func fakeImportPath(mod *analysis.Module, outDir string) (string, error) {
	rel, err := filepath.Rel(mod.ModuleDir, outDir)
	if err != nil {
		return "", fmt.Errorf("computing fake import path: %w", err)
	}
	return mod.ModulePath + "/" + filepath.ToSlash(rel) + "/internal/fake", nil
}

func packageInDirectory(mod *analysis.Module, dir string) (analysis.PackageInfo, error) {
	for _, pkg := range mod.Packages {
		if pkg.Dir == dir {
			return pkg, nil
		}
	}
	return analysis.PackageInfo{}, fmt.Errorf("no muxt-generated package found at %s", dir)
}

func newFakeServerConfig(pkg analysis.PackageInfo, fakeImportPath string) fakeserver.Config {
	return fakeserver.Config{
		PackagePath:       pkg.Path,
		PackageDir:        pkg.Dir,
		RoutesFunction:    pkg.Config.RoutesFunction,
		ReceiverInterface: pkg.Config.ReceiverInterface,
		Logger:            pkg.Config.Logger,
		PathPrefix:        pkg.Config.PathPrefix,
		Middleware:        pkg.Config.Middleware,
		FakeImportPath:    fakeImportPath,
	}
}

func writeFakeServer(outDir string, files *fakeserver.Files) error {
	fakeDir := filepath.Join(outDir, "internal", "fake")
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "main.go"), files.Main, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(fakeDir, "receiver.go"), files.Fake, 0o644)
}
