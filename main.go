// Command rcss is the RCSS backup manager. Run without arguments it opens the
// terminal UI; with a subcommand it runs headless (for cron):
//
//	rcss upload [-v] [-p]            upload all projects
//	rcss clean  [-v] [--dry-run] [--force]   clean old remote backups
//
// Both subcommands reuse the same backup package as the TUI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
	"github.com/dougmb/rcss-tui/tui"
)

func main() {
	args := os.Args[1:]

	// No subcommand → launch the TUI.
	if len(args) == 0 {
		if err := runTUI(); err != nil {
			fmt.Fprintln(os.Stderr, "rcss:", err)
			os.Exit(1)
		}
		return
	}

	switch args[0] {
	case "upload":
		os.Exit(runUpload(args[1:]))
	case "clean":
		os.Exit(runClean(args[1:]))
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "rcss: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `rcss — Rclone Cloud Simple Scripts

Usage:
  rcss                 open the terminal UI
  rcss upload [-v] [-p]
  rcss clean  [-v] [--dry-run] [--force]
  rcss help

Flags:
  -v            verbose output
  -p            show rclone transfer progress (upload)
  --dry-run     preview deletions without removing anything (clean)
  --force       bypass the safety lock (clean) — dangerous
`)
}

// newContext returns a context cancelled on SIGINT/SIGTERM, so headless runs
// stop rclone cleanly when interrupted (e.g. by cron killing the job).
func newContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// setup loads config, verifies rclone is installed, and builds a logger that
// writes to the sync log and echoes every line to stdout (for cron logs).
func setup(verbose bool) (config.Config, *rclone.Client, *backup.Logger, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, nil, nil, err
	}

	rc := rclone.New()
	if err := rc.EnsureInstalled(); err != nil {
		return cfg, nil, nil, err
	}

	logPath, err := cfg.ResolveLogFile()
	if err != nil {
		return cfg, nil, nil, err
	}
	log, err := backup.NewLogger(logPath, func(line string) { fmt.Println(line) }, verbose)
	if err != nil {
		return cfg, nil, nil, err
	}
	return cfg, rc, log, nil
}

func runUpload(argv []string) int {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose output")
	progress := fs.Bool("p", false, "show rclone transfer progress")
	_ = fs.Parse(argv)

	cfg, rc, log, err := setup(*verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rcss:", err)
		return 1
	}
	defer log.Close()

	ctx, cancel := newContext()
	defer cancel()

	if _, err := backup.Upload(ctx, cfg, rc, log, backup.UploadOptions{ShowProgress: *progress}); err != nil {
		return 1
	}
	return 0
}

func runClean(argv []string) int {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose output")
	dryRun := fs.Bool("dry-run", false, "preview deletions without removing anything")
	force := fs.Bool("force", false, "bypass the safety lock (dangerous)")
	_ = fs.Parse(argv)

	cfg, rc, log, err := setup(*verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rcss:", err)
		return 1
	}
	defer log.Close()

	ctx, cancel := newContext()
	defer cancel()

	if err := backup.Clean(ctx, cfg, rc, log, backup.CleanOptions{DryRun: *dryRun, Force: *force}); err != nil {
		return 1
	}
	return 0
}

// runTUI loads the config and rclone client, then launches the terminal UI.
func runTUI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rc := rclone.New()
	if err := rc.EnsureInstalled(); err != nil {
		return err
	}
	return tui.Run(cfg, rc)
}
