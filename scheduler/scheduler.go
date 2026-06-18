// Package scheduler installs and removes RCSS's self-scheduling entries in the
// host operating system's scheduler: the user's crontab on Unix (Linux/macOS)
// and Task Scheduler on Windows. It owns only RCSS-managed jobs — unrelated
// crontab lines or scheduled tasks are left untouched. Scheduled jobs invoke
// the rcss binary headless (e.g. `rcss upload`).
//
// The public API is platform-independent (jobs in, jobs out); each OS backend
// is selected at build time (crontab_unix.go / schtasks_windows.go) and
// translates jobs to and from the native format.
package scheduler

import "fmt"

// Kind is the headless subcommand a scheduled job runs.
type Kind int

const (
	// Upload runs `rcss upload`.
	Upload Kind = iota
	// Clean runs `rcss clean`.
	Clean
)

// Arg returns the rcss subcommand for the kind.
func (k Kind) Arg() string {
	if k == Clean {
		return "clean"
	}
	return "upload"
}

// Title returns a human label for the kind.
func (k Kind) Title() string {
	if k == Clean {
		return "Clean"
	}
	return "Upload"
}

// Job is one scheduled RCSS run.
type Job struct {
	Kind   Kind
	Hour   int  // 0–23
	Min    int  // 0–59
	Weekly bool // false = daily; true = weekly, on Sunday
}

// Time renders the job's time as HH:MM, or "??:??" when unknown (a backend may
// be unable to recover the exact time).
func (j Job) Time() string {
	if j.Hour < 0 || j.Min < 0 {
		return "??:??"
	}
	return fmt.Sprintf("%02d:%02d", j.Hour, j.Min)
}

// Cadence renders the job's recurrence in words.
func (j Job) Cadence() string {
	if j.Weekly {
		return "weekly (Sun)"
	}
	return "daily"
}

// Backend returns a human label for the active OS scheduler ("crontab" or
// "Task Scheduler"), for use in UI text.
func Backend() string { return backendName }

// Current returns the RCSS-managed jobs scheduled for the given account, or nil
// when none are scheduled. Jobs for other accounts are not returned.
func Current(account string) ([]Job, error) { return current(account) }

// Apply installs the given jobs for one account, replacing any previously
// RCSS-managed jobs for that same account while leaving other accounts' jobs
// untouched. An empty slice removes the account's jobs. exePath is the rcss
// binary the jobs run (invoked headless with --account); logPath is where a
// backend that captures stdout should append it.
func Apply(account string, jobs []Job, exePath, logPath string) error {
	return apply(account, jobs, exePath, logPath)
}
