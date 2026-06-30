package rclone

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "rclone.cmd")
		body := "@echo off\r\n" +
			"echo %*>>\"%RCSS_RCLONE_CALLS%\"\r\n" +
			"if \"%1\"==\"lsf\" echo folder/&echo file.txt\r\n" +
			"if \"%RCSS_RCLONE_FAIL%\"==\"1\" exit /b 3\r\n" +
			"exit /b 0\r\n"
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "rclone")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$RCSS_RCLONE_CALLS"
if [ "$1" = "lsf" ]; then printf 'folder/\nfile.txt\n'; fi
if [ "$RCSS_RCLONE_FAIL" = "1" ]; then exit 3; fi
`
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLsfBuildsArgumentsAndParsesLines(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_RCLONE_CALLS", calls)
	c := &Client{Bin: testBinary(t)}

	got, err := c.Lsf(t.Context(), "drive:/backups", LsfOptions{
		Mode: LsfFilesOnly, Recursive: true, MaxAge: "2d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "folder/" || got[1] != "file.txt" {
		t.Fatalf("Lsf = %#v", got)
	}
	assertCallsContain(t, calls, "lsf drive:/backups --files-only --recursive --max-age 2d")
}

func TestCopyAndDeleteBuildArguments(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_RCLONE_CALLS", calls)
	c := &Client{Bin: testBinary(t)}

	var streamed []string
	err := c.Copy(t.Context(), "source", "drive:/dest", CopyOptions{
		Update: true, UseMmap: true, Retries: 3, Progress: true,
		StatsOneLine: true, Stats: "10s", Excludes: []string{"*.tmp"},
	}, func(line string) { streamed = append(streamed, line) })
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(t.Context(), "drive:/dest", DeleteOptions{
		MinAge: "15d", DryRun: true, LogLevel: "INFO",
	}, nil); err != nil {
		t.Fatal(err)
	}

	assertCallsContain(t, calls,
		"copy source drive:/dest --stats-one-line --stats 10s --update --use-mmap --retries 3 -P --exclude *.tmp")
	assertCallsContain(t, calls,
		"delete drive:/dest --min-age 15d --dry-run --log-level INFO")
}

func TestCommandFailureIsReturned(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls.txt")
	t.Setenv("RCSS_RCLONE_CALLS", calls)
	t.Setenv("RCSS_RCLONE_FAIL", "1")
	c := &Client{Bin: testBinary(t)}

	if err := c.Delete(t.Context(), "drive:", DeleteOptions{}, nil); err == nil ||
		!strings.Contains(err.Error(), "rclone delete") {
		t.Fatalf("Delete error = %v", err)
	}
}

func TestScanLinesOrCR(t *testing.T) {
	s := bufioScanner("one\rtwo\nthree")
	var got []string
	for s.Scan() {
		got = append(got, s.Text())
	}
	if strings.Join(got, ",") != "one,two,three" {
		t.Fatalf("tokens = %v", got)
	}
}

func bufioScanner(input string) *bufio.Scanner {
	s := bufio.NewScanner(strings.NewReader(input))
	s.Split(scanLinesOrCR)
	return s
}

func assertCallsContain(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), want) {
		t.Fatalf("calls missing %q:\n%s", want, b)
	}
}
