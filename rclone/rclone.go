// Package rclone is a thin wrapper over the rclone binary (invoked via
// exec.Command). It exposes the handful of operations RCSS needs —
// ListRemotes, Lsf, Copy, Delete — and a startup check that rclone is on the
// PATH. rclone keeps its own credentials/config; this package only shells out.
package rclone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ListEntry describes one file returned by rclone lsjson. Size is nil when
// the remote does not report a size.
type ListEntry struct {
	Path    string
	Size    *int64
	ModTime time.Time
}

type listJSONEntry struct {
	Path    string          `json:"Path"`
	Size    json.RawMessage `json:"Size"`
	ModTime string          `json:"ModTime"`
	IsDir   bool            `json:"IsDir"`
}

// ListJSON recursively lists files older than minAge using rclone lsjson.
func (c *Client) ListJSON(ctx context.Context, path, minAge string) ([]ListEntry, error) {
	args := []string{"lsjson", path, "--recursive", "--files-only"}
	if minAge != "" {
		args = append(args, "--min-age", minAge)
	}
	out, err := c.output(ctx, args...)
	if err != nil {
		return nil, err
	}
	var raw []listJSONEntry
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parsing rclone lsjson: %w", err)
	}
	entries := make([]ListEntry, 0, len(raw))
	for _, item := range raw {
		if item.IsDir || item.Path == "" {
			continue
		}
		modTime, err := time.Parse(time.RFC3339Nano, item.ModTime)
		if err != nil {
			return nil, fmt.Errorf("parsing modtime for %q: %w", item.Path, err)
		}
		var size *int64
		if len(item.Size) != 0 && string(item.Size) != "null" {
			var n int64
			if err := json.Unmarshal(item.Size, &n); err != nil {
				return nil, fmt.Errorf("parsing size for %q: %w", item.Path, err)
			}
			if n >= 0 {
				size = &n
			}
		}
		entries = append(entries, ListEntry{Path: item.Path, Size: size, ModTime: modTime})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// DefaultBin is the rclone executable name looked up on the PATH.
const DefaultBin = "rclone"

// Client runs rclone commands. The zero value uses the rclone binary on PATH;
// Bin may be set to an explicit path (useful in tests).
type Client struct {
	Bin string
}

// New returns a Client using the rclone binary found on PATH.
func New() *Client { return &Client{Bin: DefaultBin} }

func (c *Client) bin() string {
	if c == nil || c.Bin == "" {
		return DefaultBin
	}
	return c.Bin
}

// EnsureInstalled reports an error if the rclone binary cannot be found on the
// PATH. Call it at startup, mirroring the scripts' `command -v rclone` guard.
func (c *Client) EnsureInstalled() error {
	if _, err := exec.LookPath(c.bin()); err != nil {
		return fmt.Errorf("rclone not found on PATH: %w", err)
	}
	return nil
}

// ListRemotes returns the configured rclone remotes (e.g. "drive:"), one per
// entry, via `rclone listremotes`.
func (c *Client) ListRemotes(ctx context.Context) ([]string, error) {
	out, err := c.output(ctx, "listremotes")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// LsfMode selects which entries `rclone lsf` returns.
type LsfMode int

const (
	// LsfAll lists both files and directories.
	LsfAll LsfMode = iota
	// LsfDirsOnly lists directories only (--dirs-only).
	LsfDirsOnly
	// LsfFilesOnly lists files only (--files-only).
	LsfFilesOnly
)

// LsfOptions configures a Lsf call.
type LsfOptions struct {
	Mode      LsfMode
	Recursive bool   // --recursive
	MaxAge    string // --max-age, e.g. "2d" (empty to omit)
}

// Lsf lists the contents of a remote path via `rclone lsf`, returning the
// entries (trailing slash on directories, as rclone prints them).
func (c *Client) Lsf(ctx context.Context, path string, opts LsfOptions) ([]string, error) {
	args := []string{"lsf", path}
	switch opts.Mode {
	case LsfDirsOnly:
		args = append(args, "--dirs-only")
	case LsfFilesOnly:
		args = append(args, "--files-only")
	}
	if opts.Recursive {
		args = append(args, "--recursive")
	}
	if opts.MaxAge != "" {
		args = append(args, "--max-age", opts.MaxAge)
	}
	out, err := c.output(ctx, args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// CopyOptions configures a Copy call. Upload uses Update/UseMmap/Retries/
// Stats; restore uses IgnoreTimes/Verbose/DryRun.
type CopyOptions struct {
	Update       bool     // --update
	UseMmap      bool     // --use-mmap
	Retries      int      // --retries N (omitted if <= 0)
	Progress     bool     // -P
	StatsOneLine bool     // --stats-one-line
	Stats        string   // --stats DURATION, e.g. "10s" (empty to omit)
	LogLevel     string   // --log-level LEVEL (empty to omit)
	Excludes     []string // --exclude PATTERN (one flag per pattern)
	IgnoreTimes  bool     // --ignore-times
	Verbose      bool     // -v
	DryRun       bool     // --dry-run
	MaxDepth     int      // --max-depth N (omitted if <= 0)
}

// Copy runs `rclone copy src dst` with the given options. Each line of
// combined stdout/stderr (including \r-delimited progress updates) is passed
// to onLine, which may be nil.
func (c *Client) Copy(ctx context.Context, src, dst string, opts CopyOptions, onLine func(string)) error {
	args := []string{"copy", src, dst}
	if opts.LogLevel != "" {
		args = append(args, "--log-level", opts.LogLevel)
	}
	if opts.StatsOneLine {
		args = append(args, "--stats-one-line")
	}
	if opts.Stats != "" {
		args = append(args, "--stats", opts.Stats)
	}
	if opts.Update {
		args = append(args, "--update")
	}
	if opts.UseMmap {
		args = append(args, "--use-mmap")
	}
	if opts.Retries > 0 {
		args = append(args, "--retries", fmt.Sprintf("%d", opts.Retries))
	}
	if opts.IgnoreTimes {
		args = append(args, "--ignore-times")
	}
	if opts.Progress {
		args = append(args, "-P")
	}
	if opts.Verbose {
		args = append(args, "-v")
	}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.MaxDepth > 0 {
		args = append(args, "--max-depth", fmt.Sprintf("%d", opts.MaxDepth))
	}
	for _, pat := range opts.Excludes {
		args = append(args, "--exclude", pat)
	}
	return c.stream(ctx, onLine, args...)
}

// DeleteOptions configures a Delete call.
type DeleteOptions struct {
	MinAge   string // --min-age, e.g. "15d" (empty to omit)
	DryRun   bool   // --dry-run
	LogLevel string // --log-level LEVEL (empty to omit)
}

// Delete runs `rclone delete path` with the given options. Each line of
// combined output is passed to onLine, which may be nil.
func (c *Client) Delete(ctx context.Context, path string, opts DeleteOptions, onLine func(string)) error {
	args := []string{"delete", path}
	if opts.MinAge != "" {
		args = append(args, "--min-age", opts.MinAge)
	}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.LogLevel != "" {
		args = append(args, "--log-level", opts.LogLevel)
	}
	return c.stream(ctx, onLine, args...)
}

// DeleteFiles deletes only paths present in files using a temporary
// --files-from-raw manifest. CR and LF are rejected because they alter manifest boundaries.
func (c *Client) DeleteFiles(ctx context.Context, path string, files []string, dryRun bool, logLevel string, onLine func(string)) error {
	if len(files) == 0 {
		return errors.New("refusing empty delete manifest")
	}
	for _, file := range files {
		if file == "" || strings.ContainsAny(file, "\r\n") {
			return fmt.Errorf("invalid manifest path %q", file)
		}
	}
	f, err := os.CreateTemp("", "rcss-clean-*")
	if err != nil {
		return fmt.Errorf("creating delete manifest: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	for _, file := range files {
		if _, err = fmt.Fprintln(f, file); err != nil {
			f.Close()
			return fmt.Errorf("writing delete manifest: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing delete manifest: %w", err)
	}
	args := []string{"delete", path, "--files-from-raw", name}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if logLevel != "" {
		args = append(args, "--log-level", logLevel)
	}
	return c.stream(ctx, onLine, args...)
}

// output runs an rclone command and returns its stdout, capturing stderr for
// error context. Use for short, list-style commands.
func (c *Client) output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if summary := summarizeFailure(strings.Split(msg, "\n")); summary != "" {
			return "", fmt.Errorf("rclone %s: %s: %w", args[0], summary, err)
		}
		return "", fmt.Errorf("rclone %s: %w", args[0], err)
	}
	return string(out), nil
}

// stream runs an rclone command, forwarding each line of combined
// stdout/stderr to onLine as it arrives. Lines are split on both "\n" and
// "\r" so rclone's progress updates surface live.
func (c *Client) stream(ctx context.Context, onLine func(string), args ...string) error {
	cmd := exec.CommandContext(ctx, c.bin(), args...)

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("starting rclone %s: %w", args[0], err)
	}
	// Close the parent's write end so the reader sees EOF when rclone exits.
	pw.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanner.Split(scanLinesOrCR)
	var recent []string
	for scanner.Scan() {
		line := scanner.Text()
		recent = append(recent, line)
		if len(recent) > 200 {
			recent = recent[len(recent)-200:]
		}
		if onLine != nil {
			onLine(line)
		}
	}
	pr.Close()

	if err := cmd.Wait(); err != nil {
		if summary := summarizeFailure(recent); summary != "" {
			return fmt.Errorf("rclone %s: %s: %w", args[0], summary, err)
		}
		return fmt.Errorf("rclone %s: %w", args[0], err)
	}
	return nil
}

// summarizeFailure turns verbose provider responses into a concise error.
func summarizeFailure(lines []string) string {
	joined := strings.Join(lines, "\n")
	lower := strings.ToLower(joined)
	switch {
	case strings.Contains(lower, "ratelimitexceeded"), strings.Contains(lower, "userratelimitexceeded"):
		return "Google Drive rate limit exceeded; retry later or request a higher quota"
	case strings.Contains(lower, "storagequotaexceeded"):
		return "Google Drive storage quota exceeded; free space or increase storage"
	}

	message := jsonStringField(lines, "message")
	reason := jsonStringField(lines, "reason")
	if message != "" && reason != "" && !strings.Contains(strings.ToLower(message), strings.ToLower(reason)) {
		return message + " (" + reason + ")"
	}
	if message != "" {
		return message
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "ERROR :") && len(line) <= 500 {
			return line
		}
	}
	return ""
}

func jsonStringField(lines []string, field string) string {
	prefix := `"` + field + `":`
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), ","))
		var value string
		if json.Unmarshal([]byte(raw), &value) == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// scanLinesOrCR is a bufio.SplitFunc that yields a token at each "\n" or "\r",
// so carriage-return progress updates are delivered as separate lines.
func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// splitLines splits command output into non-empty, trimmed lines.
func splitLines(out string) []string {
	var lines []string
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}
	return lines
}
