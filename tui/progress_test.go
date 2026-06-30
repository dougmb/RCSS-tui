package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/config"
)

func TestUploadRunningShowsDetailedLogByDefault(t *testing.T) {
	u := newUploadModel(config.Config{}, missingRclone())
	u.state = upRunning
	u.output = []string{"first", "latest progress"}

	if got := u.View(); !strings.Contains(got, "latest progress") {
		t.Fatalf("running upload should show detailed log, view: %q", got)
	}
}

func TestRestoreRunningShowsDetailedLogByDefault(t *testing.T) {
	b := newBackupsModel(config.Config{}, missingRclone())
	b.state = bsRestoring
	b.output = []string{"first", "latest progress"}

	if got := b.View(); !strings.Contains(got, "latest progress") {
		t.Fatalf("running restore should show detailed log, view: %q", got)
	}
}

func TestUploadLogOpenFailureFinishesStream(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{LogFile: filepath.Join(parent, "backup.log")}
	u := newUploadModel(cfg, missingRclone())

	u, cmd := u.start()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("start command should return a non-empty tea.BatchMsg")
	}
	u, _ = u.Update(batch[0]())
	if u.state != upError || u.err == nil {
		t.Fatalf("log open failure should finish in upError, state=%v err=%v", u.state, u.err)
	}
}

func TestStreamSinkHidesProviderJSON(t *testing.T) {
	s := newOpStream()
	sink := s.sink()
	sink("2026/06/30 ERROR : upload failed: googleapi: Error 403: {")
	sink(`  "error": {`)
	sink(`    "message": "Quota exceeded"`)
	sink("  }")
	sink("}")
	sink("final status")

	first := (<-s.ch).line
	second := (<-s.ch).line
	if !strings.Contains(first, "provider details hidden") {
		t.Fatalf("first line = %q", first)
	}
	if second != "final status" {
		t.Fatalf("JSON leaked into stream; second line = %q", second)
	}
}
