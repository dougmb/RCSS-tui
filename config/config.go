// Package config defines the RCSS configuration model and handles loading and
// saving it as a single TOML file under the user's config directory
// (~/.config/rcss/config.toml). It replaces the legacy backup.env used by the
// original Bash scripts; no secrets live here (rclone keeps credentials in its
// own config) — only paths, the remote name, and retention settings.
//
// RCSS supports multiple isolated accounts, one per rclone remote. A Store
// holds every account plus the name of the active one; each account is a Config
// with its own folders, retention, and log. The remote name is the account key.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// AppDir is the sub-directory of the user's config dir holding RCSS files.
const AppDir = "rcss"

// FileName is the TOML config file name within AppDir.
const FileName = "config.toml"

// Config holds the settings for a single account (one rclone remote). Every
// account is fully isolated: its own sync root, destination, retention, ignored
// folders, and log file. RemoteName is the account key.
type Config struct {
	// RemoteName is the rclone remote as configured via `rclone config`,
	// including the trailing colon (e.g. "drive:" or "douglas:"). It is the
	// account's unique key.
	RemoteName string `toml:"remote_name"`
	// SourceFolders are the local folders to back up. Each one is uploaded as
	// its own backup at <RemoteName>/<RemoteDestination>/<folder-basename>.
	SourceFolders []string `toml:"source_folders"`
	// SourceRoot is the deprecated single-folder field from older RCSS versions.
	// It is read only to migrate into SourceFolders on load (see LoadStore) and
	// is otherwise unused; omitempty keeps it out of freshly written configs.
	SourceRoot string `toml:"source_root,omitempty"`
	// RemoteDestination is the destination folder on the remote; backups live
	// at <RemoteName>/<RemoteDestination>/<project>/.
	RemoteDestination string `toml:"remote_destination"`

	// DeleteAfterUpload enables local cleanup after a successful upload. When
	// off, local files are never deleted. When on, files older than
	// RetentionDays are removed (RetentionDays == 0 removes them all).
	DeleteAfterUpload bool `toml:"delete_after_upload"`
	// RetentionDays is how many days of local backups to keep when
	// DeleteAfterUpload is on: files older than this are removed after a
	// successful upload (0 = delete all immediately). Ignored when
	// DeleteAfterUpload is off.
	RetentionDays int `toml:"retention_days"`

	// RemoteRetentionDays controls deletion of CLOUD files during clean.
	RemoteRetentionDays int `toml:"remote_retention_days"`
	// RemoteCleanupSafetyDays blocks remote cleanup unless a backup newer than
	// this exists on the remote — guards against wiping history if uploads
	// have silently stopped.
	RemoteCleanupSafetyDays int `toml:"remote_cleanup_safety_days"`

	// SkipFormats are file patterns excluded from uploads via rclone --exclude.
	// A bare token like "tmp" matches "*.tmp"; an entry containing a glob or a
	// leading dot (e.g. ".*", "*.log", "node_modules/**") is used verbatim, so
	// ".*" skips dotfiles. Empty means nothing is skipped.
	SkipFormats []string `toml:"skip_formats"`
	// IgnoredFolders are directory names excluded from within each backup
	// (folded into rclone --exclude as "<name>/**"), e.g. node_modules.
	IgnoredFolders []string `toml:"ignored_folders"`

	// RestoreDestination is the local folder restored files are written to.
	// Empty falls back to the matching source folder (see backup.RestoreTarget),
	// so a backup restores to the folder it came from.
	RestoreDestination string `toml:"restore_destination"`

	// LogFile is the path to the append-only backup log. Empty means a per-account
	// default location is used (see ResolveLogFile).
	LogFile string `toml:"log_file"`
}

// Store is the on-disk model: every isolated account plus the active one.
type Store struct {
	// ActiveAccount is the RemoteName of the account commands act on by default.
	ActiveAccount string `toml:"active_account"`
	// Accounts holds one Config per account, keyed by its RemoteName.
	Accounts []Config `toml:"accounts"`
}

// Default returns a Config populated with the recommended defaults: no source
// folders yet, backups go to the account root (empty RemoteDestination), 15 days
// remote retention, a 2-day cleanup safety window, with local delete-after-upload
// off and no skip patterns. RetentionDays is 0 so that, if delete-after-upload is
// later turned on, it removes everything unless a longer window is set.
func Default() Config {
	return Config{
		RemoteDestination:       "",
		RetentionDays:           0,
		RemoteRetentionDays:     15,
		RemoteCleanupSafetyDays: 2,
	}
}

// NewAccount returns the recommended defaults for a freshly added account on the
// given rclone remote.
func NewAccount(remote string) Config {
	c := Default()
	c.RemoteName = remote
	return c
}

// RemoteBase returns the remote path holding the project folders, for display
// and path-building: "<remote>/<destination>", or just "<remote>" when the
// destination is empty (backups at the account root). The remote name keeps its
// trailing colon; stray slashes are trimmed so the root renders without a
// trailing "/".
func (c Config) RemoteBase() string {
	base := strings.TrimRight(c.RemoteName, "/")
	dest := strings.Trim(c.RemoteDestination, "/")
	if dest == "" {
		return base
	}
	return base + "/" + dest
}

// --- Store operations ---

// Names returns the configured account names (RemoteName of each account).
func (s *Store) Names() []string {
	out := make([]string, 0, len(s.Accounts))
	for _, a := range s.Accounts {
		out = append(out, a.RemoteName)
	}
	return out
}

// Get returns the account with the given name.
func (s *Store) Get(name string) (Config, bool) {
	for _, a := range s.Accounts {
		if a.RemoteName == name {
			return a, true
		}
	}
	return Config{}, false
}

// Has reports whether an account exists for the given remote.
func (s *Store) Has(name string) bool {
	_, ok := s.Get(name)
	return ok
}

// Active returns the active account, or false when none is set.
func (s *Store) Active() (Config, bool) {
	if s.ActiveAccount == "" {
		return Config{}, false
	}
	return s.Get(s.ActiveAccount)
}

// Upsert adds or replaces an account, keyed by its RemoteName.
func (s *Store) Upsert(c Config) {
	for i, a := range s.Accounts {
		if a.RemoteName == c.RemoteName {
			s.Accounts[i] = c
			return
		}
	}
	s.Accounts = append(s.Accounts, c)
}

// SetActive marks an account active by name.
func (s *Store) SetActive(name string) { s.ActiveAccount = name }

// migrateSourceRoots folds the deprecated SourceRoot of each account into its
// SourceFolders list (when SourceFolders is empty) and clears SourceRoot.
// Returns whether any account changed, so the caller can persist.
func (s *Store) migrateSourceRoots() bool {
	changed := false
	for i := range s.Accounts {
		if s.Accounts[i].SourceRoot == "" {
			continue
		}
		if len(s.Accounts[i].SourceFolders) == 0 {
			s.Accounts[i].SourceFolders = []string{s.Accounts[i].SourceRoot}
		}
		s.Accounts[i].SourceRoot = ""
		changed = true
	}
	return changed
}

// Remove deletes an account's stored settings (the rclone remote itself is
// untouched). If it was active, the active account falls back to the first
// remaining one, or none.
func (s *Store) Remove(name string) {
	kept := s.Accounts[:0]
	for _, a := range s.Accounts {
		if a.RemoteName != name {
			kept = append(kept, a)
		}
	}
	s.Accounts = kept
	if s.ActiveAccount == name {
		s.ActiveAccount = ""
		if len(s.Accounts) > 0 {
			s.ActiveAccount = s.Accounts[0].RemoteName
		}
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

// sanitizeName maps an account name to a filesystem/scheduler-safe token,
// e.g. "drive:" → "drive". Used for per-account default log file names.
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "default"
	}
	return out
}

// ResolveLogFile returns the effective backup-log path: c.LogFile if set,
// otherwise a per-account file inside the config dir (backup-<account>.log), so
// each account keeps an isolated log.
func (c Config) ResolveLogFile() (string, error) {
	if c.LogFile != "" {
		return c.LogFile, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	name := "backup.log"
	if c.RemoteName != "" {
		name = "backup-" + sanitizeName(c.RemoteName) + ".log"
	}
	return filepath.Join(dir, name), nil
}

// LoadStore reads the multi-account store from its default path. On first run
// (file missing) it writes an empty store. A legacy single-account config (flat
// TOML from older RCSS versions) is migrated into one account on load.
func LoadStore() (*Store, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	var st Store
	_, err = toml.DecodeFile(path, &st)
	if errors.Is(err, os.ErrNotExist) {
		st = Store{}
		if err := st.Save(); err != nil {
			return nil, fmt.Errorf("writing initial config: %w", err)
		}
		return &st, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	// Migrate a legacy flat config (no [[accounts]] table) into one account.
	if len(st.Accounts) == 0 {
		var legacy Config
		if _, lerr := toml.DecodeFile(path, &legacy); lerr == nil && legacy.RemoteName != "" {
			st.Accounts = []Config{legacy}
			st.ActiveAccount = legacy.RemoteName
			_ = st.Save()
		}
	}

	// Migrate the deprecated single source_root into the SourceFolders list.
	if st.migrateSourceRoots() {
		_ = st.Save()
	}

	// Ensure the active pointer is valid.
	if _, ok := st.Active(); !ok && len(st.Accounts) > 0 {
		st.ActiveAccount = st.Accounts[0].RemoteName
	}
	return &st, nil
}

// Save writes the store as TOML to its default path, creating the config
// directory if needed.
func (s *Store) Save() error {
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

	if err := toml.NewEncoder(f).Encode(s); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}

// Validate checks that the required fields are set. It mirrors the original
// scripts, which abort when BACKUP_ROOT or RCLONE_REMOTE are missing/invalid.
// RemoteDestination may be empty: that means backups go to the account root.
func (c Config) Validate() error {
	var missing []string
	if c.RemoteName == "" {
		missing = append(missing, "remote_name")
	}
	if len(c.SourceFolders) == 0 {
		missing = append(missing, "source_folders")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config incomplete: %v not set", missing)
	}
	if c.RetentionDays < 0 || c.RemoteRetentionDays < 0 || c.RemoteCleanupSafetyDays < 0 {
		return errors.New("retention values must be non-negative")
	}
	return nil
}
