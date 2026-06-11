package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// UploadOptions tweaks an Upload run beyond the persisted config.
type UploadOptions struct {
	// ShowProgress adds rclone's -P progress output to the streamed lines.
	ShowProgress bool
}

// UploadResult summarizes a completed Upload run, matching the SYNC SUMMARY
// block written to the log.
type UploadResult struct {
	Status       string // "SUCCESS" or "PARTIAL"
	Duration     time.Duration
	UploadErrors int
	FilesDeleted int
	DeleteErrors int
}

// Upload ports uploadBackup.sh: it iterates the sub-directories of
// cfg.BackupRoot, skips dotfolders and ignored folders, and uploads each
// "project" to <remote>/<dest>/<project> via rclone copy. Local cleanup runs
// ONLY inside the success branch of each upload (the core safety invariant):
// when DeleteAfterUpload is set every top-level file is removed, otherwise
// only files older than RetentionDays. A SYNC SUMMARY block is appended to the
// log at the end of every run.
func Upload(ctx context.Context, cfg config.Config, rc *rclone.Client, log *Logger, opts UploadOptions) (UploadResult, error) {
	res := UploadResult{Status: "SUCCESS"}

	if err := cfg.Validate(); err != nil {
		log.Errorf("%v", err)
		return res, err
	}
	if fi, err := os.Stat(cfg.BackupRoot); err != nil || !fi.IsDir() {
		err := fmt.Errorf("backup root directory not found: %s", cfg.BackupRoot)
		log.Errorf("%v", err)
		return res, err
	}

	overallStart := time.Now()
	log.Infof("Starting backup synchronization...")
	log.Infof("Settings: root=%s | remote=%s | retention=%dd | skip_dotfiles=%t | delete_after_upload=%t",
		cfg.BackupRoot, cfg.RemoteName, cfg.RetentionDays, cfg.SkipDotfiles, cfg.DeleteAfterUpload)

	entries, err := os.ReadDir(cfg.BackupRoot)
	if err != nil {
		log.Errorf("reading backup root: %v", err)
		return res, err
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isIgnored(name, cfg) {
			log.Verbosef("   - Skipping ignored/reserved folder: %s", name)
			continue
		}

		projectPath := filepath.Join(cfg.BackupRoot, name)
		log.Infof("→ Processing project: %s", name)
		stepStart := time.Now()

		copyOpts := rclone.CopyOptions{
			LogLevel:     log.RcloneLogLevel(),
			StatsOneLine: true,
			Stats:        "10s",
			Update:       true,
			UseMmap:      true,
			Retries:      3,
			Progress:     opts.ShowProgress,
		}
		if cfg.SkipDotfiles {
			copyOpts.Excludes = []string{".*", ".*/**"}
		}

		dst := joinRemote(cfg.RemoteName, cfg.DriveDestination, name)
		err := rc.Copy(ctx, projectPath, dst, copyOpts, log.Raw)
		if err != nil {
			log.Warnf("   ⚠ Sync failed for project %s. Local cleanup SKIPPED.", name)
			res.UploadErrors++
			log.Verbosef("   Project time: %s", time.Since(stepStart).Round(time.Second))
			continue
		}

		log.Infof("   ✓ Synchronized successfully.")

		// Local cleanup — ONLY reached on a successful upload.
		deleted, delErrs := cleanupLocal(projectPath, cfg, log)
		if deleted > 0 {
			log.Infof("   - Removed %d local files.", deleted)
		}
		res.FilesDeleted += deleted
		res.DeleteErrors += delErrs

		log.Verbosef("   Project time: %s", time.Since(stepStart).Round(time.Second))
	}

	res.Duration = time.Since(overallStart)
	if res.UploadErrors > 0 {
		res.Status = "PARTIAL"
	}

	log.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Infof("✅ Synchronization completed in %s", res.Duration.Round(time.Second))
	log.writeBlock(summaryBlock(cfg, res))

	if res.UploadErrors > 0 {
		return res, fmt.Errorf("%d project(s) failed to upload", res.UploadErrors)
	}
	return res, nil
}

// cleanupLocal removes top-level files from projectPath after a successful
// upload, returning the number deleted and the number of delete errors. With
// DeleteAfterUpload it removes every top-level file; otherwise only files
// older than RetentionDays. It never recurses and never removes directories,
// matching `find -maxdepth 1 -type f`.
func cleanupLocal(projectPath string, cfg config.Config, log *Logger) (deleted, delErrs int) {
	if cfg.DeleteAfterUpload {
		log.Verbosef("   Deleting all uploaded local files...")
	} else {
		log.Verbosef("   Cleaning local files older than %d days...", cfg.RetentionDays)
	}

	entries, err := os.ReadDir(projectPath)
	if err != nil {
		log.Warnf("   ⚠ Could not list local files in %s: %v", projectPath, err)
		return 0, 1
	}

	cutoff := time.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Skip irregular files (symlinks, sockets, …): -type f only.
		if !info.Mode().IsRegular() {
			continue
		}
		if !cfg.DeleteAfterUpload && !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(projectPath, e.Name())
		if err := os.Remove(path); err != nil {
			log.Warnf("   ⚠ Could not delete: %s", path)
			delErrs++
			continue
		}
		deleted++
	}
	return deleted, delErrs
}

// summaryBlock builds the fixed-width SYNC SUMMARY appended to the log.
func summaryBlock(cfg config.Config, res UploadResult) []string {
	secs := int(res.Duration.Round(time.Second).Seconds())
	return []string{
		"════════════════════════════════════════════════",
		fmt.Sprintf("  SYNC SUMMARY — %s", time.Now().Format("2006-01-02 15:04:05")),
		"════════════════════════════════════════════════",
		fmt.Sprintf("  Status            : %s", res.Status),
		fmt.Sprintf("  Duration          : %ds", secs),
		fmt.Sprintf("  Cloud Destination : %s/", remoteDest(cfg)),
		fmt.Sprintf("  Projects w/ Errors: %d", res.UploadErrors),
		fmt.Sprintf("  Files Removed (Local): %d", res.FilesDeleted),
		fmt.Sprintf("  Delete Errors     : %d", res.DeleteErrors),
		"  --- Flags ---",
		fmt.Sprintf("  skip_dotfiles     : %t", cfg.SkipDotfiles),
		fmt.Sprintf("  delete_after_upload: %t", cfg.DeleteAfterUpload),
		fmt.Sprintf("  retention_days    : %d", cfg.RetentionDays),
		"════════════════════════════════════════════════",
		"",
	}
}
