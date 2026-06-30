package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/scheduler"
)

// Schedule screen: a single interactive window that registers RCSS jobs with the
// host OS scheduler (crontab on Unix, Task Scheduler on Windows; no root/admin),
// calling the rcss binary headless — `rcss upload` and/or `rcss clean`. Each job
// is Daily or Weekly (on a chosen weekday) at a time. The editor opens
// pre-filled with whatever is currently scheduled, saves inline, and shows
// success/failure feedback without leaving the screen. Disabling both jobs and
// saving removes the managed jobs.

// fieldKind identifies an editable field within a job block (or the Save action).
type fieldKind int

const (
	fEnabled fieldKind = iota
	fCadence
	fWeekday
	fTime
	fSave
)

// focusTarget is one stop in the flat navigation order. job is the index into
// scheduleModel.jobs, or -1 for the Save action.
type focusTarget struct {
	job   int
	field fieldKind
}

// jobForm is the editable state of one schedulable job (Upload or Clean).
type jobForm struct {
	kind    scheduler.Kind
	enabled bool
	weekly  bool
	weekday time.Weekday
	hour    int
	min     int
}

type scheduleModel struct {
	cfg config.Config

	jobs    [2]jobForm // index 0 = Upload, 1 = Clean
	focus   int        // index into fields()
	editBuf string     // in-progress digit entry for the focused Time field

	current []scheduler.Job // what is actually scheduled (for the summary)

	// done flips to true once saved; saveErr is set by apply().
	done    bool
	saveErr error

	width, height int
}

func newScheduleModel(cfg config.Config) scheduleModel {
	current, _ := scheduler.Current(cfg.RemoteName)
	s := scheduleModel{
		cfg:     cfg,
		current: current,
		width:   80,
		height:  14,
		jobs: [2]jobForm{
			{kind: scheduler.Upload, weekly: false, weekday: time.Sunday, hour: 3, min: 0},
			{kind: scheduler.Clean, weekly: true, weekday: time.Sunday, hour: 5, min: 0},
		},
	}
	// Pre-fill the editor from the active schedule so it reflects reality.
	for _, j := range current {
		idx := 0
		if j.Kind == scheduler.Clean {
			idx = 1
		}
		jf := jobForm{kind: j.Kind, enabled: true, weekly: j.Weekly, weekday: j.Weekday, hour: j.Hour, min: j.Min}
		if j.Hour < 0 || j.Min < 0 { // a backend that couldn't recover the time
			jf.hour, jf.min = s.jobs[idx].hour, s.jobs[idx].min
		}
		s.jobs[idx] = jf
	}
	return s
}

func (s scheduleModel) Init() tea.Cmd { return nil }

func (s *scheduleModel) setSize(w, h int) { s.width, s.height = w, h }

// fields returns the ordered focus targets for the current state. The Weekday
// field only appears for weekly jobs, so navigation and rendering stay in sync.
func (s scheduleModel) fields() []focusTarget {
	var ft []focusTarget
	for i := range s.jobs {
		ft = append(ft, focusTarget{i, fEnabled}, focusTarget{i, fCadence})
		if s.jobs[i].weekly {
			ft = append(ft, focusTarget{i, fWeekday})
		}
		ft = append(ft, focusTarget{i, fTime})
	}
	return append(ft, focusTarget{-1, fSave})
}

func (s *scheduleModel) clampFocus() {
	if n := len(s.fields()); s.focus >= n {
		s.focus = n - 1
	}
	if s.focus < 0 {
		s.focus = 0
	}
}

func (s scheduleModel) Update(msg tea.Msg) (scheduleModel, tea.Cmd) {
	if s.done {
		switch msg := msg.(type) {
		case doneTimeoutMsg:
			return s, func() tea.Msg { return goBackMsg{} }
		case tea.KeyMsg:
			switch msg.String() {
			case "q":
				return s, tea.Quit
			case "enter", "esc", "backspace":
				return s, func() tea.Msg { return goBackMsg{} }
			}
		}
		return s, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	cur := s.fields()[s.focus]

	switch key.String() {
	case "q":
		return s, tea.Quit
	case "esc":
		return s, func() tea.Msg { return goBackMsg{} }
	case "backspace":
		// Delete a typed digit while editing a time; otherwise go back.
		if cur.field == fTime && s.editBuf != "" {
			s.editBuf = s.editBuf[:len(s.editBuf)-1]
			return s, nil
		}
		return s, func() tea.Msg { return goBackMsg{} }
	case "up", "k", "shift+tab":
		s.commitFocusedTime()
		s.focus--
		s.clampFocus()
		return s, nil
	case "down", "j", "tab":
		s.commitFocusedTime()
		s.focus++
		s.clampFocus()
		return s, nil
	case "left", "h":
		s.changeField(cur, -1)
		return s, nil
	case "right", "l":
		s.changeField(cur, +1)
		return s, nil
	case " ", "space":
		if cur.job >= 0 {
			s.jobs[cur.job].enabled = !s.jobs[cur.job].enabled
		}
		return s, nil
	case "enter":
		return s, s.save()
	}

	// Digit entry builds a 4-digit HHMM time on the focused Time field.
	if cur.field == fTime {
		if d := key.String(); len(d) == 1 && d[0] >= '0' && d[0] <= '9' {
			s.editBuf += d
			if len(s.editBuf) > 4 {
				s.editBuf = s.editBuf[len(s.editBuf)-4:]
			}
		}
	}
	return s, nil
}

// changeField applies a left/right (dir -1/+1) change to the focused field.
func (s *scheduleModel) changeField(t focusTarget, dir int) {
	if t.job < 0 {
		return // Save action: nothing to change
	}
	j := &s.jobs[t.job]
	switch t.field {
	case fEnabled:
		j.enabled = !j.enabled
	case fCadence:
		j.weekly = !j.weekly
		s.clampFocus() // the Weekday field appears/disappears
	case fWeekday:
		j.weekday = time.Weekday(((int(j.weekday)+dir)%7 + 7) % 7)
	case fTime:
		s.commitFocusedTime()
		j.addMinutes(dir * 5)
	}
}

// addMinutes nudges the job's time by d minutes, wrapping within a day.
func (j *jobForm) addMinutes(d int) {
	total := ((j.hour*60+j.min+d)%(24*60) + 24*60) % (24 * 60)
	j.hour, j.min = total/60, total%60
}

// commitFocusedTime applies any in-progress digit entry to the focused Time
// field, then clears the buffer. A no-op when not editing a time.
func (s *scheduleModel) commitFocusedTime() {
	if s.editBuf == "" {
		return
	}
	cur := s.fields()[s.focus]
	if cur.field == fTime && cur.job >= 0 {
		if h, m, ok := parseTimeBuf(s.editBuf); ok {
			s.jobs[cur.job].hour, s.jobs[cur.job].min = h, m
		}
	}
	s.editBuf = ""
}

// parseTimeBuf interprets 1–4 typed digits as a clock entry (right-aligned to
// HHMM), clamping to a valid time.
func parseTimeBuf(buf string) (hour, min int, ok bool) {
	if buf == "" {
		return 0, 0, false
	}
	for len(buf) < 4 {
		buf = "0" + buf
	}
	buf = buf[len(buf)-4:]
	h, _ := strconv.Atoi(buf[:2])
	m, _ := strconv.Atoi(buf[2:])
	if h > 23 {
		h = 23
	}
	if m > 59 {
		m = 59
	}
	return h, m, true
}

// buildJobs collects the enabled jobs as scheduler.Jobs for Apply.
func (s scheduleModel) buildJobs() []scheduler.Job {
	var jobs []scheduler.Job
	for _, jf := range s.jobs {
		if !jf.enabled {
			continue
		}
		jobs = append(jobs, scheduler.Job{
			Kind: jf.kind, Hour: jf.hour, Min: jf.min,
			Weekly: jf.weekly, Weekday: jf.weekday,
		})
	}
	return jobs
}

func (s scheduleModel) anyEnabled() bool {
	return s.jobs[0].enabled || s.jobs[1].enabled
}

// apply registers the enabled jobs with the OS scheduler.
func (s scheduleModel) apply() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating rcss binary: %w", err)
	}
	logPath, err := s.cfg.ResolveLogFile()
	if err != nil {
		return err
	}
	return scheduler.Apply(s.cfg.RemoteName, s.buildJobs(), exe, logPath)
}

// save commits any typed time, applies the schedule, and flips to the done
// state so the confirmation is shown before auto-returning to the menu.
func (s *scheduleModel) save() tea.Cmd {
	s.commitFocusedTime()
	s.saveErr = s.apply()
	if s.saveErr == nil {
		s.current, _ = scheduler.Current(s.cfg.RemoteName)
	}
	s.done = true
	return tea.Tick(saveConfirmationTimeout, func(time.Time) tea.Msg { return doneTimeoutMsg{} })
}

func (s scheduleModel) View() string {
	if s.done {
		return s.doneView()
	}

	cw := s.width - 1
	if cw < 10 {
		cw = 10
	}
	height := s.height - 2 // title + current-schedule summary
	if height < 1 {
		height = 1
	}

	lines, focusStart, focusEnd := s.contentLines()
	offset := 0
	if focusEnd > height {
		offset = focusEnd - height
	}
	if focusStart < offset {
		offset = focusStart
	}
	if maxOff := len(lines) - height; offset > maxOff {
		offset = maxOff
	}
	if offset < 0 {
		offset = 0
	}

	bar := scrollColumn(height, len(lines), offset)
	rows := make([]string, height)
	for i := 0; i < height; i++ {
		line := ""
		if offset+i < len(lines) {
			line = lines[offset+i]
		}
		rows[i] = padLineTo(line, cw) + bar[i]
	}

	title := clip(fmt.Sprintf("Schedule — %s (%s)", s.cfg.RemoteName, scheduler.Backend()), cw)
	summary := clip(s.currentScheduleLine(), cw)
	return titleStyle.Render(title) + "\n" + subtitleStyle.Render(summary) + "\n" + strings.Join(rows, "\n")
}

// contentLines renders the editor as a flat, scrollable list and identifies
// the focused row so it remains visible at every supported terminal height.
func (s scheduleModel) contentLines() (lines []string, focusStart, focusEnd int) {
	cur := s.fields()[s.focus]
	for i, jf := range s.jobs {
		title := jf.kind.Title()
		if !jf.enabled {
			title += " (off)"
		}
		lines = append(lines, titleStyle.Render(title))

		add := func(field fieldKind, line string) {
			focused := cur.job == i && cur.field == field
			if focused {
				focusStart = len(lines)
			}
			lines = append(lines, line)
			if focused {
				focusEnd = len(lines)
			}
		}

		check := "[ ]"
		if jf.enabled {
			check = "[x]"
		}
		add(fEnabled, "Enabled: "+fieldValue(check, cur.job == i && cur.field == fEnabled))

		cadence := "Daily"
		if jf.weekly {
			cadence = "Weekly"
		}
		add(fCadence, "Cadence: "+fieldValue(cadence, cur.job == i && cur.field == fCadence))
		if jf.weekly {
			add(fWeekday, "Day:     "+fieldValue(wdShort(jf.weekday), cur.job == i && cur.field == fWeekday))
		}

		timeStr := fmt.Sprintf("%02d:%02d", jf.hour, jf.min)
		if cur.job == i && cur.field == fTime && s.editBuf != "" {
			timeStr = s.editBuf + "_"
		}
		add(fTime, "Time:    "+fieldValue(timeStr, cur.job == i && cur.field == fTime))
		lines = append(lines, "")
	}

	saveFocused := cur.field == fSave
	if saveFocused {
		focusStart = len(lines)
	}
	save := subtitleStyle.Render("[ Save ]")
	if saveFocused {
		save = titleStyle.Render("‹ Save ›")
	}
	lines = append(lines, save)
	if saveFocused {
		focusEnd = len(lines)
	}
	return lines, focusStart, focusEnd
}

func (s scheduleModel) currentScheduleLine() string {
	if len(s.current) == 0 {
		return "Currently scheduled: none."
	}
	parts := make([]string, 0, len(s.current))
	for _, job := range s.current {
		parts = append(parts, fmt.Sprintf("%s %s at %s", job.Kind.Title(), job.Cadence(), job.Time()))
	}
	return "Currently scheduled: " + strings.Join(parts, "; ")
}

// doneView renders the save confirmation (success or error) before the screen
// auto-returns to the menu.
func (s scheduleModel) doneView() string {
	if s.saveErr != nil {
		return errorStyle.Render("✗ Could not update schedule") + "\n\n" +
			subtitleStyle.Render(s.saveErr.Error())
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("✓ Schedule saved"))
	b.WriteString("\n\n")
	if !s.anyEnabled() {
		b.WriteString(infoLine("Status", "RCSS jobs removed"))
	} else {
		b.WriteString(subtitleStyle.Render("Currently scheduled:"))
		for _, j := range s.current {
			fmt.Fprintf(&b, "\n  • %s — %s at %s", j.Kind.Title(), j.Cadence(), j.Time())
		}
	}
	return b.String()
}

// fieldValue renders a focusable value, bracketing and highlighting it when
// focused so the selection is obvious; padding keeps the width steady otherwise.
func fieldValue(v string, focused bool) string {
	if focused {
		return titleStyle.Render("‹ " + v + " ›")
	}
	return "  " + v + "  "
}

// wdShort is the three-letter label for a weekday (e.g. "Wed").
func wdShort(d time.Weekday) string { return d.String()[:3] }

func (s scheduleModel) footerHint() string {
	if s.done {
		return "enter/esc back • q quit"
	}
	return "↑/↓ field • ←/→ change • space toggle • enter save • esc back"
}

// currentScheduleText summarizes the active account's managed jobs.
func (s scheduleModel) currentScheduleText() string {
	if len(s.current) == 0 {
		return fmt.Sprintf("Currently scheduled: none for %s (%s).", s.cfg.RemoteName, scheduler.Backend())
	}
	var b strings.Builder
	b.WriteString("Currently scheduled:")
	for _, j := range s.current {
		fmt.Fprintf(&b, "\n  • %s — %s at %s", j.Kind.Title(), j.Cadence(), j.Time())
	}
	return b.String()
}
