package backup

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// ListProjects returns the project folders available on the remote
// (<remote>/<dest>/), with trailing slashes stripped. Mirrors
// `rclone lsf --dirs-only`.
func ListProjects(ctx context.Context, cfg config.Config, rc *rclone.Client) ([]string, error) {
	entries, err := rc.Lsf(ctx, remoteDest(cfg)+"/", rclone.LsfOptions{Mode: rclone.LsfDirsOnly})
	if err != nil {
		return nil, err
	}
	projects := make([]string, 0, len(entries))
	for _, e := range entries {
		projects = append(projects, strings.TrimSuffix(e, "/"))
	}
	return projects, nil
}

// ListFiles returns the files inside a remote project, newest first (reverse
// sorted by name, matching restoreBackup.sh's `sort -r`). Mirrors
// `rclone lsf --files-only`.
func ListFiles(ctx context.Context, cfg config.Config, rc *rclone.Client, project string) ([]string, error) {
	path := joinRemote(cfg.RemoteName, cfg.DriveDestination, project) + "/"
	files, err := rc.Lsf(ctx, path, rclone.LsfOptions{Mode: rclone.LsfFilesOnly})
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// RestoreOptions tweaks a Restore run.
type RestoreOptions struct {
	// ShowProgress adds rclone's -P progress output.
	ShowProgress bool
	// Verbose adds rclone's -v output.
	Verbose bool
	// DryRun previews the download without writing files.
	DryRun bool
	// OutputPath overrides the download destination. When empty, the file is
	// restored to <BackupRoot>/<project>/.
	OutputPath string
}

// Restore ports restoreBackup.sh's download step (selection is handled by the
// caller/TUI): it downloads a single file from <remote>/<dest>/<project>/ to
// the local destination via rclone copy with --ignore-times. The destination
// directory is created if needed.
func Restore(ctx context.Context, cfg config.Config, rc *rclone.Client, log *Logger, project, file string, opts RestoreOptions) error {
	if project == "" || file == "" {
		return fmt.Errorf("restore requires both a project and a file")
	}

	localPath := opts.OutputPath
	if localPath == "" {
		if cfg.BackupRoot == "" {
			return fmt.Errorf("config incomplete: backup_root not set")
		}
		localPath = joinRemoteLocal(cfg.BackupRoot, project)
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", localPath, err)
	}

	src := joinRemote(cfg.RemoteName, cfg.DriveDestination, project, file)
	log.Infof("Downloading %s to %s...", file, localPath)

	copyOpts := rclone.CopyOptions{
		IgnoreTimes: true,
		Progress:    opts.ShowProgress,
		Verbose:     opts.Verbose,
		DryRun:      opts.DryRun,
	}
	if err := rc.Copy(ctx, src, localPath+"/", copyOpts, log.Raw); err != nil {
		log.Errorf("Restore failed: %v", err)
		return err
	}

	log.Infof("Procedure completed.")
	return nil
}

// joinRemoteLocal joins a local base path with a project folder, keeping it
// OS-appropriate. (Local paths use the filesystem separator, not the remote
// slash join.)
func joinRemoteLocal(base, project string) string {
	return strings.TrimRight(base, string(os.PathSeparator)) + string(os.PathSeparator) + project
}
