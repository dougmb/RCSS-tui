//go:build windows

package scheduler

import (
	"testing"
	"time"
)

func TestTaskNameIsScopedAndSanitized(t *testing.T) {
	if got, want := taskName("my drive:", Upload), "RCSS-my_drive-Upload"; got != want {
		t.Fatalf("taskName = %q, want %q", got, want)
	}
	if got, want := taskName("my drive:", Clean), "RCSS-my_drive-Clean"; got != want {
		t.Fatalf("taskName = %q, want %q", got, want)
	}
}

func TestParseTaskXMLDaily(t *testing.T) {
	xml := `<Task><Triggers><CalendarTrigger>
<StartBoundary>2026-06-30T03:45:00</StartBoundary>
<ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay>
</CalendarTrigger></Triggers></Task>`
	got := parseTaskXML(xml, Upload)
	if got.Kind != Upload || got.Hour != 3 || got.Min != 45 || got.Weekly {
		t.Fatalf("parseTaskXML = %+v", got)
	}
}

func TestParseTaskXMLWeeklyUTF16Like(t *testing.T) {
	xml := "<\x00T\x00a\x00s\x00k\x00>\x00" +
		"<\x00S\x00t\x00a\x00r\x00t\x00B\x00o\x00u\x00n\x00d\x00a\x00r\x00y\x00>\x00" +
		"2\x000\x002\x006\x00-\x000\x006\x00-\x003\x000\x00T\x002\x003\x00:\x000\x005\x00:\x000\x000\x00" +
		"<\x00/\x00S\x00t\x00a\x00r\x00t\x00B\x00o\x00u\x00n\x00d\x00a\x00r\x00y\x00>\x00" +
		"<\x00S\x00c\x00h\x00e\x00d\x00u\x00l\x00e\x00B\x00y\x00W\x00e\x00e\x00k\x00>\x00" +
		"<\x00D\x00a\x00y\x00s\x00O\x00f\x00W\x00e\x00e\x00k\x00>\x00<\x00T\x00u\x00e\x00s\x00d\x00a\x00y\x00/\x00>\x00"
	got := parseTaskXML(xml, Clean)
	if got.Kind != Clean || got.Hour != 23 || got.Min != 5 || !got.Weekly || got.Weekday != time.Tuesday {
		t.Fatalf("parseTaskXML = %+v", got)
	}
}

func TestParseTaskXMLMissingBoundary(t *testing.T) {
	got := parseTaskXML(`<Task><ScheduleByWeek/></Task>`, Upload)
	if got.Hour != -1 || got.Min != -1 || !got.Weekly {
		t.Fatalf("parseTaskXML = %+v", got)
	}
}
