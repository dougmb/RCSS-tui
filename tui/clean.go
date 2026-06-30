package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

type cleanState int

const (
	clIntro cleanState = iota
	clDryRunning
	clReport
	clConfirmSummary
	clConfirmPhrase
	clRunning
	clDone
	clOutdated
	clError
)

type cleanPreviewMsg struct {
	preview backup.CleanPreview
	err     error
}
type cleanResultMsg struct {
	result backup.CleanResult
	err    error
}
type cleanActivityTickMsg time.Time

type cleanModel struct {
	cfg           config.Config
	rc            *rclone.Client
	state         cleanState
	force         bool
	spinner       spinner.Model
	preview       backup.CleanPreview
	selected      []bool
	cursor        int
	offset        int
	input         textinput.Model
	result        backup.CleanResult
	err           error
	width, height int
	startedAt     time.Time
	activityTick  int
}

func newCleanModel(cfg config.Config, rc *rclone.Client) cleanModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512
	return cleanModel{cfg: cfg, rc: rc, spinner: sp, input: ti, state: clIntro, width: 80, height: 14}
}

func (c *cleanModel) setSize(w, h int)  { c.width, c.height = w, h }
func (c cleanModel) remoteDest() string { return c.cfg.RemoteBase() }

func (c cleanModel) localRule() string {
	if !c.cfg.DeleteAfterUpload {
		return "local files are kept (delete-after-upload is off)"
	}
	if c.cfg.RetentionDays == 0 {
		return "all local files removed after a successful upload"
	}
	return fmt.Sprintf("local files older than %d day(s) removed after upload", c.cfg.RetentionDays)
}

func (c cleanModel) previewCmd() tea.Cmd {
	cfg, rc, force := c.cfg, c.rc, c.force
	return func() tea.Msg {
		log, _ := backup.NewLogger("", nil, false)
		p, err := backup.PreviewClean(context.Background(), cfg, rc, log, force)
		if log != nil {
			log.Close()
		}
		return cleanPreviewMsg{p, err}
	}
}

func (c cleanModel) executeCmd(paths []string) tea.Cmd {
	cfg, rc, force, preview := c.cfg, c.rc, c.force, c.preview
	return func() tea.Msg {
		log, _ := backup.NewLogger("", nil, false)
		result, err := backup.ExecuteClean(context.Background(), cfg, rc, log, preview, paths, force)
		if log != nil {
			log.Close()
		}
		return cleanResultMsg{result, err}
	}
}

func (c cleanModel) startPreview() (cleanModel, tea.Cmd) {
	c.state, c.err = clDryRunning, nil
	c.startedAt, c.activityTick = time.Now(), 0
	return c, tea.Batch(c.previewCmd(), c.spinner.Tick, cleanActivityTick())
}

func cleanActivityTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return cleanActivityTickMsg(t)
	})
}

func (c cleanModel) Update(msg tea.Msg) (cleanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cleanPreviewMsg:
		if msg.err != nil {
			c.state, c.err = clError, msg.err
			return c, nil
		}
		c.preview = msg.preview
		c.selected = make([]bool, len(msg.preview.Candidates))
		for i := range c.selected {
			c.selected[i] = true
		}
		c.cursor, c.offset, c.state = 0, 0, clReport
		return c, nil
	case cleanResultMsg:
		c.result = msg.result
		if errors.Is(msg.err, backup.ErrPreviewOutdated) {
			c.state, c.err = clOutdated, msg.err
		} else if msg.err != nil {
			c.state, c.err = clError, msg.err
		} else {
			c.state = clDone
		}
		return c, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		return c, cmd
	case cleanActivityTickMsg:
		if c.state != clDryRunning && c.state != clRunning {
			return c, nil
		}
		c.activityTick++
		return c, cleanActivityTick()
	case tea.KeyMsg:
		return c.handleKey(msg)
	}
	if c.state == clConfirmPhrase {
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return c, cmd
	}
	return c, nil
}

func (c cleanModel) handleKey(msg tea.KeyMsg) (cleanModel, tea.Cmd) {
	key := msg.String()
	if key == "q" && c.state != clConfirmPhrase {
		return c, tea.Quit
	}
	if c.state == clConfirmPhrase {
		switch key {
		case "esc":
			c.state = clReport
			c.input.Blur()
			c.input.SetValue("")
			return c, nil
		case "enter":
			if strings.TrimSpace(c.input.Value()) != "DELETE "+c.remoteDest() {
				c.err = errors.New("confirmation phrase did not match")
				return c, nil
			}
			paths := c.selectedPaths()
			c.state = clRunning
			c.err = nil
			c.startedAt, c.activityTick = time.Now(), 0
			c.input.Blur()
			return c, tea.Batch(c.executeCmd(paths), c.spinner.Tick, cleanActivityTick())
		default:
			var cmd tea.Cmd
			c.input, cmd = c.input.Update(msg)
			return c, cmd
		}
	}
	if key == "esc" || key == "backspace" {
		switch c.state {
		case clDryRunning, clRunning:
			return c, nil
		case clConfirmSummary:
			c.state = clReport
			return c, nil
		default:
			return c, func() tea.Msg { return goBackMsg{} }
		}
	}
	switch key {
	case "f":
		if c.state == clIntro || c.state == clError {
			c.force = !c.force
		}
	case "enter":
		switch c.state {
		case clIntro:
			return c.startPreview()
		case clConfirmSummary:
			c.state = clConfirmPhrase
			c.input.SetValue("")
			c.err = nil
			return c, c.input.Focus()
		case clDone, clError:
			return c, func() tea.Msg { return goBackMsg{} }
		}
	case "r":
		if c.state == clReport || c.state == clOutdated || c.state == clError {
			return c.startPreview()
		}
	case "up", "k":
		if c.state == clReport && c.cursor > 0 {
			c.cursor--
			c.adjustOffset()
		}
	case "down", "j":
		if c.state == clReport && c.cursor+1 < len(c.selected) {
			c.cursor++
			c.adjustOffset()
		}
	case " ":
		if c.state == clReport && len(c.selected) > 0 {
			c.selected[c.cursor] = !c.selected[c.cursor]
		}
	case "a":
		if c.state == clReport {
			all := c.selectedCount() == len(c.selected)
			for i := range c.selected {
				c.selected[i] = !all
			}
		}
	case "x":
		if c.state == clReport && c.selectedCount() > 0 {
			c.state = clConfirmSummary
		}
	case "n":
		if c.state == clConfirmSummary {
			c.state = clReport
		}
	}
	return c, nil
}

func (c *cleanModel) adjustOffset() {
	rows := c.listRows()
	if c.cursor < c.offset {
		c.offset = c.cursor
	}
	if c.cursor >= c.offset+rows {
		c.offset = c.cursor - rows + 1
	}
}
func (c cleanModel) listRows() int {
	rows := c.height - 4 // title, two summary rows, and a blank line
	if rows < 1 {
		return 1
	}
	return rows
}
func (c cleanModel) selectedCount() int {
	n := 0
	for _, v := range c.selected {
		if v {
			n++
		}
	}
	return n
}
func (c cleanModel) selectedPaths() []string {
	out := make([]string, 0, c.selectedCount())
	for i, v := range c.selected {
		if v {
			out = append(out, c.preview.Candidates[i].Path)
		}
	}
	return out
}
func (c cleanModel) selectedTotals() (int64, int) {
	var bytes int64
	unknown := 0
	for i, v := range c.selected {
		if !v {
			continue
		}
		if c.preview.Candidates[i].Size == nil {
			unknown++
		} else {
			bytes += *c.preview.Candidates[i].Size
		}
	}
	return bytes, unknown
}

func formatSize(size *int64) string {
	if size == nil {
		return "unknown"
	}
	return humanize.IBytes(uint64(*size))
}
func (c cleanModel) forceLabel() string {
	if c.force {
		return warnStyle.Render("ON  ⚠ bypasses the safety lock")
	}
	return "OFF"
}

func (c cleanModel) View() string {
	switch c.state {
	case clIntro:
		return c.introView()
	case clDryRunning:
		return c.activityView("Validating safety lock and previewing exact candidates…")
	case clReport:
		return c.reportView()
	case clConfirmSummary:
		return c.summaryView()
	case clConfirmPhrase:
		return c.phraseView()
	case clRunning:
		return c.activityView("Revalidating remote and deleting selected files…")
	case clDone:
		return fmt.Sprintf("%s\n\nSelected: %d  Removed: %d  Failures: %d", titleStyle.Render("✓ Cleanup complete."), c.result.Selected, c.result.Removed, c.result.Failures)
	case clOutdated:
		return errorStyle.Render("Preview outdated — nothing was deleted.") + "\n\nThe remote path, size, or modification time changed. Run a new dry-run."
	case clError:
		return errorStyle.Render("✗ Clean aborted: " + c.err.Error())
	}
	return ""
}

func (c cleanModel) activityView(label string) string {
	const segment = 5
	width := c.width - 2
	if width > 48 {
		width = 48
	}
	if width < segment+2 {
		width = segment + 2
	}
	track := width - 2
	travel := track - segment
	pos := 0
	if travel > 0 {
		pos = c.activityTick % (2 * travel)
		if pos > travel {
			pos = 2*travel - pos
		}
	}
	bar := "[" + strings.Repeat(" ", pos) + strings.Repeat("=", segment) +
		strings.Repeat(" ", track-pos-segment) + "]"
	elapsed := time.Since(c.startedAt).Truncate(time.Second)
	if c.startedAt.IsZero() || elapsed < 0 {
		elapsed = 0
	}
	return fmt.Sprintf("%s %s\n\n%s\nElapsed: %s — waiting for rclone", c.spinner.View(), label, bar, elapsed)
}

func (c cleanModel) introView() string {
	if c.height > 0 && c.height < 10 {
		w := c.width
		if w < 10 {
			w = 10
		}
		lines := []string{
			titleStyle.Render(clip("Clean — remove old CLOUD backups", w)),
			clip(fmt.Sprintf("Cloud: %s", c.remoteDest()), w),
			clip(fmt.Sprintf("Delete files older than %d day(s).", c.cfg.RemoteRetentionDays), w),
			clip(fmt.Sprintf("Safety: recent backup within %d day(s).", c.cfg.RemoteCleanupSafetyDays), w),
			clip("Local: "+c.localRule(), w),
			subtitleStyle.Render("Force: ") + c.forceLabel(),
		}
		if len(lines) > c.height {
			lines = lines[:c.height]
		}
		return strings.Join(lines, "\n")
	}
	return titleStyle.Render("Clean — remove old CLOUD backups") + fmt.Sprintf("\n\nDeletes files at %s older than %d day(s).\nRequires a recent backup within %d day(s).\n\nLocal files are NOT touched here:\n  • %s\n\n%s%s", c.remoteDest(), c.cfg.RemoteRetentionDays, c.cfg.RemoteCleanupSafetyDays, c.localRule(), subtitleStyle.Render("Force: "), c.forceLabel())
}

func (c cleanModel) reportView() string {
	selectedBytes, selectedUnknown := c.selectedTotals()
	header := []string{
		titleStyle.Render(clip("Dry-run complete — nothing deleted.", c.width)),
		clip(fmt.Sprintf("Candidates: %d  Known size: %s  Unknown sizes: %d", len(c.preview.Candidates), humanize.IBytes(uint64(c.preview.KnownBytes)), c.preview.UnknownSizes), c.width),
		clip(fmt.Sprintf("Selected: %d  Known size: %s  Unknown sizes: %d", c.selectedCount(), humanize.IBytes(uint64(selectedBytes)), selectedUnknown), c.width),
		"",
	}

	height := c.listRows()
	cw := c.width - 1
	if cw < 10 {
		cw = 10
	}
	lines := make([]string, len(c.preview.Candidates))
	for i, candidate := range c.preview.Candidates {
		mark := "[ ]"
		if c.selected[i] {
			mark = "[x]"
		}
		cursor := "  "
		if i == c.cursor {
			cursor = "> "
		}
		line := fmt.Sprintf("%s%s %-12s %s  %s", cursor, mark, formatSize(candidate.Size), candidate.ModTime.Local().Format("2006-01-02 15:04"), candidate.Path)
		lines[i] = clip(line, cw)
	}
	if len(lines) == 0 {
		lines = []string{"No files match the retention rule."}
	}
	offset := c.offset
	if max := len(lines) - height; offset > max {
		offset = max
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
	return strings.Join(header, "\n") + "\n" + strings.Join(rows, "\n")
}

func (c cleanModel) summaryView() string {
	bytes, unknown := c.selectedTotals()
	force := "Safety lock will be revalidated."
	if c.force {
		force = warnStyle.Render("⚠ FORCE is ON: safety lock will be bypassed.")
	}
	if c.height > 0 && c.height < 10 {
		w := c.width
		if w < 10 {
			w = 10
		}
		lines := []string{
			warnStyle.Render("Confirm cloud deletion"),
			clip(fmt.Sprintf("Files: %d  Known: %s  Unknown: %d", c.selectedCount(), humanize.IBytes(uint64(bytes)), unknown), w),
			clip("Remote: "+c.remoteDest(), w),
			fmt.Sprintf("Retention: %d day(s)", c.cfg.RemoteRetentionDays),
			force,
			"Enter to continue; esc to cancel.",
		}
		if len(lines) > c.height {
			lines = lines[:c.height]
		}
		return strings.Join(lines, "\n")
	}
	return warnStyle.Render("Confirm cloud deletion") + fmt.Sprintf("\n\nFiles: %d  Known size: %s  Unknown sizes: %d\nRemote: %s\nRetention: %d day(s)\n%s\n\nPress enter to continue to typed confirmation, or esc to cancel.", c.selectedCount(), humanize.IBytes(uint64(bytes)), unknown, c.remoteDest(), c.cfg.RemoteRetentionDays, force)
}

func (c cleanModel) phraseView() string {
	want := "DELETE " + c.remoteDest()
	extra := ""
	if c.err != nil {
		extra = "\n" + errorStyle.Render(c.err.Error())
	}
	return errorStyle.Render("Final confirmation") + "\n\nType exactly: " + want + "\n\n" + c.input.View() + extra
}

func (c cleanModel) footerHint() string {
	switch c.state {
	case clIntro:
		return "enter dry-run • f toggle force • esc back"
	case clReport:
		return "↑/↓ navigate • space toggle • a all/none • x delete • r preview • esc back"
	case clConfirmSummary:
		return "enter typed confirmation • esc cancel"
	case clConfirmPhrase:
		return "enter confirm • esc cancel"
	case clOutdated:
		return "r new dry-run • esc back"
	case clError:
		return "r retry • enter/esc back"
	case clDone:
		return "enter/esc back • q quit"
	default:
		return "working… • q quit"
	}
}
