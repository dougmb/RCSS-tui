# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

RCSS (Rclone Cloud Simple Scripts) is a Go program — a Bubbletea terminal UI plus a headless CLI — for managing per-project backups on an `rclone` cloud remote (typically Google Drive). It uploads each project folder, prunes old backups locally and remotely, restores files, and schedules itself via the user's crontab.

The backup logic was **ported to Go from three original Bash scripts** (`uploadBackup.sh`, `cleanRemoteBackups.sh`, `restoreBackup.sh`), which remain in the repo root as read-only reference (along with `backup.env`). The Go code — not the scripts — is now the source of truth. `plan.md` documents the port plan and decisions.

`rclone` is the only runtime dependency; it stores cloud credentials in its own config. RCSS never handles API secrets.

## Build / run / verify

```bash
go build ./...        # build everything
go vet ./...          # vet — keep clean
go build -o rcss .    # produce the binary

./rcss                       # open the TUI
./rcss upload [-v] [-p]      # headless upload (what cron runs)
./rcss clean [-v] [--dry-run] [--force]
```

There is no test suite yet. Verify changes by building/vetting and by driving the relevant package with a **fake `rclone`** on the PATH (a shell script that echoes canned `lsf`/`copy`/`delete` output) — this is how the backup logic and TUI flows are exercised without a real remote. For TUI work, sub-models can be driven headless by feeding `tea.Msg`s into `Update` and inspecting `View()`.

## Architecture

```
main.go      entrypoint: no args → TUI; `upload`/`clean` → headless (cron). Shared backup engine either way.
config/      Config struct + Load/Save of ~/.config/rcss/config.toml (XDG-aware) + recommended defaults.
rclone/      thin wrapper over the rclone binary: ListRemotes, Lsf, Copy, Delete, EnsureInstalled (PATH check).
backup/      ported business logic: Upload, Clean, Restore (+ ListProjects/ListFiles) and the Logger.
cron/        read/write a single delimited block in the user's crontab.
tui/         Bubbletea root model (app.go) + styles.go + one file per screen.
```

**Config** is a single `~/.config/rcss/config.toml` (no more `backup.env`, no `source`-ing shell). `config.Load()` creates it with defaults on first run; the Settings screen and Account/Folder screens persist changes via `config.Save()`. Required fields are checked by `Config.Validate()`. Two distinct retention concepts — keep them separate: `RetentionDays` (local cleanup after upload) vs `RemoteRetentionDays` (cloud cleanup).

**rclone wrapper**: list-style commands use `output()` (capture stdout, stderr → error). Long operations use `stream()` — one `os.Pipe` receives stdout+stderr and a custom `bufio.SplitFunc` (`scanLinesOrCR`) splits on `\n` **and** `\r`, so `-P` progress updates surface live. All calls take a `context.Context`.

**backup package** mirrors the original scripts and ports their logging (`Logger` writes timestamped, fixed-width-level lines to `sync.log` and to a sink callback; the upload SYNC SUMMARY block is appended verbatim). The Logger sink is what makes one engine serve both the UI (sink → Bubbletea msgs) and headless mode (sink → stdout).

## Safety invariants — preserve these when editing

- **Upload performs local cleanup ONLY inside the success branch** of a project's `rclone copy`. A failed upload increments the error count and `continue`s; it must never delete local files. (`backup/upload.go`)
- **Clean enforces a safety lock**: before deleting it confirms a backup newer than `RemoteCleanupSafetyDays` exists on the remote (`rclone lsf --max-age`), aborting with `ErrNoRecentBackup` otherwise. `CleanOptions.Force` bypasses it — treat as dangerous. (`backup/clean.go`)
- **Clean starts from a dry-run** in the UI; real deletion requires an explicit key. (`tui/clean.go`)
- **Restore logs to the terminal only** (`NewLogger("")`), like the original interactive script — it does not append to `sync.log`.
- **Cron edits touch only the `# >>> RCSS-managed >>>` … `# <<< RCSS-managed <<<` block**; all other crontab lines are preserved. Disabling all presets removes the block. (`cron/cron.go`)

## TUI conventions (Elm architecture)

- The root `Model` (`tui/app.go`) holds `width/height`, the active `screen` enum, and one sub-model per screen. It enforces the **80×24 minimum** guard, routes `tea.WindowSizeMsg`/keys/screen-switches, and frames each screen with a shared footer (`withFooter`). Global keys: `ctrl+c` always quits; `q` quits; `esc` goes back.
- **Sub-models are value types** with `Update(msg) (subModel, tea.Cmd)` and `View() string`; they communicate upward with small messages (`switchScreenMsg`, `goBackMsg`, `remoteChosenMsg`, `folderChosenMsg`, `settingsSavedMsg`). The root sets `m.cfg` on these and recreates cfg-dependent sub-models on screen entry.
- Sub-models that get sized on `WindowSizeMsg` (lists, huh forms, viewport) **must be initialized in `New`**, or `SetSize`/`WithWidth` will panic on a zero value.
- Streaming backup operations use `tui/stream.go`'s `opStream`: the operation runs in a goroutine writing lines through the Logger sink; `opStream.wait()` is a command that delivers one `opEvent` at a time (buffered channel = backpressure). `finishWith` carries a typed result (e.g. `UploadResult`).
- Reuse the shared lipgloss styles in `styles.go`; don't inline colors.

## Reference scripts

`*.sh` and `backup.env` are kept as a portability reference and are **not** part of the build. When the Go port is fully trusted they can be removed (they remain in git history). Don't add new logic to them.
