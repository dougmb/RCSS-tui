// Package config defines the RCSS configuration model and handles loading and
// saving it as a single TOML file under the user's config directory
// (~/.config/rcss/config.toml). It replaces the legacy backup.env used by the
// original Bash scripts; no secrets live here (rclone keeps credentials in its
// own config) — only paths, the remote name, and retention settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// AppDir is the sub-directory of the user's config dir holding RCSS files.
const AppDir = "rcss"

// FileName is the TOML config file name within AppDir.
const FileName = "config.toml"

// Config mirrors the settings the original scripts read from backup.env, with
// one combined source of truth. Field order matches how it is written to disk.
type Config struct {
	// RemoteName is the rclone remote as configured via `rclone config`,
	// including the trailing colon (e.g. "drive:" or "douglas:").
	RemoteName string `toml:"remote_name"`
	// BackupRoot is the local directory whose sub-folders are the projects.
	BackupRoot string `toml:"backup_root"`
	// DriveDestination is the destination folder on the remote; backups live
	// at <RemoteName>/<DriveDestination>/<project>/.
	DriveDestination string `toml:"drive_destination"`

	// RetentionDays controls deletion of LOCAL files after a successful
	// upload (files older than this are removed). Ignored when
	// DeleteAfterUpload is true.
	RetentionDays int `toml:"retention_days"`
	// RemoteRetentionDays controls deletion of CLOUD files during clean.
	RemoteRetentionDays int `toml:"remote_retention_days"`
	// RemoteCleanupSafetyDays blocks remote cleanup unless a backup newer than
	// this exists on the remote — guards against wiping history if uploads
	// have silently stopped.
	RemoteCleanupSafetyDays int `toml:"remote_cleanup_safety_days"`

	// DeleteAfterUpload deletes all local files immediately after a successful
	// upload, bypassing RetentionDays.
	DeleteAfterUpload bool `toml:"delete_after_upload"`
	// SkipDotfiles excludes hidden files/folders (starting with ".") from
	// uploads, protecting files like .env, .git/, .ssh/.
	SkipDotfiles bool `toml:"skip_dotfiles"`
	// IgnoredFolders are sub-folders of BackupRoot never treated as projects.
	IgnoredFolders []string `toml:"ignored_folders"`

	// LogFile is the path to the append-only sync log. Empty means a default
	// location is used (see ResolveLogFile).
	LogFile string `toml:"log_file"`
}

// Default returns a Config populated with the recommended defaults: 1 day of
// local retention, 15 days remote, a 2-day cleanup safety window, with
// delete-after-upload and skip-dotfiles disabled.
func Default() Config {
	return Config{
		RemoteName:              "",
		BackupRoot:              "",
		DriveDestination:        "Backups",
		RetentionDays:           1,
		RemoteRetentionDays:     15,
		RemoteCleanupSafetyDays: 2,
		DeleteAfterUpload:       false,
		SkipDotfiles:            false,
		IgnoredFolders:          []string{"scripts", "config", "bin", "logs", "lost+found"},
		LogFile:                 "",
	}
}

// Dir returns the directory holding RCSS config files
// (e.g. ~/.config/rcss), respecting XDG_CONFIG_HOME via os.UserConfigDir.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(base, AppDir), nil
}

// Path returns the full path to the config TOML file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// ResolveLogFile returns the effective sync-log path: c.LogFile if set,
// otherwise sync.log inside the config dir.
func (c Config) ResolveLogFile() (string, error) {
	if c.LogFile != "" {
		return c.LogFile, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sync.log"), nil
}

// Load reads the config from its default path. On first run (file missing) it
// writes the recommended defaults to disk and returns them, so callers always
// get a usable Config.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	meta, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, os.ErrNotExist) {
		cfg = Default()
		if err := Save(cfg); err != nil {
			return Config{}, fmt.Errorf("writing initial config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	_ = meta
	return cfg, nil
}

// Save writes the config as TOML to its default path, creating the config
// directory if needed.
func Save(c Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir %s: %w", dir, err)
	}

	path := filepath.Join(dir, FileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}

// Validate checks that the required fields are set. It mirrors the original
// scripts, which abort when BACKUP_ROOT, RCLONE_REMOTE, or RETENTION_DAYS are
// missing/invalid.
func (c Config) Validate() error {
	var missing []string
	if c.RemoteName == "" {
		missing = append(missing, "remote_name")
	}
	if c.BackupRoot == "" {
		missing = append(missing, "backup_root")
	}
	if c.DriveDestination == "" {
		missing = append(missing, "drive_destination")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config incomplete: %v not set", missing)
	}
	if c.RetentionDays < 0 || c.RemoteRetentionDays < 0 || c.RemoteCleanupSafetyDays < 0 {
		return errors.New("retention values must be non-negative")
	}
	return nil
}
