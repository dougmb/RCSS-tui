package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

func TestCleanSafetyLock(t *testing.T) {
	out := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_FAKE_OUTPUT", out)
	rc := &rclone.Client{Bin: fakeRclone(t)}
	log, err := NewLogger("", func(string) {}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	cfg := config.Config{
		RemoteName:              "drive:",
		RemoteDestination:       "backups",
		RemoteRetentionDays:     15,
		RemoteCleanupSafetyDays: 2,
	}
	err = Clean(t.Context(), cfg, rc, log, CleanOptions{})
	if !errors.Is(err, ErrNoRecentBackup) {
		t.Fatalf("Clean error = %v, want ErrNoRecentBackup", err)
	}

	calls, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(calls), "delete ") {
		t.Fatalf("safety lock allowed delete call:\n%s", calls)
	}
}

func TestCleanRecentBackupUsesDryRun(t *testing.T) {
	out := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_FAKE_OUTPUT", out)
	t.Setenv("RCSS_FAKE_LSF", "project/recent.zip")
	rc := &rclone.Client{Bin: fakeRclone(t)}
	log, _ := NewLogger("", func(string) {}, false)
	defer log.Close()

	cfg := config.Config{
		RemoteName:              "drive:",
		RemoteDestination:       "backups",
		RemoteRetentionDays:     15,
		RemoteCleanupSafetyDays: 2,
	}
	if err := Clean(t.Context(), cfg, rc, log, CleanOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}

	calls, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(calls)
	for _, want := range []string{
		"lsf drive:/backups --files-only --recursive --max-age 2d",
		"delete drive:/backups --min-age 15d --dry-run",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("calls missing %q:\n%s", want, got)
		}
	}
}

func TestCleanForceBypassesSafetyLock(t *testing.T) {
	out := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_FAKE_OUTPUT", out)
	rc := &rclone.Client{Bin: fakeRclone(t)}
	log, _ := NewLogger("", func(string) {}, false)
	defer log.Close()

	cfg := config.Config{RemoteName: "drive:", RemoteRetentionDays: 7}
	if err := Clean(t.Context(), cfg, rc, log, CleanOptions{Force: true}); err != nil {
		t.Fatal(err)
	}

	calls, _ := os.ReadFile(out)
	got := string(calls)
	if strings.Contains(got, "lsf ") {
		t.Fatalf("force mode unexpectedly checked safety lock:\n%s", got)
	}
	if !strings.Contains(got, "delete drive: --min-age 7d") {
		t.Fatalf("force mode did not call delete:\n%s", got)
	}
}
