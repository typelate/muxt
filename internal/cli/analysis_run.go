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
// from. A caller that wraps must also turn cobra's own error line back on if a
// diagnostic silenced it, or the run ends with no word of whose failure it was.
func namedFailure(job packageJob, several bool, err error) error {
	if !several {
		return err
	}
	return fmt.Errorf("%s: %w", job.Key, err)
}

// keyedWriter writes key and a newline ahead of the first thing written to w,
// so a job that reports nothing prints nothing, and one that reports
// something says whose it is.
type keyedWriter struct {
	w       io.Writer
	key     string
	written bool
}

func (k *keyedWriter) Write(p []byte) (int, error) {
	if !k.written {
		k.written = true
		if _, err := io.WriteString(k.w, k.key+"\n"); err != nil {
			return 0, err
		}
	}
	return k.w.Write(p)
}

// checkRunner checks jobs from one load, a package after the packages it
// imports, and stops at the first one that fails.
func checkRunner(loadPackages packageLoader) func(*cobra.Command, []packageJob, analysis.CheckConfiguration) error {
	return func(cmd *cobra.Command, jobs []packageJob, config analysis.CheckConfiguration) error {
		return runJobs(cmd, loadPackages, jobs, jobVisitor{
			Visit: func(_ int, job packageJob, l *load.Loaded, several bool) (bool, error) {
				// Over several jobs the diagnostics of one say which it is.
				var diagnostics io.Writer = cmd.ErrOrStderr()
				if several {
					diagnostics = &keyedWriter{w: diagnostics, key: job.Key}
				}
				logger := log.New(diagnostics, "", 0)
				config.TemplatesVariables = job.Config.TemplatesVariables
				pkg, err := l.Package(job.Dir, config.TemplatesVariables)
				if err != nil {
					err = checkFailure(cmd, err)
					if several {
						cmd.SilenceErrors = false
					}
					return false, namedFailure(job, several, err)
				}
				result, err := analysis.Check(config, logger, pkg)
				if err != nil {
					// Check logged each diagnostic; its error is only their count, and
					// cobra repeating it after them would bury the last diagnostic.
					// With several jobs the line carries the job's key, so it stays on.
					cmd.SilenceErrors = !several
					return false, namedFailure(job, several, err)
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
				if several {
					cmd.SilenceErrors = false
				}
				return false, namedFailure(job, several, err)
			}
			var key string
			if several {
				key = job.Key
			}
			result, err := list(pkg, key)
			if err != nil {
				if several {
					cmd.SilenceErrors = false
				}
				return false, namedFailure(job, several, err)
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
