package cli

import (
	"go/token"
	"log"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
)

// generateJob is one package directory and what to generate it with. Key is
// the packages key the configuration came from; it is empty when flags gave it.
type generateJob struct {
	Key    string
	Dir    string
	Config generate.RoutesFileConfiguration
}

// packageLoader is the only way a run asks for packages, so one go command
// run can serve several package directories and a test can answer without one.
// root is where the go command runs, dirs the package directories, patterns
// extra imports.
type packageLoader func(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error)

// loadFromGoCommand is the packageLoader that runs the go command.
func loadFromGoCommand(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error) {
	return load.PackagesInDirectories(root, nil, dirs, patterns...)
}

// generateRunner resolves every job from one load, then writes. Resolving
// everything first means a failure in one job leaves no job's files written.
func generateRunner(loadPackages packageLoader) func(*cobra.Command, []generateJob) error {
	return func(cmd *cobra.Command, jobs []generateJob) error {
		if len(jobs) == 0 {
			return nil
		}
		dirs := make([]string, 0, len(jobs))
		var patterns []string
		for _, job := range jobs {
			if err := requireDirectory(cmd.Name(), job.Dir); err != nil {
				return err
			}
			dirs = append(dirs, job.Dir)
			patterns = append(patterns, job.Config.ReceiverPackage)
		}
		stdout := cmd.OutOrStdout()
		stderr := log.New(cmd.ErrOrStderr(), "", 0)
		_, pl, err := loadPackages(jobs[0].Dir, dirs, patterns)
		if err != nil {
			return err
		}
		warnPartialAST(stderr, pl)
		type resolved struct {
			job   generateJob
			files []generate.GeneratedFile
		}
		var all []resolved
		for _, job := range jobs {
			config := job.Config
			pkg, receiver, err := load.PackageWithReceiver(config.OutputDirectory(job.Dir), pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
			if err != nil {
				printMultiLineError(cmd, err)
				return err
			}
			defs, err := muxt.ResolveDefinitions(pkg, receiver, load.StandardLibrary(pl))
			if err != nil {
				printMultiLineError(cmd, err)
				return err
			}
			files, err := generate.TemplateRoutesFiles(job.Dir, config, pkg, defs, log.New(stdout, "", 0))
			if err != nil {
				printMultiLineError(cmd, err)
				return err
			}
			all = append(all, resolved{job: job, files: files})
		}
		for _, r := range all {
			owned, err := ownedGeneratedFiles(r.job.Dir, r.job.Config.RoutesFunction, stderr)
			if err != nil {
				return err
			}
			written, err := writeGeneratedFiles(stdout, r.files, r.job.Config)
			if err != nil {
				return err
			}
			if err := removeOrphans(owned, written); err != nil {
				return err
			}
		}
		return nil
	}
}
