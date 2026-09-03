//go:build windows

package scheduler

import (
	"strings"
	"testing"
	"time"
)

// TestTaskNameUniquePerFolder checks that each job maps to its own task name:
// the all-folders upload and the clean job keep their plain names, while two
// source folders that share a base name still get distinct tasks (the short
// hash of the full path is what separates them).
func TestTaskNameUniquePerFolder(t *testing.T) {
	all := taskName("drive:", Job{Kind: Upload})
	clean := taskName("drive:", Job{Kind: Clean})
	a := taskName("drive:", Job{Kind: Upload, Folder: `C:\projects\alpha`})
	b := taskName("drive:", Job{Kind: Upload, Folder: `D:\archive\alpha`})

	if all != "RCSS-drive-Upload" {
		t.Errorf("all-folders task = %q", all)
	}
	if clean != "RCSS-drive-Clean" {
		t.Errorf("clean task = %q", clean)
	}
	if a == b {
		t.Errorf("folders sharing a base name collided on %q", a)
	}
	for _, n := range []string{a, b} {
		if !strings.HasPrefix(n, taskPrefix("drive:")) {
			t.Errorf("task %q must carry the account prefix", n)
		}
		if !strings.Contains(n, "alpha") {
			t.Errorf("task %q should stay readable (base name)", n)
		}
	}
	// A different account must never share a prefix with drive:.
	if strings.HasPrefix(taskName("work:", Job{Kind: Upload}), taskPrefix("drive:")) {
		t.Error("accounts must not share a task prefix")
	}
}

// taskXML builds a minimal schtasks /XML document, escaping the arguments the
// way Task Scheduler does, so the parser is exercised on realistic input.
func taskXML(weekly bool, args string) string {
	schedule := "<ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay>"
	if weekly {
		schedule = "<ScheduleByWeek><DaysOfWeek><Wednesday/></DaysOfWeek></ScheduleByWeek>"
	}
	return `<?xml version="1.0" encoding="UTF-16"?><Task><Triggers><CalendarTrigger>` +
		`<StartBoundary>2024-01-01T04:45:00</StartBoundary>` + schedule +
		`</CalendarTrigger></Triggers><Actions><Exec><Command>C:\rcss.exe</Command><Arguments>` +
		strings.ReplaceAll(args, `"`, "&quot;") + `</Arguments></Exec></Actions></Task>`
}

// TestParseTaskXML checks that kind, target folder, cadence and time are all
// recovered from a task's XML — including a folder path containing a space,
// which only survives because the arguments are unescaped before tokenizing.
func TestParseTaskXML(t *testing.T) {
	t.Run("per-folder weekly upload", func(t *testing.T) {
		j := parseTaskXML(taskXML(true,
			`upload --account "drive:" --folder "C:\My Projects\alpha"`))
		if j.Kind != Upload {
			t.Errorf("kind = %v, want Upload", j.Kind)
		}
		if j.Folder != `C:\My Projects\alpha` {
			t.Errorf("folder = %q", j.Folder)
		}
		if !j.Weekly || j.Weekday != time.Wednesday {
			t.Errorf("cadence = %s", j.Cadence())
		}
		if j.Time() != "04:45" {
			t.Errorf("time = %q, want 04:45", j.Time())
		}
	})

	t.Run("all-folders upload has no folder", func(t *testing.T) {
		j := parseTaskXML(taskXML(false, `upload --account "drive:"`))
		if j.Kind != Upload || j.Folder != "" {
			t.Errorf("got %+v, want an all-folders upload", j)
		}
		if j.Label() != "All folders" {
			t.Errorf("label = %q", j.Label())
		}
	})

	t.Run("clean ignores folder", func(t *testing.T) {
		j := parseTaskXML(taskXML(false, `clean --account "drive:"`))
		if j.Kind != Clean {
			t.Errorf("kind = %v, want Clean", j.Kind)
		}
	})
}
