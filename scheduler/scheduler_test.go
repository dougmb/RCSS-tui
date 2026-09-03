package scheduler

import (
	"testing"
	"time"
)

// TestJobCadenceAndTime checks the human-readable recurrence and time labels,
// including the weekday for weekly jobs and the unknown-time fallback.
func TestJobCadenceAndTime(t *testing.T) {
	daily := Job{Kind: Upload, Hour: 3, Min: 5}
	if got := daily.Cadence(); got != "daily" {
		t.Errorf("daily cadence = %q", got)
	}
	if got := daily.Time(); got != "03:05" {
		t.Errorf("time = %q", got)
	}

	weekly := Job{Kind: Clean, Hour: 7, Min: 30, Weekly: true, Weekday: time.Wednesday}
	if got := weekly.Cadence(); got != "weekly (Wed)" {
		t.Errorf("weekly cadence = %q", got)
	}

	if got := (Job{Hour: -1, Min: -1}).Time(); got != "??:??" {
		t.Errorf("unknown time = %q", got)
	}
}

// TestWeekdayShort checks the three-letter label for every weekday, including
// out-of-range normalization (cron's 7 → Sunday).
func TestWeekdayShort(t *testing.T) {
	want := map[time.Weekday]string{
		time.Sunday: "Sun", time.Monday: "Mon", time.Tuesday: "Tue",
		time.Wednesday: "Wed", time.Thursday: "Thu", time.Friday: "Fri",
		time.Saturday: "Sat",
	}
	for d, w := range want {
		if got := weekdayShort(d); got != w {
			t.Errorf("weekdayShort(%v) = %q, want %q", d, got, w)
		}
	}
	if got := weekdayShort(time.Weekday(7)); got != "Sun" {
		t.Errorf("weekdayShort(7) = %q, want Sun", got)
	}
}

// TestSplitArgs checks the quote-aware tokenizer that lets folders, binaries and
// log paths contain spaces without corrupting the parse on the way back.
func TestSplitArgs(t *testing.T) {
	got := splitArgs(`0 3 * * * "/opt/my apps/rcss" upload --account "drive:" --folder "/home/u/My Docs" >> "/tmp/a b.log" 2>&1`)
	want := []string{
		"0", "3", "*", "*", "*", "/opt/my apps/rcss", "upload",
		"--account", "drive:", "--folder", "/home/u/My Docs", ">>", "/tmp/a b.log", "2>&1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSplitArgsBackslashes pins the escape rule: only \" and \\ are escapes
// (what Go's %q emits into a crontab line), so a Windows path keeps its literal
// backslashes when it comes back out of a scheduled task's arguments.
func TestSplitArgsBackslashes(t *testing.T) {
	cases := map[string][]string{
		`upload --folder "C:\My Projects\alpha"`: {"upload", "--folder", `C:\My Projects\alpha`},
		`upload --folder "/home/u/a\\b"`:         {"upload", "--folder", `/home/u/a\b`},
		`upload --folder "/home/u/say \"hi\""`:   {"upload", "--folder", `/home/u/say "hi"`},
	}
	for line, want := range cases {
		got := splitArgs(line)
		if len(got) != len(want) {
			t.Errorf("splitArgs(%s) = %q, want %q", line, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitArgs(%s)[%d] = %q, want %q", line, i, got[i], want[i])
			}
		}
	}
}

// TestJobLabelAndSameTarget checks how a job names its target and how backends
// and the UI decide two jobs address the same scheduler entry.
func TestJobLabelAndSameTarget(t *testing.T) {
	all := Job{Kind: Upload}
	alpha := Job{Kind: Upload, Folder: "/srv/alpha"}
	beta := Job{Kind: Upload, Folder: "/srv/beta"}
	clean := Job{Kind: Clean}

	if got := all.Label(); got != "All folders" {
		t.Errorf("all.Label() = %q, want All folders", got)
	}
	if got := alpha.Label(); got != "alpha" {
		t.Errorf("alpha.Label() = %q, want alpha", got)
	}
	if alpha.SameTarget(beta) {
		t.Error("different folders must not be the same target")
	}
	if alpha.SameTarget(all) {
		t.Error("a per-folder upload is not the all-folders upload")
	}
	if !alpha.SameTarget(Job{Kind: Upload, Folder: "/srv/alpha", Hour: 9}) {
		t.Error("same kind+folder must be the same target regardless of time")
	}
	// Clean is per-account, so any two Clean jobs address the same entry.
	if !clean.SameTarget(Job{Kind: Clean, Weekly: true}) {
		t.Error("Clean jobs must always share a target")
	}
}
