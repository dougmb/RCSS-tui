package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// Backups screen: browse remote projects → files and restore a chosen file.
// It reimplements restoreBackup.sh's interactive selection in Go (calling
// backup.ListProjects/ListFiles/Restore), streaming rclone progress to the UI.

type backupsState int

const (
	bsLoadingProjects backupsState = iota
	bsProjects
	bsLoadingFiles
	bsFiles
	bsConfirmDest
	bsRestoring
	bsDone
	bsError
)

// --- messages ---

type projectsLoadedMsg struct {
	entries []backup.RemoteEntry
	err     error
}

// entryItem is a top-level backup entry: a project folder or a loose file.
type entryItem struct {
	name  string
	isDir bool
}

func (e entryItem) Title() string {
	if e.isDir {
		return "📁 " + e.name
	}
	return "📄 " + e.name
}
func (e entryItem) Description() string {
	if e.isDir {
		return "project folder"
	}
	return "file"
}
func (e entryItem) FilterValue() string { return e.name }

type filesLoadedMsg struct {
	files []string
	err   error
}

// backupsModel is the backups screen's sub-model.
type backupsModel struct {
	cfg config.Config
	rc  *rclone.Client

	state           backupsState
	projects        list.Model
	files           list.Model
	selectedProject string
	selectedFile    string

	// destInput edits the local restore destination before the download. It is
	// pre-filled with backup.RestoreTarget (the configured restore destination or
	// the backup source) and the edit applies to this run only.
	destInput textinput.Model

	spinner spinner.Model
	stream  *opStream
	output  []string

	height int
	err    error
}

func newBackupsModel(cfg config.Config, rc *rclone.Client) backupsModel {
	mkList := func(title string) list.Model {
		l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
		l.Title = title
		l.SetShowStatusBar(false)
		l.SetShowHelp(false)
		l.Styles.Title = titleStyle
		return l
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = ""

	return backupsModel{
		cfg:       cfg,
		rc:        rc,
		state:     bsLoadingProjects,
		projects:  mkList("Select a backup (📁 folder or 📄 file)"),
		files:     mkList("Select file to restore"),
		destInput: ti,
		spinner:   sp,
	}
}

func (b *backupsModel) setSize(w, h int) {
	b.height = h
	b.projects.SetSize(w, h)
	b.files.SetSize(w, h)
	iw := w - 2
	if iw < 10 {
		iw = 10
	}
	b.destInput.Width = iw
}

// filtering reports whether either list is currently capturing filter text, so
// the root can avoid treating `/` or `?` as a global shortcut.
func (b backupsModel) filtering() bool {
	return b.projects.FilterState() == list.Filtering ||
		b.files.FilterState() == list.Filtering
}

// loadProjects lists the remote top-level entries (project folders and loose
// files).
func (b backupsModel) loadProjects() tea.Cmd {
	cfg, rc := b.cfg, b.rc
	return func() tea.Msg {
		es, err := backup.ListTopLevel(context.Background(), cfg, rc)
		return projectsLoadedMsg{entries: es, err: err}
	}
}

// loadFiles lists the files of the selected project (newest first).
func (b backupsModel) loadFiles() tea.Cmd {
	cfg, rc, project := b.cfg, b.rc, b.selectedProject
	return func() tea.Msg {
		fs, err := backup.ListFiles(context.Background(), cfg, rc, project)
		return filesLoadedMsg{files: fs, err: err}
	}
}

// enterConfirm moves to the destination-confirm step, pre-filling the editable
// local path with what backup.Restore would use by default (the configured
// restore destination or the backup source, plus the project sub-folder).
func (b backupsModel) enterConfirm() (backupsModel, tea.Cmd) {
	target, err := backup.RestoreTarget(b.cfg, b.selectedProject)
	if err != nil {
		b.state, b.err = bsError, err
		return b, nil
	}
	b.destInput.SetValue(target)
	b.destInput.CursorEnd()
	b.destInput.Focus()
	b.state = bsConfirmDest
	return b, textinput.Blink
}

// startRestore launches backup.Restore in a goroutine, streaming its output.
// outputPath is the confirmed local destination for this run.
func (b backupsModel) startRestore(file, outputPath string) (backupsModel, tea.Cmd) {
	b.state = bsRestoring
	b.output = nil
	stream := newOpStream()
	b.stream = stream

	cfg, rc, project := b.cfg, b.rc, b.selectedProject
	go func() {
		// Restore logs to the terminal only (no backup log), like the original
		// restoreBackup.sh; the sink streams every line to the UI.
		log, _ := backup.NewLogger("", stream.sink(), false)
		err := backup.Restore(context.Background(), cfg, rc, log, project, file,
			backup.RestoreOptions{ShowProgress: true, OutputPath: outputPath})
		log.Close()
		stream.finish(err)
	}()

	return b, tea.Batch(stream.wait(), b.spinner.Tick)
}

// Update handles the backups screen's messages and keys.
func (b backupsModel) Update(msg tea.Msg) (backupsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case projectsLoadedMsg:
		if msg.err != nil {
			b.state, b.err = bsError, msg.err
			return b, nil
		}
		items := make([]list.Item, len(msg.entries))
		for i, e := range msg.entries {
			items[i] = entryItem{name: e.Name, isDir: e.IsDir}
		}
		b.projects.SetItems(items)
		b.state = bsProjects
		return b, nil

	case filesLoadedMsg:
		if msg.err != nil {
			b.state, b.err = bsError, msg.err
			return b, nil
		}
		b.files.SetItems(toItems(msg.files))
		b.state = bsFiles
		return b, nil

	case opEvent:
		if msg.done {
			if msg.err != nil {
				b.state, b.err = bsError, msg.err
			} else {
				b.state = bsDone
			}
			return b, nil
		}
		b.output = append(b.output, msg.line)
		return b, b.stream.wait()

	case spinner.TickMsg:
		var cmd tea.Cmd
		b.spinner, cmd = b.spinner.Update(msg)
		return b, cmd

	case tea.KeyMsg:
		return b.handleKey(msg)
	}
	return b, nil
}

func (b backupsModel) handleKey(msg tea.KeyMsg) (backupsModel, tea.Cmd) {
	// While a list is filtering, forward keys to it.
	switch b.state {
	case bsProjects:
		if b.projects.FilterState() == list.Filtering {
			var cmd tea.Cmd
			b.projects, cmd = b.projects.Update(msg)
			return b, cmd
		}
	case bsFiles:
		if b.files.FilterState() == list.Filtering {
			var cmd tea.Cmd
			b.files, cmd = b.files.Update(msg)
			return b, cmd
		}
	}

	// The destination-confirm step has a focused text field: forward typing to
	// it; only enter (restore) and esc (step back) are control keys. (ctrl+c
	// still quits — the root handles it before we get here.)
	if b.state == bsConfirmDest {
		switch msg.String() {
		case "enter":
			return b.startRestore(b.selectedFile, strings.TrimSpace(b.destInput.Value()))
		case "esc":
			if b.selectedProject != "" {
				b.state = bsFiles
			} else {
				b.state = bsProjects
			}
			return b, nil
		}
		var cmd tea.Cmd
		b.destInput, cmd = b.destInput.Update(msg)
		return b, cmd
	}

	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "esc", "backspace":
		// Step back within the screen, or leave to the menu.
		if b.state == bsFiles {
			b.state = bsProjects
			return b, nil
		}
		return b, func() tea.Msg { return goBackMsg{} }
	case "enter":
		switch b.state {
		case bsProjects:
			if it, ok := b.projects.SelectedItem().(entryItem); ok {
				if it.isDir {
					// Drill into a project folder to choose a file.
					b.selectedProject = it.name
					b.state = bsLoadingFiles
					return b, tea.Batch(b.loadFiles(), b.spinner.Tick)
				}
				// A loose file at the destination root: confirm the local
				// destination, then restore.
				b.selectedProject = ""
				b.selectedFile = it.name
				return b.enterConfirm()
			}
		case bsFiles:
			if it, ok := b.files.SelectedItem().(stringItem); ok {
				b.selectedFile = string(it)
				return b.enterConfirm()
			}
		case bsDone, bsError:
			return b, func() tea.Msg { return goBackMsg{} }
		}
		return b, nil
	}

	// Default: drive the active list.
	switch b.state {
	case bsProjects:
		var cmd tea.Cmd
		b.projects, cmd = b.projects.Update(msg)
		return b, cmd
	case bsFiles:
		var cmd tea.Cmd
		b.files, cmd = b.files.Update(msg)
		return b, cmd
	}
	return b, nil
}

// View renders the inner body of the backups screen (the root frames it and
// adds the footer).
func (b backupsModel) View() string {
	switch b.state {
	case bsLoadingProjects:
		return b.spinner.View() + " Loading backups…"
	case bsProjects:
		if len(b.projects.Items()) == 0 {
			return titleStyle.Render("Restore") + "\n\n" +
				subtitleStyle.Render("No backups found on the remote yet. Run an upload first.")
		}
		return b.projects.View()
	case bsLoadingFiles:
		return b.spinner.View() + " Loading files for " + b.selectedProject + "…"
	case bsFiles:
		return b.files.View()
	case bsConfirmDest:
		var sb strings.Builder
		sb.WriteString(titleStyle.Render("Restore — confirm destination"))
		sb.WriteString("\n\n")
		label := b.selectedFile
		if b.selectedProject != "" {
			label = b.selectedProject + "/" + b.selectedFile
		}
		sb.WriteString(subtitleStyle.Render("File: " + label))
		sb.WriteString("\n\n")
		sb.WriteString(subtitleStyle.Render("Restore to (local folder):"))
		sb.WriteString("\n")
		sb.WriteString(b.destInput.View())
		sb.WriteString("\n\n")
		sb.WriteString("Press enter to restore.")
		return sb.String()
	case bsRestoring:
		return b.viewOutput(b.spinner.View() + " Restoring…")
	case bsDone:
		return b.viewOutput(titleStyle.Render("✓ Restore complete."))
	case bsError:
		return errorStyle.Render("Error: " + b.err.Error())
	}
	return ""
}

// viewOutput shows a heading and the tail of the streamed restore output that
// fits the available height.
func (b backupsModel) viewOutput(heading string) string {
	lines := b.output
	maxLines := b.height - 4
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return heading + "\n\n" + subtitleStyle.Render(strings.Join(lines, "\n"))
}

// footerHint returns the key hints appropriate to the current state.
func (b backupsModel) footerHint() string {
	switch b.state {
	case bsProjects, bsFiles:
		return "↑/↓ move • enter select • / filter • esc back • q quit"
	case bsConfirmDest:
		return "edit folder • enter restore • esc back"
	case bsDone, bsError:
		return "enter/esc back • q quit"
	default:
		return "esc back • q quit"
	}
}
