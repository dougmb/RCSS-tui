package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeRclone creates a small platform-native executable used by integration
// tests. It records every invocation, can return canned lsf output, and can be
// instructed to fail copy calls.
func fakeRclone(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "rclone.cmd")
		script := "@echo off\r\n" +
			"echo %*>>\"%RCSS_FAKE_OUTPUT%\"\r\n" +
			"if \"%1\"==\"lsf\" if defined RCSS_FAKE_LSF echo %RCSS_FAKE_LSF%\r\n" +
			"if \"%1\"==\"lsjson\" echo %RCSS_FAKE_LSJSON%\r\n" +
			"if \"%1\"==\"delete\" if \"%3\"==\"--files-from-raw\" type \"%4\">>\"%RCSS_FAKE_MANIFEST%\"\r\n" +
			"if \"%1\"==\"copy\" if \"%RCSS_FAKE_FAIL_COPY%\"==\"1\" exit /b 1\r\n" +
			"exit /b 0\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	path := filepath.Join(dir, "rclone")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$RCSS_FAKE_OUTPUT"
if [ "$1" = "lsf" ] && [ -n "$RCSS_FAKE_LSF" ]; then
	printf '%s\n' "$RCSS_FAKE_LSF"
fi
if [ "$1" = "lsjson" ]; then
	printf '%s\n' "$RCSS_FAKE_LSJSON"
fi
if [ "$1" = "delete" ] && [ "$3" = "--files-from-raw" ]; then
	cat "$4" >> "$RCSS_FAKE_MANIFEST"
fi
if [ "$1" = "copy" ] && [ "$RCSS_FAKE_FAIL_COPY" = "1" ]; then
	exit 1
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
