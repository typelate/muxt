package cli

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/load"
)

// jobVisitor is a command's work on the jobs runJobs loaded. i indexes the
// jobs slice, so a visitor reads its own per-job data from a slice it holds.
type jobVisitor struct {
	// Ready, when set, sees every job against the first load before any
	// Visit, so a refusal comes before the first write.
	Ready func(i int, job packageJob, l *load.Loaded) error
	// Begin, when set, runs before job's load is refreshed, so what it prints
	// comes before a failed reload's error.
	Begin func(i int, job packageJob)
	// Visit does one job and reports whether it wrote files in the job's
	// output directory. Its error is returned as it is; naming the job is
	// the visitor's to do.
	Visit func(i int, job packageJob, l *load.Loaded, several bool) (wrote bool, err error)
}

// runJobs requires each job's directory, loads every directory and receiver
// package once, warns once about a partial AST, and visits the jobs, a
// package after the packages it imports. A load that came before a write
// saw its package without the new files, so a job importing a package
// written since needs a fresh load; one that imports none keeps the load it
// has.
func runJobs(cmd *cobra.Command, loadPackages packageLoader, jobs []packageJob, v jobVisitor) error {
	if len(jobs) == 0 {
		return nil
	}
	dirs := make([]string, len(jobs))
	outputDirs := make([]string, len(jobs))
	patterns := make([]string, len(jobs))
	for i, job := range jobs {
		if err := requireDirectory(cmd.Name(), job.Dir); err != nil {
			return err
		}
		dirs[i] = job.Dir
		outputDirs[i] = job.Config.OutputDirectory(job.Dir)
		patterns[i] = job.Config.ReceiverPackage
	}
	l, err := loadPackages(jobs[0].Dir, dirs, patterns)
	if err != nil {
		return err
	}
	warnPartialAST(log.New(cmd.ErrOrStderr(), "", 0), l)
	if v.Ready != nil {
		for i, job := range jobs {
			if err := v.Ready(i, job, l); err != nil {
				return err
			}
		}
	}
	graph := l.Imports(outputDirs)
	var written []int
	for _, i := range graph.Order() {
		job := jobs[i]
		if v.Begin != nil {
			v.Begin(i, job)
		}
		if graph.ImportsAny(i, written) {
			if l, err = loadPackages(jobs[0].Dir, dirs, patterns); err != nil {
				if len(jobs) > 1 {
					return fmt.Errorf("%s: %w", job.Key, err)
				}
				return err
			}
			graph = l.Imports(outputDirs)
			written = nil
		}
		wrote, err := v.Visit(i, job, l, len(jobs) > 1)
		if err != nil {
			return err
		}
		if wrote {
			written = append(written, i)
		}
	}
	return nil
}
