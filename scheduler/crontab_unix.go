//go:build !windows

package scheduler

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const backendName = "crontab"

const (
	beginMarker = "# >>> RCSS-managed >>>"
	endMarker   = "# <<< RCSS-managed <<<"
)

// readCrontab returns the user's current crontab, or "" if none exists.
func readCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		// `crontab -l` exits non-zero when there is no crontab yet.
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", fmt.Errorf("reading crontab: %w", err)
	}
	return string(out), nil
}

// writeCrontab installs content as the user's crontab via `crontab -`.
func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing crontab: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// stripBlock returns content with the RCSS-managed block (markers included)
// removed, leaving all other lines intact.
func stripBlock(content string) []string {
	var kept []string
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.TrimSpace(line) == beginMarker:
			inBlock = true
		case strings.TrimSpace(line) == endMarker:
			inBlock = false
		case !inBlock:
			kept = append(kept, line)
		}
	}
	// Trim trailing empties left behind.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	return kept
}

// managedLines returns the cron lines currently inside the RCSS block (without
// the markers), or nil if the block is absent.
func managedLines() ([]string, error) {
	content, err := readCrontab()
	if err != nil {
		return nil, err
	}
	var lines []string
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case beginMarker:
			inBlock = true
		case endMarker:
			inBlock = false
		default:
			if inBlock && strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines, nil
}

// setManaged replaces the RCSS block with the given cron lines. Empty input
// removes the block entirely. Other crontab entries are preserved.
func setManaged(lines []string) error {
	content, err := readCrontab()
	if err != nil {
		return err
	}
	kept := stripBlock(content)

	out := strings.Join(kept, "\n")
	if len(lines) > 0 {
		block := append([]string{beginMarker}, lines...)
		block = append(block, endMarker)
		if out != "" {
			out += "\n"
		}
		out += strings.Join(block, "\n")
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return writeCrontab(out)
}

// apply rewrites the managed block: it keeps every line that doesn't belong to
// account, then appends one line per job for this account. Daily jobs use
// "* * *" for the day fields; weekly jobs run on Sunday (dow 0). Each line runs
// the rcss binary headless with --account so the right account is selected, and
// appends stdout/stderr to logPath.
func apply(account string, jobs []Job, exe, logPath string) error {
	existing, err := managedLines()
	if err != nil {
		return err
	}
	var lines []string
	for _, ln := range existing {
		if lineAccount(ln) != account {
			lines = append(lines, ln) // preserve other accounts' jobs
		}
	}
	for _, j := range jobs {
		dow := "*"
		if j.Weekly {
			dow = "0"
		}
		lines = append(lines, fmt.Sprintf(`%d %d * * %s %s %s --account %q >> %s 2>&1`,
			j.Min, j.Hour, dow, exe, j.Kind.Arg(), account, logPath))
	}
	return setManaged(lines)
}

// current parses the managed crontab lines that belong to account back into
// jobs. Lines that don't match the expected shape are skipped.
func current(account string) ([]Job, error) {
	lines, err := managedLines()
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, ln := range lines {
		if lineAccount(ln) != account {
			continue
		}
		f := strings.Fields(ln)
		if len(f) < 6 {
			continue
		}
		min, err1 := strconv.Atoi(f[0])
		hour, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		j := Job{Kind: Upload, Hour: hour, Min: min, Weekly: f[4] == "0"}
		for _, tok := range f[5:] {
			if tok == "clean" {
				j.Kind = Clean
				break
			}
			if tok == "upload" {
				break
			}
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// lineAccount returns the account a managed cron line targets (the token after
// --account, unquoted), or "" if it carries none.
func lineAccount(line string) string {
	f := strings.Fields(line)
	for i, tok := range f {
		if tok == "--account" && i+1 < len(f) {
			return strings.Trim(f[i+1], `"`)
		}
	}
	return ""
}
