package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dougmb/rcss-tui/config"
)

// TestFormatExcludes checks the mapping from user tokens/patterns to rclone
// --exclude patterns: bare tokens become "*.<ext>", patterns pass through, and
// ".*" also excludes dot-directories.
func TestFormatExcludes(t *testing.T) {
	cfg := config.Config{SkipFormats: []string{"tmp", ".*", "*.log", "node_modules/**", "   ", "DS_Store"}}
	got := formatExcludes(cfg)
	want := []string{"*.tmp", ".*", ".*/**", "*.log", "node_modules/**", "*.DS_Store"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("excludes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestMatchesSkip checks file-name matching against the skip patterns.
func TestMatchesSkip(t *testing.T) {
	cfg := config.Config{SkipFormats: []string{"tmp", ".*"}}
	cases := map[string]bool{
		"build.tmp": true,  // *.tmp
		".env":      true,  // .*
		"main.go":   false,
		"notes.txt": false,
	}
	for name, want := range cases {
		if got := matchesSkip(name, cfg); got != want {
			t.Errorf("matchesSkip(%q) = %v, want %v", name, got, want)
		}
	}
}

// seedProject creates a project dir with a recent file, an excluded file, an old
// file (48h), and a sub-directory. It returns the dir.
func seedProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("a.txt")
	write("b.tmp") // excluded by skip "tmp"
	old := write("old.txt")
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// TestCleanupLocalModel verifies the new local-cleanup semantics: off keeps
// everything; on with retention 0 deletes all non-excluded files; on with a
// retention window keeps recent files. Directories and excluded files are never
// removed.
func TestCleanupLocalModel(t *testing.T) {
	log, _ := NewLogger("", func(string) {}, false)
	defer log.Close()

	t.Run("off keeps everything", func(t *testing.T) {
		dir := seedProject(t)
		cfg := config.Config{DeleteAfterUpload: false, SkipFormats: []string{"tmp"}}
		deleted, errs := cleanupLocal(dir, cfg, log)
		if deleted != 0 || errs != 0 {
			t.Fatalf("deleted=%d errs=%d, want 0/0", deleted, errs)
		}
		if !exists(filepath.Join(dir, "a.txt")) || !exists(filepath.Join(dir, "old.txt")) {
			t.Error("files were deleted while delete-after-upload is off")
		}
	})

	t.Run("on retention 0 deletes all but excluded", func(t *testing.T) {
		dir := seedProject(t)
		cfg := config.Config{DeleteAfterUpload: true, RetentionDays: 0, SkipFormats: []string{"tmp"}}
		deleted, _ := cleanupLocal(dir, cfg, log)
		if deleted != 2 { // a.txt + old.txt
			t.Errorf("deleted = %d, want 2", deleted)
		}
		if exists(filepath.Join(dir, "a.txt")) || exists(filepath.Join(dir, "old.txt")) {
			t.Error("expected a.txt and old.txt removed")
		}
		if !exists(filepath.Join(dir, "b.tmp")) {
			t.Error("excluded b.tmp must be kept")
		}
		if !exists(filepath.Join(dir, "sub")) {
			t.Error("directories must be kept")
		}
	})

	t.Run("on retention 1 keeps recent", func(t *testing.T) {
		dir := seedProject(t)
		cfg := config.Config{DeleteAfterUpload: true, RetentionDays: 1, SkipFormats: []string{"tmp"}}
		deleted, _ := cleanupLocal(dir, cfg, log)
		if deleted != 1 { // only old.txt
			t.Errorf("deleted = %d, want 1", deleted)
		}
		if exists(filepath.Join(dir, "old.txt")) {
			t.Error("old.txt should be removed")
		}
		if !exists(filepath.Join(dir, "a.txt")) {
			t.Error("recent a.txt should be kept")
		}
	})
}
