// Package cron manages a single delimited block in the user's crontab (no
// root, no system files). RCSS owns only the lines between its markers; every
// other crontab entry is preserved untouched. The scheduled lines invoke the
// rcss binary in headless mode (e.g. `rcss upload`).
package cron

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

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

// ManagedLines returns the cron lines currently inside the RCSS block (without
// the markers), or nil if the block is absent.
func ManagedLines() ([]string, error) {
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

// SetManaged replaces the RCSS block with the given cron lines. Empty input
// removes the block entirely. Other crontab entries are preserved.
func SetManaged(lines []string) error {
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

// Remove deletes the RCSS-managed block from the crontab.
func Remove() error { return SetManaged(nil) }
