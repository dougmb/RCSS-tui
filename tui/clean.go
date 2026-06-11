package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// Clean screen: previews remote deletions with a dry-run first (started
// automatically on entry), then requires an explicit key to execute the real
// deletion. The safety lock lives in backup.Clean and surfaces here as an
// error state.

type cleanState int

const (
	clDryRunning cleanState = iota
	clDryDone
	clRunning
	clDone
	clError
)

type cleanModel struct {
	cfg config.Config
	rc  *rclone.Client

	state   cleanState
	spinner spinner.Model
	stream  *opStream
	output  []string
	err     error
	height  int
}

func newCleanModel(cfg config.Config, rc *rclone.Client) cleanModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return cleanModel{cfg: cfg, rc: rc, spinner: sp, state: clDryRunning}
}

func (c *cleanModel) setHeight(h int) { c.height = h }

// run launches backup.Clean (dry-run or real) in a goroutine, streaming output.
func (c cleanModel) run(dry bool) (cleanModel, tea.Cmd) {
	if dry {
		c.state = clDryRunning
	} else {
		c.state = clRunning
	}
	c.output = nil
	stream := newOpStream()
	c.stream = stream

	cfg, rc := c.cfg, c.rc
	go func() {
		logPath, _ := cfg.ResolveLogFile()
		log, _ := backup.NewLogger(logPath, stream.sink(), false)
		err := backup.Clean(context.Background(), cfg, rc, log, backup.CleanOptions{DryRun: dry})
		log.Close()
		stream.finish(err)
	}()

	return c, tea.Batch(stream.wait(), c.spinner.Tick)
}

func (c cleanModel) Update(msg tea.Msg) (cleanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case opEvent:
		if msg.done {
			switch {
			case msg.err != nil:
				c.state, c.err = clError, msg.err
			case c.state == clDryRunning:
				c.state = clDryDone
			default:
				c.state = clDone
			}
			return c, nil
		}
		c.output = append(c.output, msg.line)
		return c, c.stream.wait()

	case spinner.TickMsg:
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		return c, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return c, tea.Quit
		case "esc", "backspace":
			if c.state == clRunning || c.state == clDryRunning {
				return c, nil // don't leave mid-operation
			}
			return c, func() tea.Msg { return goBackMsg{} }
		case "x":
			if c.state == clDryDone {
				return c.run(false) // execute the real deletion
			}
		case "enter":
			if c.state == clDone || c.state == clError {
				return c, func() tea.Msg { return goBackMsg{} }
			}
		}
	}
	return c, nil
}

func (c cleanModel) View() string {
	switch c.state {
	case clDryRunning:
		return outputView(c.spinner.View()+" Previewing deletions (dry-run)…", c.output, c.height)
	case clDryDone:
		heading := titleStyle.Render("Dry-run complete — nothing deleted yet.")
		return outputView(heading, c.output, c.height)
	case clRunning:
		return outputView(c.spinner.View()+" Deleting old remote backups…", c.output, c.height)
	case clDone:
		return outputView(titleStyle.Render("✓ Cleanup complete."), c.output, c.height)
	case clError:
		return outputView(errorStyle.Render("✗ Clean aborted: "+c.err.Error()), c.output, c.height)
	}
	return ""
}

func (c cleanModel) footerHint() string {
	switch c.state {
	case clDryDone:
		return "x execute deletion • esc back • q quit"
	case clDone, clError:
		return "enter/esc back • q quit"
	default:
		return "working… • q quit"
	}
}
