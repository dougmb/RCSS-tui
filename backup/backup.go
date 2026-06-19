// Package backup ports the business logic of the original RCSS Bash scripts
// (uploadBackup.sh, cleanRemoteBackups.sh, restoreBackup.sh) to Go. It drives
// the rclone binary through the rclone package and is shared by both the TUI
// and the headless cron subcommands.
//
// The safety invariants of the original scripts are preserved:
//   - Upload performs local cleanup ONLY on a successful per-project upload.
//   - Clean refuses to delete remote files unless a recent backup exists
//     (safety lock), unless explicitly forced.
package backup

import (
	"strings"

	"github.com/dougmb/rcss-tui/config"
)

// remoteDest returns "<remote>/<destination>" — the base remote path holding
// all project folders, e.g. "douglas:/Backups", or just "douglas:" when the
// destination is empty (backups at the account root). Delegates to
// config.RemoteBase so display and path-building stay consistent.
func remoteDest(cfg config.Config) string {
	return cfg.RemoteBase()
}

// joinRemote joins remote path segments with single slashes, trimming any
// stray slashes between parts while preserving the remote-name colon. Empty
// segments (e.g. an unset project when restoring a root-level file) are
// dropped so no double slash appears.
func joinRemote(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for i, p := range parts {
		if i == 0 {
			cleaned = append(cleaned, strings.TrimRight(p, "/"))
			continue
		}
		if trimmed := strings.Trim(p, "/"); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "/")
}

// isIgnored reports whether a project folder name should be skipped: hidden
// folders (starting with ".") and any name listed in cfg.IgnoredFolders.
func isIgnored(name string, cfg config.Config) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, ig := range cfg.IgnoredFolders {
		if ig == name {
			return true
		}
	}
	return false
}
