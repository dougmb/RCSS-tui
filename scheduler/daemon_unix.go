//go:build !windows

package scheduler

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cronDaemons are the process names of the cron implementations a crontab entry
// can be executed by. Writing to the crontab is useless if none of them runs.
var cronDaemons = map[string]bool{
	"cron": true, "crond": true, "cronie": true, "fcron": true, "dcron": true,
}

// daemonState reports whether a cron daemon is running, by scanning /proc for a
// known cron process. Without /proc (macOS, BSD) the question can't be answered
// cheaply, so it returns DaemonUnknown — the UI stays quiet rather than claiming
// something it cannot verify.
func daemonState() (DaemonStatus, string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return DaemonUnknown, ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue // not a pid directory
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue // the process ended, or is not ours to read
		}
		if cronDaemons[strings.TrimSpace(string(comm))] {
			return DaemonRunning, ""
		}
	}
	return DaemonStopped, "No cron daemon is running, so saved jobs will not execute. " +
		"Start one, e.g. `sudo systemctl enable --now cronie` (or cron / crond on your distro)."
}
