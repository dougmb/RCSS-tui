package backup

import (
	"os"
	"testing"

	"github.com/dougmb/rcss-tui/config"
)

// TestRestoreTarget covers the default local restore destination: the configured
// RestoreDestination wins, otherwise it falls back to SourceRoot, a project adds
// a sub-folder, and an unset config errors.
func TestRestoreTarget(t *testing.T) {
	sep := string(os.PathSeparator)

	// RestoreDestination wins over SourceRoot; project adds a sub-folder.
	cfg := config.Config{SourceRoot: "/src", RestoreDestination: "/restore"}
	if got, err := RestoreTarget(cfg, "proj"); err != nil || got != "/restore"+sep+"proj" {
		t.Errorf("project restore: got %q err=%v", got, err)
	}
	// A root-level file (no project) lands directly in the base.
	if got, err := RestoreTarget(cfg, ""); err != nil || got != "/restore" {
		t.Errorf("root restore: got %q err=%v", got, err)
	}
	// Falls back to SourceRoot when RestoreDestination is empty.
	if got, err := RestoreTarget(config.Config{SourceRoot: "/src"}, "proj"); err != nil || got != "/src"+sep+"proj" {
		t.Errorf("fallback restore: got %q err=%v", got, err)
	}
	// Errors when neither destination is configured.
	if _, err := RestoreTarget(config.Config{}, "proj"); err == nil {
		t.Error("expected error when no destination configured")
	}
}
