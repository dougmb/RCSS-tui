//go:build windows

package scheduler

// daemonState is not implemented on Windows: the Task Scheduler service is a
// core component and is effectively always running, so there is nothing useful
// to warn about. Reporting Unknown keeps the UI from making a claim it has not
// checked.
func daemonState() (DaemonStatus, string) { return DaemonUnknown, "" }
