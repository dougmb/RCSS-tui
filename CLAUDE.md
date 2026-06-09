# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

RCSS (Rclone Cloud Simple Scripts) is a backup management toolkit. Three Bash scripts wrap `rclone` to upload, clean, and restore per-project backups on a cloud remote (typically Google Drive). The `tui/` directory is a planned Go/Bubbletea terminal UI that will front these scripts — currently scaffolding only (empty `models/` and `runner/` dirs).

All shared configuration lives in `backup.env`, sourced by every script. `backup.env` contains real secrets/paths and is **not** committed; `sync.log` is the append-only run log written by upload and clean.

## Running

```bash
chmod +x *.sh                 # one-time
./uploadBackup.sh -p          # upload all projects with progress bar
./uploadBackup.sh -v -D       # verbose, delete local files after upload
./uploadBackup.sh -a <file>   # single-file mode (skips project loop)
./cleanRemoteBackups.sh -d -v # dry-run remote cleanup (preview deletions)
./cleanRemoteBackups.sh       # actually delete old remote backups
./restoreBackup.sh -p         # interactive restore (prompts for project + file)
```

There is no build, test, or lint setup yet. When linting Bash, use `shellcheck` — the scripts already follow its conventions (`# shellcheck source=/dev/null` directives, quoted expansions, array-based rclone flags).

## Architecture & conventions

**Config resolution order** (in `uploadBackup.sh`): CLI flag overrides (`-o`, `-r`, `-d`, `-i`) > `backup.env` values > hardcoded defaults. `BACKUP_ROOT`, `RCLONE_REMOTE`, and `RETENTION_DAYS` are required and validated via `: "${VAR:?...}"`; the script aborts if any is unset. Other scripts validate their own required subset.

**Remote layout**: backups live at `${RCLONE_REMOTE}/${DRIVE_DESTINATION}/<PROJECT_NAME>/`. Upload iterates each subdirectory of `BACKUP_ROOT` as a "project", skipping dotfolders and anything in `IGNORED_FOLDERS` (default: `scripts config bin logs lost+found`). Restore mirrors this: it lists projects via `rclone lsf --dirs-only`, then files within the chosen project.

**Two retention concepts — keep them distinct**: `RETENTION_DAYS` controls deletion of *local* files after upload (`find -mtime`); `REMOTE_RETENTION_DAYS` controls deletion of *cloud* files (`rclone delete --min-age`). The `-D`/`DELETE_AFTER_UPLOAD` flag bypasses `RETENTION_DAYS` and deletes all local files immediately after a successful upload.

**Safety invariants — preserve these when editing:**
- Local cleanup in `uploadBackup.sh` runs *only* inside the `if rclone copy ... ; then` success branch. A failed upload must never delete local files.
- `cleanRemoteBackups.sh` refuses to delete unless a recent backup (within `REMOTE_CLEANUP_SAFETY_DAYS`) exists on the remote, guarding against wiping history when the upload cron has silently stopped. The `-f` flag bypasses this lock — treat it as dangerous.
- All scripts use `set -euo pipefail`. `uploadBackup.sh` installs an EXIT trap (`cleanup_on_error`) that logs unexpected failures; it is explicitly removed (`trap - EXIT`) before clean exits.

**Logging**: `upload` and `clean` share an identical `_log`/`log_info`/`log_warn`/`log_error`/`log_verbose` helper block that writes both to stdout and `LOG_FILE` (default `$SCRIPT_DIR/sync.log`). `restore` instead logs only to the terminal with ANSI colors (it's interactive). Upload appends a fixed-width "SYNC SUMMARY" block to the log at the end of every run. If editing log format, keep these consistent across scripts.

**rclone invocation**: flags are built into a Bash array (`RCLONE_FLAGS`) and conditionally appended, then expanded as `"${RCLONE_FLAGS[@]}"`. Verbosity maps to rclone log level via `rclone_log_level` (`DEBUG` when `-v`, else `NOTICE`).

## Planned TUI (see plan.md)

Go + Bubbletea/Lipgloss/Bubbles, living in `tui/`. The intended boundary: the TUI shells out to the existing scripts via `exec.Command` for upload and clean (capturing piped stdout/stderr live), but **reimplements restore in Go** — calling `rclone lsf`/`rclone copy` directly rather than driving `restoreBackup.sh`'s interactive `select_from_list` prompts. Scripts stay in the repo root and remain the source of truth for backup logic; `backup.env` stays shared. Read `plan.md` before starting TUI work.
