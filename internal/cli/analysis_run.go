package cli

import (
	"fmt"
	"io"
	"log"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/source"
)

// namedFailure is err for a run over several jobs, naming the job it came
// from. A diagnostic printed with cobra silenced would otherwise end the run
// with no word of whose it was, so cobra's own line is turned back on to
// carry the key.
func namedFailure(cmd *cobra.Command, job packageJob, several bool, err error) error {
	if !several {
		return err
	}
	cmd.SilenceErrors = false
	return fmt.Errorf("%s: %w", job.Key, err)
}

// checkRunner checks jobs from one load, a package after the packages it
// imports, and stops at the first one that fails.
func checkRunner(loadPackages packageLoader) func(*cobra.Command, []packageJob, analysis.CheckConfiguration) error {
	return func(cmd *cobra.Command, jobs []packageJob, config analysis.CheckConfiguration) error {
		logger := log.New(cmd.ErrOrStderr(), "", 0)
		return runJobs(cmd, loadPackages, jobs, jobVisitor{
			Visit: func(_ int, job packageJob, l *load.Loaded, several bool) (bool, error) {
				config.TemplatesVariables = job.Config.TemplatesVariables
				pkg, err := l.Package(job.Dir, config.TemplatesVariables)
				if err != nil {
					return false, namedFailure(cmd, job, several, checkFailure(cmd, err))
				}
				result, err := analysis.Check(config, logger, pkg)
				if err != nil {
					// Check logged each diagnostic; its error is only their count, and
					// cobra repeating it after them would bury the last diagnostic.
					cmd.SilenceErrors = true
					return false, namedFailure(cmd, job, several, err)
				}
				summary := plural(result.Checked, "template")
				if result.NonLiteral > 0 {
					summary += ", " + plural(result.NonLiteral, "call site") + " skipped"
				}
				if several {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), job.Key)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s\n", summary)
				return false, nil
			},
		})
	}
}

// callersRunner lists the template callers of jobs from one load.
func callersRunner(loadPackages packageLoader) func(*cobra.Command, []packageJob, analysis.TemplateCallersConfiguration) error {
	return func(cmd *cobra.Command, jobs []packageJob, config analysis.TemplateCallersConfiguration) error {
		return runListing(cmd, loadPackages, jobs, func(pkg source.Package, key string) (io.WriterTo, error) {
			result, err := analysis.NewTemplateCallers(config, pkg)
			if err != nil {
				return nil, err
			}
			return callersListing{TemplateCallers: result, Key: key}, nil
		})
	}
}

// callsRunner lists the template calls of jobs from one load.
func callsRunner(loadPackages packageLoader) func(*cobra.Command, []packageJob, analysis.TemplateCallsConfiguration) error {
	return func(cmd *cobra.Command, jobs []packageJob, config analysis.TemplateCallsConfiguration) error {
		return runListing(cmd, loadPackages, jobs, func(pkg source.Package, key string) (io.WriterTo, error) {
			result, err := analysis.NewTemplateCalls(config, pkg)
			if err != nil {
				return nil, err
			}
			return callsListing{TemplateCalls: result, Key: key}, nil
		})
	}
}

// runListing writes what list makes of each job's package. Over several jobs
// each listing carries its job's key, and with --format json they are one
// array, since several top-level values are not a document a JSON parser
// reads.
func runListing(cmd *cobra.Command, loadPackages packageLoader, jobs []packageJob, list func(pkg source.Package, key string) (io.WriterTo, error)) error {
	var (
		asJSON, formatKnown bool
		collected           []io.WriterTo
	)
	err := runJobs(cmd, loadPackages, jobs, jobVisitor{
		Visit: func(_ int, job packageJob, l *load.Loaded, several bool) (bool, error) {
			if !formatKnown {
				var err error
				if asJSON, err = isJSON(cmd); err != nil {
					return false, err
				}
				formatKnown = true
			}
			pkg, err := l.Package(job.Dir, job.Config.TemplatesVariables)
			if err != nil {
				return false, namedFailure(cmd, job, several, err)
			}
			var key string
			if several {
				key = job.Key
			}
			result, err := list(pkg, key)
			if err != nil {
				return false, namedFailure(cmd, job, several, err)
			}
			if asJSON && several {
				collected = append(collected, result)
				return false, nil
			}
			return false, writeResult(cmd, cmd.OutOrStdout(), result)
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

// callsListing is one package's template calls, headed by its key when a run
// lists several.
type callsListing struct {
	*analysis.TemplateCalls
	Key string `json:",omitzero"`
}

func (l callsListing) WriteTo(w io.Writer) (int64, error) {
	return writeKeyed(w, l.Key, l.TemplateCalls)
}

// callersListing is one package's template callers, headed by its key when a
// run lists several.
type callersListing struct {
	*analysis.TemplateCallers
	Key string `json:",omitzero"`
}

func (l callersListing) WriteTo(w io.Writer) (int64, error) {
	return writeKeyed(w, l.Key, l.TemplateCallers)
}

// writeKeyed writes key on a line of its own, when there is one, then result.
func writeKeyed(w io.Writer, key string, result io.WriterTo) (int64, error) {
	var n int64
	if key != "" {
		written, err := fmt.Fprintln(w, key)
		n += int64(written)
		if err != nil {
			return n, err
		}
	}
	written, err := result.WriteTo(w)
	return n + written, err
}
