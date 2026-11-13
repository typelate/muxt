package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
)

// routesJob is one package directory the route listing reads. It is a
// packageJob because the listing reads what generate reads: the same
// directories, in the same order, resolved the same way.
type routesJob struct {
	packageJob
	// Report is the Configuration section the listing starts with.
	Report analysis.ConfigurationReport
}

// newRoutesJobs is a routesJob for each job sel chose, each with the report of
// its configuration: the muxt.json it came from, or the flags and defaults.
func newRoutesJobs(sel selection) ([]routesJob, error) {
	jobs := make([]routesJob, len(sel.Jobs))
	for i, job := range sel.Jobs {
		report, err := newConfigurationReport(sel.EntryFile(job), job.Key, job.Given, job.Config)
		if err != nil {
			return nil, err
		}
		jobs[i] = routesJob{packageJob: job, Report: report}
	}
	return jobs, nil
}

// definitions is the part of the job's configuration the listing resolves with.
func (j routesJob) definitions() analysis.DefinitionsConfiguration {
	return analysis.DefinitionsConfiguration{
		Verbose:            j.Config.Verbose,
		ReceiverPackage:    j.Config.ReceiverPackage,
		ReceiverType:       j.Config.ReceiverType,
		TemplatesVariables: j.Config.TemplatesVariables,
	}
}

// rootSpec is the jobSpec of bare muxt. Only the three shared flags are read
// from the command line, so the rest of its Fallback is what an empty entry
// gives.
func rootSpec(flagConfig analysis.DefinitionsConfiguration, settings []setting, getEnv func(string) string) jobSpec {
	return jobSpec{
		Name:     rootCommandName,
		Settings: settings,
		Scope:    eachEntry,
		Verbose:  flagConfig.Verbose,
		Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
			config, err := defaultConfiguration(dir, getEnv)
			if err != nil {
				return generate.RoutesFileConfiguration{}, err
			}
			config.ReceiverPackage = flagConfig.ReceiverPackage
			config.ReceiverType = flagConfig.ReceiverType
			config.TemplatesVariables = flagConfig.TemplatesVariables
			return config, nil
		},
	}
}

// routesRunner lists jobs from one load, a package after the packages it
// imports.
func routesRunner(loadPackages packageLoader) func(*cobra.Command, []routesJob) error {
	return func(cmd *cobra.Command, jobs []routesJob) error {
		packageJobs := make([]packageJob, len(jobs))
		for i, job := range jobs {
			packageJobs[i] = job.packageJob
		}
		var (
			asJSON, formatKnown bool
			// Over several entries JSON is one array, not a value per package.
			collected []io.WriterTo
		)
		err := runJobs(cmd, loadPackages, packageJobs, jobVisitor{
			Visit: func(i int, job packageJob, l *load.Loaded, several bool) (bool, error) {
				if !formatKnown {
					var err error
					if asJSON, err = isJSON(cmd); err != nil {
						return false, err
					}
					formatKnown = true
				}
				config := jobs[i].definitions()
				pkg, receiver, err := l.PackageWithReceiver(job.Config.OutputDirectory(job.Dir), config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
				if err != nil {
					printMultiLineError(cmd, err)
					return false, err
				}
				results, err := analysis.NewRoutes(pkg, receiver)
				if err != nil {
					printMultiLineError(cmd, err)
					return false, err
				}
				for n, result := range results {
					listing := routesListing{Routes: result, Configuration: jobs[i].Report, printConfiguration: n == 0}
					if several {
						listing.Key = job.Key
					}
					if asJSON && several {
						collected = append(collected, listing)
						continue
					}
					if err := writeResult(cmd, cmd.OutOrStdout(), listing); err != nil {
						return false, err
					}
				}
				return false, nil
			},
		})
		if err != nil {
			return err
		}
		if collected == nil {
			return nil
		}
		return writeListings(cmd.OutOrStdout(), collected)
	}
}
