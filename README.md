# RCSS-tui — Rclone Cloud Simple Scripts

A small, polished **terminal UI** (and headless CLI) for managing per-project
backups on any cloud remote supported by [`rclone`](https://rclone.org) —
typically Google Drive. RCSS uploads each project folder, prunes old backups
locally and in the cloud, restores files, and can schedule itself through your
crontab.

It is a **pure Go** application built on [Bubbletea](https://github.com/charmbracelet/bubbletea):
a single self-contained binary that drives the `rclone` binary directly. There
are no Bash scripts and no `backup.env` — all configuration lives in one
`config.toml`. `rclone` is the only runtime dependency and keeps your cloud
credentials in its own config; RCSS never handles API secrets.

> RCSS began as three Bash scripts. Those live on as a portability reference in
> the original repository ([`dougmb/RCSS`](https://github.com/dougmb/RCSS)); this
> repo is the standalone Go rewrite. See `plan.md` for the port plan and the
> confirmed design decisions.

## Stack

- `charmbracelet/bubbletea` — framework (Elm architecture)
- `charmbracelet/lipgloss` — styles / theme
- `charmbracelet/bubbles` — list, filepicker, viewport, progress, spinner
- `charmbracelet/huh` — forms (Settings, Schedule)
- `BurntSushi/toml` — config file
- Runtime: the `rclone` binary on your `PATH`

## Install

```bash
# Requires Go and rclone on your PATH
git clone https://github.com/dougmb/RCSS-tui.git
cd RCSS-tui
go build -o rcss .
./rcss
```

Make sure `rclone` is installed and has at least one remote configured:

```bash
rclone config        # set up a Google Drive (or other) remote
```

## Usage

### Terminal UI

```bash
rcss
```

Opens the full UI. From the main menu:

| Screen | What it does |
| --- | --- |
| **Account** | Pick the rclone remote, or launch `rclone config` to add a new one |
| **Backup folder** | Choose the local root folder whose sub-folders are your projects |
| **Backups** | Browse remote projects → files and restore one, with live progress |
| **Upload** | Run a backup now, streaming rclone progress |
| **Clean** | Preview (dry-run) then delete old remote backups |
| **Settings** | Edit retention and behavior; saved to `config.toml` |
| **Schedule** | Install daily-upload / weekly-clean jobs into your crontab |
| **Logs** | Scroll the sync log with ERROR/WARN highlighting |

The UI requires a terminal of at least **80×24**; anything smaller renders a
single centered notice (`Not enough space to render panels`).

Global keys: `↑/↓` move · `enter` select · `esc` back · `q` / `ctrl+c` quit ·
`/` filter (in lists).

### Headless (for cron)

The same backup engine runs without the UI — this is exactly what the Schedule
screen writes into your crontab:

```bash
rcss upload [-v] [-p]                    # upload all projects
rcss clean  [-v] [--dry-run] [--force]   # remove old remote backups
rcss help
```

## Configuration

Settings live in a single TOML file at `~/.config/rcss/config.toml` (respecting
`XDG_CONFIG_HOME`), created with recommended defaults on first run and edited
from the **Settings** screen. It holds paths and the remote name — **no API
secrets** (those stay in rclone's own config), so it is naturally outside the
repository.

| Field | Default | Meaning |
| --- | --- | --- |
| `remote_name` | — | rclone remote, e.g. `drive:` |
| `backup_root` | — | local folder whose sub-folders are projects |
| `drive_destination` | `Backups` | destination folder on the remote |
| `retention_days` | `1` | delete **local** files older than this after a successful upload |
| `remote_retention_days` | `15` | delete **cloud** files older than this on clean |
| `remote_cleanup_safety_days` | `2` | clean is blocked unless a backup newer than this exists |
| `delete_after_upload` | `false` | delete all local files right after a successful upload |
| `skip_dotfiles` | `false` | exclude hidden files/folders from uploads |
| `ignored_folders` | `scripts config bin logs lost+found` | sub-folders never treated as projects |
| `log_file` | (config dir)/`sync.log` | append-only run log |

## Safety guarantees

- **Local files are deleted only after a successful upload** of that project —
  a failed upload never removes local data.
- **Remote cleanup is locked** unless a recent backup (within
  `remote_cleanup_safety_days`) exists on the remote, so a silently-stopped
  upload cron cannot let cleanup wipe your history. `--force` bypasses the lock
  and is intentionally dangerous.
- In the UI, **Clean always starts with a dry-run**; the real deletion requires
  an explicit keypress.
- Scheduling only ever edits a single delimited
  `# >>> RCSS-managed >>>` … `# <<< RCSS-managed <<<` block in your crontab;
  every other entry is preserved, and clearing the schedule removes just that
  block.

## Project layout

```
main.go      entrypoint: no args → TUI; `upload`/`clean` → headless (cron)
config/      config.toml model + Load/Save + defaults
rclone/      thin wrapper over the rclone binary (ListRemotes, Lsf, Copy, Delete)
backup/      ported Upload / Clean / Restore + logging (safety invariants)
cron/        read/write the managed crontab block
tui/         Bubbletea root model + styles + one file per screen
```

## License

See repository.
