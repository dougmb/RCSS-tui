package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

func previewFixture(t *testing.T) (config.Config, *rclone.Client, string, string) {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "calls.txt")
	manifest := filepath.Join(t.TempDir(), "manifest.txt")
	t.Setenv("RCSS_FAKE_OUTPUT", calls)
	t.Setenv("RCSS_FAKE_MANIFEST", manifest)
	t.Setenv("RCSS_FAKE_LSF", "recent/file")
	t.Setenv("RCSS_FAKE_LSJSON", `[{"Path":"old/a","Size":10,"ModTime":"2024-01-01T00:00:00Z"},{"Path":"old/b","Size":-1,"ModTime":"2024-01-02T00:00:00Z"}]`)
	cfg := config.Config{RemoteName: "drive:", RemoteDestination: "backups", RemoteRetentionDays: 15, RemoteCleanupSafetyDays: 2}
	return cfg, &rclone.Client{Bin: fakeRclone(t)}, calls, manifest
}

func TestPreviewCleanSafetyAndExactDryRun(t *testing.T) {
	cfg, rc, calls, manifest := previewFixture(t)
	p, err := PreviewClean(t.Context(), cfg, rc, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 2 || p.KnownBytes != 10 || p.UnknownSizes != 1 {
		t.Fatalf("preview = %+v", p)
	}
	b, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "old/a\nold/b\n" {
		t.Fatalf("manifest = %q", b)
	}
	commands, _ := os.ReadFile(calls)
	if !strings.Contains(string(commands), "--dry-run") {
		t.Fatalf("dry-run missing:\n%s", commands)
	}
	t.Setenv("RCSS_FAKE_LSF", "")
	if _, err := PreviewClean(t.Context(), cfg, rc, nil, false); !errors.Is(err, ErrNoRecentBackup) {
		t.Fatalf("error = %v", err)
	}
	if _, err := PreviewClean(t.Context(), cfg, rc, nil, true); err != nil {
		t.Fatalf("force preview: %v", err)
	}
}

func TestExecuteCleanRevalidatesAndDeletesSelectionOnly(t *testing.T) {
	cfg, rc, calls, manifest := previewFixture(t)
	p, err := PreviewClean(t.Context(), cfg, rc, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := ExecuteClean(t.Context(), cfg, rc, nil, p, []string{"old/b"}, false)
	if err != nil || r.Removed != 1 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	b, _ := os.ReadFile(manifest)
	if string(b) != "old/b\n" {
		t.Fatalf("real manifest=%q", b)
	}
	before, _ := os.ReadFile(calls)
	t.Setenv("RCSS_FAKE_LSJSON", `[{"Path":"old/a","Size":11,"ModTime":"2024-01-01T00:00:00Z"},{"Path":"old/b","Size":-1,"ModTime":"2024-01-02T00:00:00Z"}]`)
	if _, err := ExecuteClean(t.Context(), cfg, rc, nil, p, []string{"old/a"}, false); !errors.Is(err, ErrPreviewOutdated) {
		t.Fatalf("error=%v", err)
	}
	after, _ := os.ReadFile(calls)
	if strings.Count(string(after), "delete ") != strings.Count(string(before), "delete ") {
		t.Fatal("outdated preview invoked delete")
	}
}

func TestSameCandidatesChecksEveryField(t *testing.T) {
	n := int64(1)
	tm := time.Unix(10, 0)
	base := []CleanupCandidate{{Path: "a", Size: &n, ModTime: tm}}
	if !sameCandidates(base, []CleanupCandidate{{Path: "a", Size: &n, ModTime: tm}}) {
		t.Fatal("equal candidates differ")
	}
	n2 := int64(2)
	for _, changed := range [][]CleanupCandidate{
		{{Path: "b", Size: &n, ModTime: tm}},
		{{Path: "a", Size: &n2, ModTime: tm}},
		{{Path: "a", Size: &n, ModTime: tm.Add(time.Second)}},
	} {
		if sameCandidates(base, changed) {
			t.Fatalf("change not detected: %+v", changed)
		}
	}
}
