package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level is a log severity, rendered as a fixed-width tag to match the original
// scripts' "[INFO   ]" / "[WARN   ]" / "[ERROR  ]" / "[VERBOSE]" columns.
type Level string

const (
	LevelInfo    Level = "INFO   "
	LevelWarn    Level = "WARN   "
	LevelError   Level = "ERROR  "
	LevelVerbose Level = "VERBOSE"
)

// Logger writes timestamped lines to the sync log file (append-only) and
// forwards every line to a sink callback, which the TUI uses to render output
// live and the headless mode uses to print to stdout. It mirrors the shared
// _log/log_* helpers of uploadBackup.sh and cleanRemoteBackups.sh.
//
// The file may be nil (restore logs only to the terminal, like the original
// restoreBackup.sh). The sink may be nil.
type Logger struct {
	mu      sync.Mutex
	file    io.WriteCloser
	sink    func(string)
	verbose bool
	ownFile bool
}

// NewLogger opens (creating/appending) the given log file. If path is empty,
// the logger writes to the sink only. sink may be nil; verbose enables
// Verbosef output and DEBUG-level rclone logging.
func NewLogger(path string, sink func(string), verbose bool) (*Logger, error) {
	l := &Logger{sink: sink, verbose: verbose}
	if path == "" {
		return l, nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	l.file = f
	l.ownFile = true
	return l, nil
}

// Close closes the underlying log file if this logger owns it.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil && l.ownFile {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// IsVerbose reports whether verbose logging is enabled.
func (l *Logger) IsVerbose() bool { return l.verbose }

// RcloneLogLevel maps verbosity to the rclone --log-level the scripts used:
// DEBUG when verbose, otherwise NOTICE.
func (l *Logger) RcloneLogLevel() string {
	if l.verbose {
		return "DEBUG"
	}
	return "NOTICE"
}

func (l *Logger) emit(level Level, format string, args ...any) {
	msg := fmt.Sprintf("[%s] [%s] %s",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		fmt.Sprintf(format, args...),
	)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		fmt.Fprintln(l.file, msg)
	}
	if l.sink != nil {
		l.sink(msg)
	}
}

// Infof logs at INFO level.
func (l *Logger) Infof(format string, args ...any) { l.emit(LevelInfo, format, args...) }

// Warnf logs at WARN level.
func (l *Logger) Warnf(format string, args ...any) { l.emit(LevelWarn, format, args...) }

// Errorf logs at ERROR level.
func (l *Logger) Errorf(format string, args ...any) { l.emit(LevelError, format, args...) }

// Verbosef logs at VERBOSE level, but only when verbose is enabled.
func (l *Logger) Verbosef(format string, args ...any) {
	if l.verbose {
		l.emit(LevelVerbose, format, args...)
	}
}

// Raw forwards an unformatted line (e.g. live rclone progress output) to the
// sink only. Like the scripts, raw rclone output is not appended to the log
// file — only the structured _log lines are.
func (l *Logger) Raw(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sink != nil {
		l.sink(line)
	}
}

// writeBlock appends a raw multi-line block to the log file only (no sink),
// matching how the scripts write the SYNC SUMMARY directly to LOG_FILE.
func (l *Logger) writeBlock(lines []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	for _, ln := range lines {
		fmt.Fprintln(l.file, ln)
	}
}
