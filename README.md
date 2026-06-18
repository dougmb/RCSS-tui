# RCSS-tui — Rclone Cloud Simple Scripts

A small, polished **terminal UI** (and headless CLI) for managing per-project
backups on any cloud remote supported by [`rclone`](https://rclone.org) —
typically Google Drive. RCSS uploads each project folder, prunes old backups
locally and in the cloud, restores files, and can schedule itself through your
operating system's scheduler (crontab on Linux/macOS, Task Scheduler on
Windows). It runs on Linux, macOS, and Windows, and supports multiple fully
isolated accounts (one per rclone remote).

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
| **Account** | Manage accounts (one per rclone remote): switch the active one, add via `rclone config`, or forget one |
| **Sync folder** | Choose the local root folder whose sub-folders are your projects |
| **Backups** | Browse remote projects → files and restore one, with live progress |
| **Back Up Now** | Copy all projects to the cloud now (one-way upload), streaming rclone progress |
| **Clean** | Remove old **cloud** backups: explains the criteria, previews with a dry-run, then deletes; optional Force (double-confirmed) bypasses the safety lock |
| **Settings** | Edit retention and behavior; saved to `config.toml` with a visible confirmation |
| **Schedule** | Install daily-backup / weekly-clean jobs into your OS scheduler (crontab / Task Scheduler) |
| **Logs** | Scroll the sync log with ERROR/WARN highlighting |
| **About** | Version, rclone/scheduler status, and config/log locations |

The UI requires a terminal of at least **80×24**; anything smaller renders a
single centered notice (`Not enough space to render panels`).

Global keys: `↑/↓` move · `enter`/`→` open · `1`–`9` jump to a screen (incl.
`9` About) · `esc` back · `/` filter (in lists) · `?` keyboard help · `q` /
`ctrl+c` quit.

If `rclone` is not installed, the UI still opens and warns about the missing
dependency; the cloud screens stay locked until you install it (the **Settings**
and **Logs** screens remain usable).

### Multiple accounts

RCSS supports **multiple accounts — one per rclone remote — fully isolated**
from each other: each has its own sync folder, destination, retention, log, and
schedule. The active account is shown at the top of the sidebar and on the
**About** screen.

From the **Account** screen: `enter` switches the active account (creating its
settings with defaults the first time a remote is chosen), `d` forgets an
account's RCSS settings (the rclone remote itself is left intact), and
**Configure a new account…** opens `rclone config` to add a remote. Settings,
Schedule, Backups, etc. all act on the active account.

### Headless (for cron)

The same backup engine runs without the UI — this is exactly what the Schedule
screen registers with your OS scheduler (per account):

```bash
rcss upload [-v] [-p] [--account NAME]            # back up an account's projects
rcss clean  [-v] [--dry-run] [--force] [--account NAME]   # remove old cloud backups
rcss help
```

`--account NAME` is the rclone remote (e.g. `drive:`); it defaults to the active
account.

## Configuration

Settings live in a single TOML file at `~/.config/rcss/config.toml` (respecting
`XDG_CONFIG_HOME`), created on first run and edited from the UI. It holds an
`active_account` and an `[[accounts]]` array — one entry per account — with
**no API secrets** (those stay in rclone's own config), so it is naturally
outside the repository. A legacy single-account config from older RCSS versions
is migrated automatically on first load.

Each account entry has these fields:

| Field | Default | Meaning |
| --- | --- | --- |
| `remote_name` | — | rclone remote, e.g. `drive:` (the account key) |
| `sync_root` | — | local folder whose sub-folders are projects |
| `drive_destination` | `Backups` | destination folder on the remote |
| `retention_days` | `1` | delete **local** files older than this after a successful upload |
| `remote_retention_days` | `15` | delete **cloud** files older than this on clean |
| `remote_cleanup_safety_days` | `2` | clean is blocked unless a backup newer than this exists |
| `delete_after_upload` | `false` | delete all local files right after a successful upload |
| `skip_dotfiles` | `false` | exclude hidden files/folders from uploads |
| `ignored_folders` | `scripts config bin logs lost+found` | sub-folders never treated as projects |
| `log_file` | (config dir)/`sync-<account>.log` | append-only run log (per account) |

## Safety guarantees

- **Local files are deleted only after a successful upload** of that project —
  a failed upload never removes local data.
- **Remote cleanup is locked** unless a recent backup (within
  `remote_cleanup_safety_days`) exists on the remote, so a silently-stopped
  upload cron cannot let cleanup wipe your history. `--force` bypasses the lock
  and is intentionally dangerous.
- In the UI, **Clean always previews with a dry-run first**; the real deletion
  requires an explicit keypress. Clean only ever deletes **cloud** files — local
  files are pruned by Back Up Now. The optional **Force** toggle bypasses the
  safety lock and is double-confirmed in the UI (and is `--force` headless).
- Scheduling only ever touches RCSS-managed entries: a single delimited
  `# >>> RCSS-managed >>>` … `# <<< RCSS-managed <<<` block in your crontab on
  Unix, or the `RCSS-Upload` / `RCSS-Clean` tasks on Windows. Every other entry
  is preserved, clearing the schedule removes just those, and neither needs
  root/admin rights.

## Project layout

```
main.go      entrypoint: no args → TUI; `upload`/`clean` → headless (cron)
config/      config.toml model + Load/Save + defaults
rclone/      thin wrapper over the rclone binary (ListRemotes, Lsf, Copy, Delete)
backup/      ported Upload / Clean / Restore + logging (safety invariants)
scheduler/   install/remove RCSS jobs in the OS scheduler (crontab / Task Scheduler)
tui/         Bubbletea root model + styles + one file per screen
```

## License

See repository.
