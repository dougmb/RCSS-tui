package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// Upload screen: runs backup.Upload, streaming rclone progress and the log
// lines to the UI. Same code path as `rcss upload` (headless/cron).

type uploadState int

const (
	upIdle uploadState = iota
	upRunning
	upDone
	upError
)

type uploadModel struct {
	cfg config.Config
	rc  *rclone.Client

	state   uploadState
	spinner spinner.Model
	stream  *opStream
	output  []string
	result  backup.UploadResult
	err     error
	height  int
}

func newUploadModel(cfg config.Config, rc *rclone.Client) uploadModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return uploadModel{cfg: cfg, rc: rc, spinner: sp, state: upIdle}
}

func (u *uploadModel) setHeight(h int) { u.height = h }

// start launches backup.Upload in a goroutine, streaming its output.
func (u uploadModel) start() (uploadModel, tea.Cmd) {
	u.state = upRunning
	u.output = nil
	stream := newOpStream()
	u.stream = stream

	cfg, rc := u.cfg, u.rc
	go func() {
		logPath, _ := cfg.ResolveLogFile()
		log, _ := backup.NewLogger(logPath, stream.sink(), false)
		res, err := backup.Upload(context.Background(), cfg, rc, log,
			backup.UploadOptions{ShowProgress: true})
		log.Close()
		stream.finishWith(res, err)
	}()

	return u, tea.Batch(stream.wait(), u.spinner.Tick)
}

func (u uploadModel) Update(msg tea.Msg) (uploadModel, tea.Cmd) {
	switch msg := msg.(type) {
	case opEvent:
		if msg.done {
			if res, ok := msg.payload.(backup.UploadResult); ok {
				u.result = res
			}
			if msg.err != nil {
				u.state, u.err = upError, msg.err
			} else {
				u.state = upDone
			}
			return u, nil
		}
		u.output = append(u.output, msg.line)
		return u, u.stream.wait()

	case spinner.TickMsg:
		var cmd tea.Cmd
		u.spinner, cmd = u.spinner.Update(msg)
		return u, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return u, tea.Quit
		case "esc", "backspace":
			if u.state == upRunning {
				return u, nil // don't leave mid-upload
			}
			return u, func() tea.Msg { return goBackMsg{} }
		case "enter":
			switch u.state {
			case upIdle:
				return u.start()
			case upDone, upError:
				return u, func() tea.Msg { return goBackMsg{} }
			}
		}
	}
	return u, nil
}

func (u uploadModel) View() string {
	switch u.state {
	case upIdle:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Back Up Now"))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("Copies each project sub-folder to the cloud (one-way upload)."))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("Root:   " + u.cfg.SyncRoot))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Remote: %s/%s", u.cfg.RemoteName, u.cfg.DriveDestination)))
		b.WriteString("\n\n")
		b.WriteString("Press enter to start the backup.")
		return b.String()
	case upRunning:
		return outputView(u.spinner.View()+" Backing up…", u.output, u.height)
	case upDone:
		heading := titleStyle.Render(fmt.Sprintf("✓ Backup %s in %s — %d removed, %d errors",
			u.result.Status, u.result.Duration.Round(1e9), u.result.FilesDeleted, u.result.UploadErrors))
		return outputView(heading, u.output, u.height)
	case upError:
		return outputView(errorStyle.Render("✗ Backup failed: "+u.err.Error()), u.output, u.height)
	}
	return ""
}

func (u uploadModel) footerHint() string {
	switch u.state {
	case upIdle:
		return "enter start • esc back • q quit"
	case upRunning:
		return "backing up… • q quit"
	default:
		return "enter/esc back • q quit"
	}
}

// outputView renders a heading and the tail of streamed output that fits the
// available height. Shared by the upload and clean screens.
func outputView(heading string, output []string, height int) string {
	lines := output
	maxLines := height - 4
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return heading + "\n\n" + subtitleStyle.Render(strings.Join(lines, "\n"))
}
