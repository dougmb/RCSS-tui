package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLog writes content to a temp log file and returns its path.
func writeLog(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "backup.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLastRunPicksMostRecentBlock(t *testing.T) {
	// Two SYNC SUMMARY blocks; LastRun must return the second (latest) one.
	const log = `[2026-06-16 10:00:00] [INFO   ] Starting backup synchronization...
════════════════════════════════════════════════
  SYNC SUMMARY — 2026-06-16 10:00:30
════════════════════════════════════════════════
  Status            : PARTIAL
  Duration          : 30s
════════════════════════════════════════════════

[2026-06-17 14:30:00] [INFO   ] Starting backup synchronization...
════════════════════════════════════════════════
  SYNC SUMMARY — 2026-06-17 14:30:42
════════════════════════════════════════════════
  Status            : SUCCESS
  Duration          : 42s
════════════════════════════════════════════════
`
	info, ok, err := LastRun(writeLog(t, log))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a recorded run")
	}
	if got := info.Time.Format("2006-01-02 15:04:05"); got != "2026-06-17 14:30:42" {
		t.Errorf("wrong time: %s", got)
	}
	if info.Status != "SUCCESS" {
		t.Errorf("wrong status: %q", info.Status)
	}
}

func TestLastRunMissingFile(t *testing.T) {
	_, ok, err := LastRun(filepath.Join(t.TempDir(), "does-not-exist.log"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if ok {
		t.Error("expected ok=false for missing log")
	}
}

func TestLastRunEmptyPath(t *testing.T) {
	if _, ok, err := LastRun(""); err != nil || ok {
		t.Errorf("empty path: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}
