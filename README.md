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

> **rclone is required.** RCSS drives the [`rclone`](https://rclone.org) binary
> for every cloud operation and does nothing useful until rclone is installed
> and has a configured remote. See [Prerequisite: rclone](#prerequisite-rclone)
> before your first run.

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

You also need **rclone** installed with at least one configured remote — see
[Prerequisite: rclone](#prerequisite-rclone) next.

## Prerequisite: rclone

RCSS is a front-end for [`rclone`](https://rclone.org): it shells out to the
`rclone` binary for every upload, restore, and cleanup, and relies on rclone to
keep your cloud credentials in its own config. Install rclone and configure at
least one remote before RCSS can back anything up.

### Installing rclone

The official script installs the latest rclone on Linux and macOS:

```bash
sudo -v ; curl https://rclone.org/install.sh | sudo bash
```

Or use your package manager:

| Platform | Command |
| --- | --- |
| Arch Linux | `sudo pacman -S rclone` |
| Debian / Ubuntu | `sudo apt install rclone` |
| Fedora | `sudo dnf install rclone` |
| openSUSE | `sudo zypper install rclone` |
| macOS (Homebrew) | `brew install rclone` |
| Windows (winget) | `winget install Rclone.Rclone` |

On Windows you can instead use `scoop install rclone`, `choco install rclone`, or
download the zip from [rclone.org/downloads](https://rclone.org/downloads/) and
add `rclone.exe` to your `PATH`. Distro packages (apt/dnf/…) can lag a few
releases behind the official script.

Confirm it's on your `PATH`:

```bash
rclone version
```

### Configuring a remote (Google Drive example)

Create a remote with the interactive wizard:

```bash
rclone config
```

Then, for Google Drive:

1. `n` — **New remote**.
2. Name it, e.g. `drive` (this name becomes the account key in RCSS).
3. Storage type: choose `drive` (Google Drive).
4. `client_id` / `client_secret`: leave **blank** to use rclone's defaults.
5. Scope: `1` (full access) is the usual choice.
6. `root_folder_id` / `service_account_file`: leave blank.
7. Edit advanced config? `N`.
8. Use auto config? `Y` — rclone opens your browser to authorize Google.
9. Confirm `y` to save.

You now have a `drive:` remote. RCSS refers to remotes **with the trailing
colon** (`drive:`) and stores each project at `<remote>/<destination>/<project>/`.
The destination folder is configurable (in **Settings**, and editable when you
run a backup); it defaults to **blank**, meaning the account root —
`<remote>/<project>/`.

> You can also run this wizard **from inside RCSS**: on the **Rclone Account**
> screen choose `＋ Configure a new account…`, which suspends the UI, runs
> `rclone config`, then returns.

### Encrypting backups (rclone crypt)

To encrypt file names and contents in the cloud, wrap your remote with rclone's
[`crypt`](https://rclone.org/crypt/) backend. Run `rclone config` again and add a
second remote:

1. `n` — **New remote**; name it e.g. `secret`.
2. Storage type: `crypt`.
3. `remote`: the path to encrypt, e.g. `drive:Encrypted` (a folder on the remote
   configured above).
4. `filename_encryption`: `standard` (also encrypts file names).
5. `directory_name_encryption`: `true`.
6. Set a **password** (and optionally a second password/salt) — let rclone
   generate strong ones if you prefer.
7. Confirm to save.

Now point RCSS at the **`secret:`** remote instead of `drive:` (select it on the
**Rclone Account** screen). Encryption is fully transparent to RCSS: it uploads,
lists, and restores through the crypt remote, while everything stored on Google
Drive is encrypted. A plain account and an encrypted one can even coexist as two
separate RCSS accounts.

> **Keep your crypt password safe.** It is the only key to your encrypted
> backups — rclone does not store a recoverable copy. If you lose it, the
> backups are unrecoverable. See the [crypt docs](https://rclone.org/crypt/) for
> details.

## Usage

### Quick start — a complete walkthrough

A first run, end to end:

1. **Launch** the UI: `rcss`.
2. **Rclone Account** — pick your remote (or `＋ Configure a new account…` to run
   `rclone config`). The first time you select a remote, RCSS seeds sensible
   defaults and makes it the active account (the sidebar shows `● active`).
3. **Backup source** — browse with `→`/`l` to drill in and `←`/`h` to go up,
   then press `enter` on the folder whose **sub-folders are your projects**.
4. *(optional)* **Settings** — tune local/remote retention, “delete after
   upload”, dotfile skipping, and ignored folders; saved with a confirmation.
5. **Back Up Now** — the remote destination is pre-filled from Settings (blank =
   account root) and **editable** to confirm before you start; press `enter` to
   begin the one-way upload. Progress streams live and it finishes with
   `✓ Backup … in <duration>`. (Edits here apply to this run only.)
6. **Schedule** — answer *Schedule daily upload?* (+ time `HH:MM`) and *Schedule
   weekly clean (Sundays)?* (+ time). RCSS installs the matching crontab / Task
   Scheduler jobs for this account — no root/admin needed.
7. **Restore** — pick a project (📁) → pick a file (or a loose file 📄) → confirm
   the **local destination** (pre-filled from Settings, editable) → `enter`
   restores it with progress.
8. **Clean** — press `enter` for a dry-run **preview**, then `x` to execute the
   real deletion of old **cloud** backups. `f` toggles **Force** (bypasses the
   safety lock) and is double-confirmed.

The scheduled jobs simply run the headless commands described under
[Headless (for cron)](#headless-for-cron) below.

### Terminal UI

```bash
rcss
```

Opens the full UI. From the main menu:

| Screen | What it does |
| --- | --- |
| **Rclone Account** | Manage accounts (one per rclone remote): switch the active one, add via `rclone config`, or forget one |
| **Backup source** | Choose the local root folder whose sub-folders are your projects |
| **Restore** | Browse remote projects → files and restore one, with live progress |
| **Back Up Now** | Copy all projects to the cloud now (one-way upload), streaming rclone progress |
| **Clean** | Remove old **cloud** backups: explains the criteria, previews with a dry-run, then deletes; optional Force (double-confirmed) bypasses the safety lock |
| **Settings** | Edit retention and behavior; saved to `config.toml` with a visible confirmation |
| **Schedule** | Install daily-backup / weekly-clean jobs into your OS scheduler (crontab / Task Scheduler) |
| **Logs** | Scroll the sync log with ERROR/WARN highlighting |
| **About** | Version, rclone/scheduler status, and config/log locations |

The UI requires a terminal of at least **80×12**; anything smaller renders a
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

From the **Rclone Account** screen: `enter` switches the active account (creating its
settings with defaults the first time a remote is chosen), `d` forgets an
account's RCSS settings (the rclone remote itself is left intact), and
**Configure a new account…** opens `rclone config` to add a remote. Settings,
Schedule, Restore, etc. all act on the active account.

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
| `source_root` | — | local folder whose sub-folders are projects |
| `remote_destination` | `` (blank) | destination folder on the remote; **blank = the account root** |
| `restore_destination` | `` (blank) | local folder restores are written to; **blank = the backup source** |
| `retention_days` | `1` | delete **local** files older than this after a successful upload |
| `remote_retention_days` | `15` | delete **cloud** files older than this on clean |
| `remote_cleanup_safety_days` | `2` | clean is blocked unless a backup newer than this exists |
| `delete_after_upload` | `false` | delete all local files right after a successful upload |
| `skip_dotfiles` | `false` | exclude hidden files/folders from uploads |
| `ignored_folders` | `scripts config bin logs lost+found` | sub-folders never treated as projects |
| `log_file` | (config dir)/`backup-<account>.log` | append-only run log (per account) |

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

Released under the [MIT License](LICENSE).
