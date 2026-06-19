package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/scheduler"
)

func keyType(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func sendSched(s scheduleModel, k tea.KeyMsg) scheduleModel {
	s, _ = s.Update(k)
	return s
}

// TestScheduleEditorBuildsJob drives the editor headless: enable Clean, set its
// weekday and time, and check the job it would register. It never calls save()/
// apply(), so the real crontab is untouched. The account name is unlikely to
// have existing jobs, so pre-population stays at defaults regardless of host.
func TestScheduleEditorBuildsJob(t *testing.T) {
	s := newScheduleModel(config.Config{RemoteName: "test-rcss-zzz:"})

	// Fields: 0 U.enabled 1 U.cadence 2 U.time | 3 C.enabled 4 C.cadence
	//         5 C.weekday 6 C.time | 7 Save  (Clean defaults to weekly).
	down := keyType(tea.KeyDown)
	right := keyType(tea.KeyRight)
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}

	s = sendSched(s, down)  // 1
	s = sendSched(s, down)  // 2
	s = sendSched(s, down)  // 3 → Clean Enabled
	s = sendSched(s, space) // enable Clean
	s = sendSched(s, down)  // 4 cadence
	s = sendSched(s, down)  // 5 weekday
	s = sendSched(s, right) // Sunday → Monday
	s = sendSched(s, down)  // 6 time
	for _, d := range "0730" {
		s = sendSched(s, keyRune(d))
	}
	s = sendSched(s, down) // 7 Save — commits the typed time

	jobs := s.buildJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected only Clean enabled, got %d jobs: %+v", len(jobs), jobs)
	}
	want := scheduler.Job{Kind: scheduler.Clean, Hour: 7, Min: 30, Weekly: true, Weekday: time.Monday}
	if jobs[0] != want {
		t.Fatalf("built job = %+v, want %+v", jobs[0], want)
	}
}

// TestScheduleCadenceToggleShowsWeekday checks toggling a job to weekly adds the
// weekday field to the focus order, and back to daily removes it.
func TestScheduleCadenceToggleShowsWeekday(t *testing.T) {
	s := newScheduleModel(config.Config{RemoteName: "test-rcss-zzz:"})

	// Upload starts daily: Enabled, Cadence, Time (no weekday).
	if n := countWeekdayFields(s); n != 1 {
		// Clean defaults to weekly, so exactly one weekday field exists.
		t.Fatalf("expected 1 weekday field initially, got %d", n)
	}
	// Move to Upload's Cadence (index 1) and switch it to weekly.
	s = sendSched(s, keyType(tea.KeyDown))
	s = sendSched(s, keyType(tea.KeyRight))
	if n := countWeekdayFields(s); n != 2 {
		t.Fatalf("expected 2 weekday fields after making Upload weekly, got %d", n)
	}

	if !strings.Contains(s.View(), "Schedule —") {
		t.Error("View should render the Schedule header")
	}
}

func countWeekdayFields(s scheduleModel) int {
	n := 0
	for _, f := range s.fields() {
		if f.field == fWeekday {
			n++
		}
	}
	return n
}
