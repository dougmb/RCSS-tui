package backup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// ErrPreviewOutdated means the remote candidate set changed after preview.
var ErrPreviewOutdated = errors.New("cleanup preview is outdated; run dry-run again")

type CleanupCandidate struct {
	Path    string
	Size    *int64
	ModTime time.Time
}

type CleanPreview struct {
	Candidates   []CleanupCandidate
	KnownBytes   int64
	UnknownSizes int
}

type CleanResult struct {
	Selected int
	Removed  int
	Failures int
}

func checkCleanSafety(ctx context.Context, cfg config.Config, rc *rclone.Client, force bool) error {
	if force {
		return nil
	}
	recent, err := rc.Lsf(ctx, remoteDest(cfg), rclone.LsfOptions{
		Mode: rclone.LsfFilesOnly, Recursive: true,
		MaxAge: fmt.Sprintf("%dd", cfg.RemoteCleanupSafetyDays),
	})
	if err != nil {
		return err
	}
	if len(recent) == 0 {
		return ErrNoRecentBackup
	}
	return nil
}

func listCleanupCandidates(ctx context.Context, cfg config.Config, rc *rclone.Client) ([]CleanupCandidate, error) {
	entries, err := rc.ListJSON(ctx, remoteDest(cfg), fmt.Sprintf("%dd", cfg.RemoteRetentionDays))
	if err != nil {
		return nil, err
	}
	out := make([]CleanupCandidate, len(entries))
	for i, e := range entries {
		out[i] = CleanupCandidate{Path: e.Path, Size: e.Size, ModTime: e.ModTime}
	}
	return out, nil
}

// PreviewClean validates the safety lock, lists the exact candidate set and
// asks rclone to dry-run only that set.
func PreviewClean(ctx context.Context, cfg config.Config, rc *rclone.Client, log *Logger, force bool) (CleanPreview, error) {
	if cfg.RemoteName == "" {
		return CleanPreview{}, errors.New("config incomplete: remote_name not set")
	}
	if err := checkCleanSafety(ctx, cfg, rc, force); err != nil {
		return CleanPreview{}, err
	}
	candidates, err := listCleanupCandidates(ctx, cfg, rc)
	if err != nil {
		return CleanPreview{}, err
	}
	p := CleanPreview{Candidates: candidates}
	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.Path
		if c.Size == nil {
			p.UnknownSizes++
		} else {
			p.KnownBytes += *c.Size
		}
	}
	if len(paths) > 0 {
		level := ""
		if log != nil && log.IsVerbose() {
			level = "INFO"
		}
		if err := rc.DeleteFiles(ctx, remoteDest(cfg), paths, true, level, loggerSink(log)); err != nil {
			return CleanPreview{}, err
		}
	}
	return p, nil
}

// ExecuteClean rechecks both the safety lock and the complete remote candidate
// set before deleting only selected paths.
func ExecuteClean(ctx context.Context, cfg config.Config, rc *rclone.Client, log *Logger, preview CleanPreview, selected []string, force bool) (CleanResult, error) {
	result := CleanResult{Selected: len(selected)}
	if len(selected) == 0 {
		return result, errors.New("no cleanup files selected")
	}
	if err := checkCleanSafety(ctx, cfg, rc, force); err != nil {
		return result, err
	}
	current, err := listCleanupCandidates(ctx, cfg, rc)
	if err != nil {
		return result, err
	}
	if !sameCandidates(preview.Candidates, current) {
		return result, ErrPreviewOutdated
	}
	allowed := make(map[string]bool, len(preview.Candidates))
	for _, c := range preview.Candidates {
		allowed[c.Path] = true
	}
	for _, path := range selected {
		if !allowed[path] {
			return result, fmt.Errorf("selected path not in preview: %q", path)
		}
	}
	level := ""
	if log != nil && log.IsVerbose() {
		level = "INFO"
	}
	if err := rc.DeleteFiles(ctx, remoteDest(cfg), selected, false, level, loggerSink(log)); err != nil {
		result.Failures = len(selected)
		return result, err
	}
	result.Removed = len(selected)
	return result, nil
}

func loggerSink(log *Logger) func(string) {
	if log == nil {
		return nil
	}
	return log.Raw
}

func sameCandidates(a, b []CleanupCandidate) bool {
	if len(a) != len(b) {
		return false
	}
	return slices.EqualFunc(a, b, func(x, y CleanupCandidate) bool {
		if x.Path != y.Path || !x.ModTime.Equal(y.ModTime) {
			return false
		}
		if x.Size == nil || y.Size == nil {
			return x.Size == nil && y.Size == nil
		}
		return *x.Size == *y.Size
	})
}
