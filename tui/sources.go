package tui

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Backup source screen: a manager for the account's list of source folders.
// Each folder is uploaded as its own backup. The user adds folders with the
// directory picker (folderModel) and removes them from the list; pressing enter
// saves the list (sourcesSavedMsg), esc discards.

// sourcesSavedMsg carries the edited source-folder list back to the root.
type sourcesSavedMsg struct{ folders []string }

type sourcesModel struct {
	folders []string
	cursor  int

	picking bool        // true while the add-folder picker is open
	picker  folderModel // reused directory picker
	start   string      // directory the picker opens at

	status        string // transient notice (e.g. duplicate warning)
	width, height int

	// done flips to true once the root has saved the source folders.
	done    bool
	saveErr error
}

func newSourcesModel(folders []string, start string) sourcesModel {
	return sourcesModel{folders: append([]string(nil), folders...), start: start}
}

func (s sourcesModel) Init() tea.Cmd { return nil }

func (s *sourcesModel) setSize(w, h int) {
	s.width, s.height = w, h
	s.picker.setHeight(h - 4)
}

func (s sourcesModel) Update(msg tea.Msg) (sourcesModel, tea.Cmd) {
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

	// The embedded picker's selection bubbles up to the root and is routed back
	// here as a folderChosenMsg; add it to the list rather than letting it leave
	// this screen.
	if fc, ok := msg.(folderChosenMsg); ok {
		s.picking = false
		s.addFolder(fc.path)
		return s, nil
	}
	if s.picking {
		return s.updatePicking(msg)
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "q":
		return s, tea.Quit
	case "esc":
		return s, func() tea.Msg { return goBackMsg{} }
	case "enter":
		s.done = true
		return s, tea.Batch(
			func() tea.Msg { return sourcesSavedMsg{folders: append([]string(nil), s.folders...)} },
			tea.Tick(saveConfirmationTimeout, func(time.Time) tea.Msg { return doneTimeoutMsg{} }),
		)
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.folders)-1 {
			s.cursor++
		}
	case "a":
		s.picking = true
		s.picker = newFolderModel(s.start)
		s.picker.setHeight(s.height - 4)
		s.status = ""
		return s, s.picker.Init()
	case "d", "delete", "x":
		if len(s.folders) > 0 {
			s.folders = append(s.folders[:s.cursor], s.folders[s.cursor+1:]...)
			if s.cursor >= len(s.folders) && s.cursor > 0 {
				s.cursor--
			}
		}
	}
	return s, nil
}

// updatePicking drives the directory picker while adding a folder. esc cancels
// the add (back to the list) instead of leaving the screen.
func (s sourcesModel) updatePicking(msg tea.Msg) (sourcesModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			s.picking = false
			return s, nil
		case "q":
			return s, tea.Quit
		}
	}
	var cmd tea.Cmd
	s.picker, cmd = s.picker.Update(msg)
	return s, cmd
}

// addFolder appends a picked folder, skipping exact duplicates and warning when
// another source shares its basename (they would merge on the remote).
func (s *sourcesModel) addFolder(path string) {
	path = filepath.Clean(path)
	for _, f := range s.folders {
		if filepath.Clean(f) == path {
			s.status = "Already added: " + path
			return
		}
	}
	name := filepath.Base(path)
	for _, f := range s.folders {
		if filepath.Base(f) == name {
			s.status = "⚠ Another source is also named “" + name + "” — they will merge on the remote."
			break
		}
	}
	s.folders = append(s.folders, path)
	s.cursor = len(s.folders) - 1
}

func (s sourcesModel) View() string {
	if s.done {
		return s.doneView()
	}
	if s.picking {
		return s.picker.View()
	}

	w := s.width - 2
	if w < 10 {
		w = 10
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Backup source folders"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Each folder is uploaded as its own backup (remote/<folder-name>)."))
	b.WriteString("\n\n")

	if len(s.folders) == 0 {
		b.WriteString(subtitleStyle.Render("No folders yet — press a to add one."))
	} else {
		for i, f := range s.folders {
			line := clip(f, w-2)
			if i == s.cursor {
				b.WriteString(titleStyle.Render("› " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteString("\n")
		}
	}
	if s.status != "" {
		b.WriteString("\n" + subtitleStyle.Render(s.status))
	}
	return b.String()
}

// doneView renders the save confirmation before auto-returning to the menu.
func (s sourcesModel) doneView() string {
	if s.saveErr != nil {
		return errorStyle.Render("✗ Could not save backup sources") + "\n\n" +
			subtitleStyle.Render(s.saveErr.Error())
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("✓ Backup sources saved"))
	b.WriteString("\n\n")
	if len(s.folders) == 0 {
		b.WriteString(subtitleStyle.Render("No folders configured."))
	} else {
		for _, f := range s.folders {
			b.WriteString("  • ")
			b.WriteString(clip(f, s.width-6))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (s sourcesModel) footerHint() string {
	if s.done {
		return "enter/esc back • q quit"
	}
	if s.picking {
		return "→/l open • ←/h up • enter add this folder • esc cancel"
	}
	return "↑/↓ move • a add • d remove • enter save • esc cancel"
}
