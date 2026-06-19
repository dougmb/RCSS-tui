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

// RemoteEntry is one item in a remote listing: a project folder or a file.
type RemoteEntry struct {
	Name  string
	IsDir bool
}

// isDirNotFound reports whether err is rclone's "directory not found" — the
// expected state before any backup has been uploaded (the destination folder
// doesn't exist yet). Callers treat it as an empty listing, not a failure.
func isDirNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "directory not found")
}

// ListTopLevel lists the destination root: project folders (IsDir) and loose
// files. Returns an empty slice (no error) when the destination doesn't exist
// yet. Directories are listed before files, each group reverse-sorted by name.
func ListTopLevel(ctx context.Context, cfg config.Config, rc *rclone.Client) ([]RemoteEntry, error) {
	lines, err := rc.Lsf(ctx, remoteDest(cfg)+"/", rclone.LsfOptions{Mode: rclone.LsfAll})
	if isDirNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var dirs, files []RemoteEntry
	for _, l := range lines {
		if strings.HasSuffix(l, "/") {
			dirs = append(dirs, RemoteEntry{Name: strings.TrimSuffix(l, "/"), IsDir: true})
		} else {
			files = append(files, RemoteEntry{Name: l})
		}
	}
	reverseByName(dirs)
	reverseByName(files)
	return append(dirs, files...), nil
}

// ListFiles returns the files inside a remote project, newest first (reverse
// sorted by name, matching restoreBackup.sh's `sort -r`). Mirrors
// `rclone lsf --files-only`. An absent project yields an empty slice.
func ListFiles(ctx context.Context, cfg config.Config, rc *rclone.Client, project string) ([]string, error) {
	path := joinRemote(cfg.RemoteName, cfg.RemoteDestination, project) + "/"
	files, err := rc.Lsf(ctx, path, rclone.LsfOptions{Mode: rclone.LsfFilesOnly})
	if isDirNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

func reverseByName(es []RemoteEntry) {
	sort.Slice(es, func(i, j int) bool { return es[i].Name > es[j].Name })
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
	// restored to <RestoreDestination or SourceRoot>/<project>/.
	OutputPath string
}

// Restore ports restoreBackup.sh's download step (selection is handled by the
// caller/TUI): it downloads a single file from <remote>/<dest>/<project>/ to
// the local destination via rclone copy with --ignore-times. The destination
// directory is created if needed.
func Restore(ctx context.Context, cfg config.Config, rc *rclone.Client, log *Logger, project, file string, opts RestoreOptions) error {
	if file == "" {
		return fmt.Errorf("restore requires a file")
	}

	localPath := opts.OutputPath
	if localPath == "" {
		var err error
		if localPath, err = RestoreTarget(cfg, project); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", localPath, err)
	}

	src := joinRemote(cfg.RemoteName, cfg.RemoteDestination, project, file)
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

// RestoreTarget returns the default local folder a project's files restore to:
// <RestoreDestination or SourceRoot>/<project> (root-level files, project == "",
// land directly in the base). The configured restore destination wins;
// otherwise it falls back to the backup source so existing configs keep their
// previous behavior. The TUI uses this to pre-fill the (editable) restore
// destination, so the confirm field matches what Restore would do.
func RestoreTarget(cfg config.Config, project string) (string, error) {
	base := cfg.RestoreDestination
	if base == "" {
		base = cfg.SourceRoot
	}
	if base == "" {
		return "", fmt.Errorf("config incomplete: set a restore destination or source_root")
	}
	if project != "" {
		return joinRemoteLocal(base, project), nil
	}
	return base, nil
}

// joinRemoteLocal joins a local base path with a project folder, keeping it
// OS-appropriate. (Local paths use the filesystem separator, not the remote
// slash join.)
func joinRemoteLocal(base, project string) string {
	return strings.TrimRight(base, string(os.PathSeparator)) + string(os.PathSeparator) + project
}
