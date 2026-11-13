package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/load"
)

// routesJob is one package directory the route listing reads. It is a
// packageJob because the listing reads what generate reads: the same
// directories, in the same order, resolved the same way.
type routesJob struct {
	packageJob
	// Report is the Configuration section the listing starts with.
	Report configurationReport
}

// newRoutesJob is the job for a packageJob, with the report of its
// configuration. f is the muxt.json the job's entry came from, nil when flags
// configured it, and flagNames are the flags that did.
func newRoutesJob(f *composite.File, flagNames []string, job packageJob) (routesJob, error) {
	report, err := newConfigurationReport(f, job.Key, flagNames, job.Config)
	if err != nil {
		return routesJob{}, err
	}
	return routesJob{packageJob: job, Report: report}, nil
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

// routesJobs chooses what the route listing reads, the way generateJobs
// chooses what generate writes: with no shared flag given and a muxt.json
// entry for wd, the entry decides; otherwise the flags do.
func routesJobs(wd string, flags *pflag.FlagSet, flagConfig analysis.DefinitionsConfiguration, getEnv func(string) string) ([]routesJob, error) {
	flagNames := givenGenerateFlags(flags)
	if len(flagNames) > 0 {
		if err := rejectConfiguredDirectory("muxt", wd, flagNames); err != nil {
			return nil, err
		}
	} else {
		f, jobs, found, err := entryJobs(wd, getEnv, noVersion)
		if err != nil {
			return nil, err
		}
		if found {
			listing := make([]routesJob, len(jobs))
			for i, job := range jobs {
				// Verbose is a run setting, never a muxt.json key.
				job.Config.Verbose = flagConfig.Verbose
				if listing[i], err = newRoutesJob(f, nil, job); err != nil {
					return nil, err
				}
			}
			return listing, nil
		}
	}
	// Only the three shared flags are read from the command line, so the
	// rest of the configuration is what an empty entry gives.
	config, err := defaultConfiguration(wd, getEnv)
	if err != nil {
		return nil, err
	}
	config.Verbose = flagConfig.Verbose
	config.ReceiverPackage = flagConfig.ReceiverPackage
	config.ReceiverType = flagConfig.ReceiverType
	config.TemplatesVariables = flagConfig.TemplatesVariables
	job, err := newRoutesJob(nil, flagNames, packageJob{Dir: wd, Config: config})
	if err != nil {
		return nil, err
	}
	return []routesJob{job}, nil
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
			collected []routesListing
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
