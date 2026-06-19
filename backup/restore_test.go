package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// TestRestoreTarget covers the default local restore destination.
// A configured RestoreDestination is the base; empty falls back to cwd.
// The returned path is the parent directory into which the item will be copied,
// preserving the remote's relative structure.
func TestRestoreTarget(t *testing.T) {
	sep := string(os.PathSeparator)

	cfg := config.Config{
		SourceFolders:      []string{"/home/me/work/website", "/home/me/notes"},
		RestoreDestination: "/restore",
	}

	// Root-level folder: restore into the configured base.
	if got, err := RestoreTarget(cfg, "website"); err != nil || got != "/restore" {
		t.Errorf("root folder: got %q err=%v, want %q", got, err, "/restore")
	}
	// Nested file: preserve intermediate directories.
	if got, err := RestoreTarget(cfg, "website/index.html"); err != nil || got != "/restore"+sep+"website" {
		t.Errorf("nested file: got %q err=%v, want %q", got, err, "/restore"+sep+"website")
	}
	// Loose file at the destination root lands directly in the base.
	if got, err := RestoreTarget(cfg, "file.txt"); err != nil || got != "/restore" {
		t.Errorf("loose file: got %q err=%v, want %q", got, err, "/restore")
	}
	// Empty relPath restores the whole destination into the base.
	if got, err := RestoreTarget(cfg, ""); err != nil || got != "/restore" {
		t.Errorf("empty relPath: got %q err=%v, want %q", got, err, "/restore")
	}

	// With no RestoreDestination, the base is the current directory.
	src := config.Config{SourceFolders: []string{"/home/me/work/website"}}
	cwd, _ := os.Getwd()
	if got, err := RestoreTarget(src, "website"); err != nil || got != cwd {
		t.Errorf("no dest, root folder: got %q err=%v, want %q", got, err, cwd)
	}
	if got, err := RestoreTarget(src, "website/css/main.css"); err != nil || got != cwd+sep+"website"+sep+"css" {
		t.Errorf("no dest, nested file: got %q err=%v, want %q", got, err, cwd+sep+"website"+sep+"css")
	}
	if got, err := RestoreTarget(src, ""); err != nil || got != cwd {
		t.Errorf("no dest, empty relPath: got %q err=%v, want %q", got, err, cwd)
	}
}

// TestRestoreBuildsDestination verifies that Restore creates the correct local
// destination for files and directories. It uses a fake rclone binary that
// records the received src/dst arguments and succeeds immediately.
func TestRestoreBuildsDestination(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "rclone")
	if err := os.WriteFile(fake, []byte(`#!/bin/sh
# Fake rclone: record arguments and succeed.
echo "$*" > "$RCSS_FAKE_OUTPUT"
`), 0o755); err != nil {
		t.Fatal(err)
	}

	runRestore := func(isDir bool, relPath, restoreDir string) (src, dst string) {
		t.Helper()
		outFile := filepath.Join(t.TempDir(), "out.txt")
		t.Setenv("RCSS_FAKE_OUTPUT", outFile)

		cfg := config.Config{RemoteName: "drive:", RestoreDestination: restoreDir}
		// Use the real Client but point it at our fake binary.
		rc := &rclone.Client{Bin: fake}
		log, _ := NewLogger("", func(string) {}, false)
		defer log.Close()

		if err := Restore(t.Context(), cfg, rc, log, relPath, RestoreOptions{IsDir: isDir}); err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		out, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("reading fake output: %v", err)
		}
		// Output format: "copy <src> <dst> --ignore-times ..."
		parts := strings.Fields(string(out))
		if len(parts) < 3 || parts[0] != "copy" {
			t.Fatalf("unexpected fake output: %q", string(out))
		}
		return parts[1], parts[2]
	}

	restoreDir := t.TempDir()
	src, dst := runRestore(false, "website/index.html", restoreDir)
	if src != "drive:/website/index.html" {
		t.Errorf("file src = %q, want drive:/website/index.html", src)
	}
	want := restoreDir + string(os.PathSeparator) + "website" + string(os.PathSeparator)
	if dst != want {
		t.Errorf("file dst = %q, want %q", dst, want)
	}

	src, dst = runRestore(true, "website", restoreDir)
	if src != "drive:/website" {
		t.Errorf("dir src = %q, want drive:/website", src)
	}
	want = restoreDir + string(os.PathSeparator) + "website"
	if dst != want {
		t.Errorf("dir dst = %q, want %q", dst, want)
	}

	src, dst = runRestore(true, "", restoreDir)
	if src != "drive:" {
		t.Errorf("root src = %q, want drive:", src)
	}
	if dst != restoreDir {
		t.Errorf("root dst = %q, want %q", dst, restoreDir)
	}
}
