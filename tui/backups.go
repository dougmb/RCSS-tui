package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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

	return backupsModel{
		cfg:      cfg,
		rc:       rc,
		state:    bsLoadingProjects,
		projects: mkList("Select a backup (📁 folder or 📄 file)"),
		files:    mkList("Select file to restore"),
		spinner:  sp,
	}
}

func (b *backupsModel) setSize(w, h int) {
	b.height = h
	b.projects.SetSize(w, h)
	b.files.SetSize(w, h)
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

// startRestore launches backup.Restore in a goroutine, streaming its output.
func (b backupsModel) startRestore(file string) (backupsModel, tea.Cmd) {
	b.state = bsRestoring
	b.output = nil
	stream := newOpStream()
	b.stream = stream

	cfg, rc, project := b.cfg, b.rc, b.selectedProject
	go func() {
		// Restore logs to the terminal only (no sync.log), like the original
		// restoreBackup.sh; the sink streams every line to the UI.
		log, _ := backup.NewLogger("", stream.sink(), false)
		err := backup.Restore(context.Background(), cfg, rc, log, project, file,
			backup.RestoreOptions{ShowProgress: true})
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
				// A loose file at the destination root: restore it directly.
				b.selectedProject = ""
				return b.startRestore(it.name)
			}
		case bsFiles:
			if it, ok := b.files.SelectedItem().(stringItem); ok {
				return b.startRestore(string(it))
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
			return titleStyle.Render("Backups") + "\n\n" +
				subtitleStyle.Render("No backups found on the remote yet. Run an upload first.")
		}
		return b.projects.View()
	case bsLoadingFiles:
		return b.spinner.View() + " Loading files for " + b.selectedProject + "…"
	case bsFiles:
		return b.files.View()
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
	case bsDone, bsError:
		return "enter/esc back • q quit"
	default:
		return "esc back • q quit"
	}
}
