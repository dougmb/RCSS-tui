//go:build windows

package scheduler

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const backendName = "Task Scheduler"

// managedKinds are the job kinds RCSS owns in Task Scheduler. apply removes both
// for the account before recreating the requested ones, so disabling a job
// deletes it.
var managedKinds = []Kind{Upload, Clean}

// sanitize maps an account name to a Task Scheduler-safe token (task names
// can't contain characters like ':' or '\\'). e.g. "drive:" → "drive".
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "default"
	}
	return out
}

// taskName is the Task Scheduler task name for an account+kind, e.g.
// "RCSS-drive-Upload". The "RCSS-" prefix scopes the tasks RCSS manages and the
// account segment keeps accounts isolated.
func taskName(account string, k Kind) string {
	return "RCSS-" + sanitize(account) + "-" + k.Title()
}

// apply removes the account's RCSS tasks (best effort) then creates the
// requested ones. Per-user tasks created this way do not require administrator
// rights. The headless rcss run writes its own per-account log, so no stdout
// redirection is needed here (logPath is unused on Windows).
func apply(account string, jobs []Job, exe, _ string) error {
	for _, k := range managedKinds {
		_ = deleteTask(taskName(account, k)) // ignore "task not found"
	}
	for _, j := range jobs {
		if err := createTask(account, j, exe); err != nil {
			return err
		}
	}
	return nil
}

// createTask registers one job for the account via schtasks /Create.
func createTask(account string, j Job, exe string) error {
	// /TR is a single string; quote the executable so paths with spaces work,
	// and pass --account so the headless run targets the right account.
	tr := fmt.Sprintf(`"%s" %s --account "%s"`, exe, j.Kind.Arg(), account)
	args := []string{
		"/Create", "/F",
		"/TN", taskName(account, j.Kind),
		"/TR", tr,
		"/ST", fmt.Sprintf("%02d:%02d", j.Hour, j.Min),
	}
	if j.Weekly {
		args = append(args, "/SC", "WEEKLY", "/D", "SUN")
	} else {
		args = append(args, "/SC", "DAILY")
	}
	if out, err := exec.Command("schtasks", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("creating task %s: %w: %s", taskName(account, j.Kind), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// deleteTask removes a task; the error (e.g. when absent) is the caller's to
// ignore.
func deleteTask(name string) error {
	return exec.Command("schtasks", "/Delete", "/F", "/TN", name).Run()
}

// current queries the account's managed tasks and reconstructs the jobs from the
// task XML, which (unlike the table/CSV output) is locale-independent.
func current(account string) ([]Job, error) {
	var jobs []Job
	for _, k := range managedKinds {
		out, err := exec.Command("schtasks", "/Query", "/TN", taskName(account, k), "/XML", "ONE").Output()
		if err != nil {
			continue // task not present
		}
		jobs = append(jobs, parseTaskXML(string(out), k))
	}
	return jobs, nil
}

// parseTaskXML extracts the time and cadence from schtasks /XML output. The
// output may be UTF-16, so interleaved NUL bytes are stripped before scanning
// for the ASCII tags.
func parseTaskXML(xml string, k Kind) Job {
	xml = strings.ReplaceAll(xml, "\x00", "")
	j := Job{Kind: k, Hour: -1, Min: -1, Weekly: strings.Contains(xml, "ScheduleByWeek")}

	const open, close = "<StartBoundary>", "</StartBoundary>"
	i := strings.Index(xml, open)
	if i < 0 {
		return j
	}
	rest := xml[i+len(open):]
	e := strings.Index(rest, close)
	if e < 0 {
		return j
	}
	ts := rest[:e] // e.g. 2024-01-01T03:00:00
	t := strings.IndexByte(ts, 'T')
	if t < 0 || len(ts) < t+6 {
		return j
	}
	hhmm := strings.Split(ts[t+1:t+6], ":")
	if len(hhmm) != 2 {
		return j
	}
	if h, err := strconv.Atoi(hhmm[0]); err == nil {
		j.Hour = h
	}
	if m, err := strconv.Atoi(hhmm[1]); err == nil {
		j.Min = m
	}
	return j
}
