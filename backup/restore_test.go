package backup

import (
	"os"
	"testing"

	"github.com/dougmb/rcss-tui/config"
)

// TestRestoreTarget covers the default local restore destination: a configured
// RestoreDestination wins (project adds a sub-folder); otherwise the project is
// mapped back to the source folder it came from; an unmatched project errors.
func TestRestoreTarget(t *testing.T) {
	sep := string(os.PathSeparator)

	// RestoreDestination wins; project adds a sub-folder.
	cfg := config.Config{
		SourceFolders:      []string{"/home/me/work/website", "/home/me/notes"},
		RestoreDestination: "/restore",
	}
	if got, err := RestoreTarget(cfg, "proj"); err != nil || got != "/restore"+sep+"proj" {
		t.Errorf("project restore: got %q err=%v", got, err)
	}
	// A root-level file (no project) lands directly in the base.
	if got, err := RestoreTarget(cfg, ""); err != nil || got != "/restore" {
		t.Errorf("root restore: got %q err=%v", got, err)
	}
	// With no RestoreDestination, a backup restores to its original source folder.
	src := config.Config{SourceFolders: []string{"/home/me/work/website", "/home/me/notes"}}
	if got, err := RestoreTarget(src, "website"); err != nil || got != "/home/me/work/website" {
		t.Errorf("in-place restore: got %q err=%v", got, err)
	}
	// Errors when no destination and no matching source folder.
	if _, err := RestoreTarget(src, "unknown"); err == nil {
		t.Error("expected error when no matching backup source")
	}
}
