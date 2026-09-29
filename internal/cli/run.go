package cli

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/mutation"
	"github.com/typelate/muxt/internal/muxt"
)

// This file holds what each command does with its configuration: load the
// package, run the implementation, and write what it produced. What the
// flags decide, but for --format, happens before a runner is called, in
// commands.go.

func runRoutes(cmd *cobra.Command, wd string, config analysis.DefinitionsConfiguration) error {
	_, pl, err := load.Packages(wd, config.ReceiverPackage)
	if err != nil {
		return err
	}
	pkg, receiver, err := load.PackageWithReceiver(wd, pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	results, err := analysis.NewRoutes(pkg, receiver)
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	for _, result := range results {
		if err := writeResult(cmd, cmd.OutOrStdout(), result); err != nil {
			return err
		}
	}
	return nil
}

func runCheck(cmd *cobra.Command, wd string, config analysis.CheckConfiguration) error {
	_, pl, err := load.Packages(wd)
	if err != nil {
		return err
	}
	logger := log.New(cmd.ErrOrStderr(), "", 0)
	warnPartialAST(logger, pl)
	pkg, err := load.Package(wd, pl, config.TemplatesVariables)
	if err != nil {
		return checkFailure(cmd, err)
	}
	checked, err := analysis.Check(config, logger, pkg)
	if err != nil {
		return checkFailure(cmd, err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s\n", plural(checked, "template"))
	return nil
}

func runTemplateCallers(cmd *cobra.Command, wd string, config analysis.TemplateCallersConfiguration) error {
	_, pl, err := load.Packages(wd)
	if err != nil {
		return err
	}
	pkg, err := load.Package(wd, pl, config.TemplatesVariables)
	if err != nil {
		return err
	}
	result, err := analysis.NewTemplateCallers(config, pkg)
	if err != nil {
		return err
	}
	return writeResult(cmd, cmd.OutOrStdout(), result)
}

func runTemplateCalls(cmd *cobra.Command, wd string, config analysis.TemplateCallsConfiguration) error {
	_, pl, err := load.Packages(wd)
	if err != nil {
		return err
	}
	pkg, err := load.Package(wd, pl, config.TemplatesVariables)
	if err != nil {
		return err
	}
	result, err := analysis.NewTemplateCalls(config, pkg)
	if err != nil {
		return err
	}
	return writeResult(cmd, cmd.OutOrStdout(), result)
}

func runTemplateMutations(cmd *cobra.Command, wd string, config mutation.Configuration) error {
	report, err := mutation.Run(config, wd, cmd.ErrOrStderr())
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	return writeResult(cmd, cmd.OutOrStdout(), report)
}

func runGenerate(cmd *cobra.Command, wd string, config generate.RoutesFileConfiguration) error {
	stdout := cmd.OutOrStdout()
	_, pl, err := load.Packages(wd, config.ReceiverPackage)
	if err != nil {
		return err
	}
	stderr := log.New(cmd.ErrOrStderr(), "", 0)
	warnPartialAST(stderr, pl)
	pkg, receiver, err := load.PackageWithReceiver(config.OutputDirectory(wd), pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	defs, err := muxt.ResolveDefinitions(pkg, receiver, load.StandardLibrary(pl))
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	files, err := generate.TemplateRoutesFiles(wd, config, pkg, defs, log.New(stdout, "", 0))
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	owned, err := ownedGeneratedFiles(wd, config.RoutesFunction, stderr)
	if err != nil {
		return err
	}
	written, err := writeGeneratedFiles(stdout, files, config)
	if err != nil {
		return err
	}
	return removeOrphans(owned, written)
}
