#!/usr/bin/env bash
# Backup Synchronization to Google Drive via rclone
# Usage: ./uploadBackup.sh [-v] [-p] [-s] [-D] [-o <origin>] [-r <rclone_remote>] [-d <drive_destination>] [-a <file>] [-i <ignored_folders>]
# This script iterates through /opt/backups/<PROJECT> and uploads to Drive.

set -euo pipefail

# ─────────────────────────────────────────────
# Arguments
# ─────────────────────────────────────────────

VERBOSE=0
SHOW_PROGRESS=0
BACKUP_ROOT_OVERRIDE=""
RCLONE_REMOTE_OVERRIDE=""
DRIVE_DESTINATION_OVERRIDE=""
SINGLE_FILE=""
IGNORED_FOLDERS_OVERRIDE=""
SKIP_DOTFILES_FLAG=0
DELETE_AFTER_UPLOAD_FLAG=0
while getopts ":vpsDo:r:d:a:i:" opt; do
    case $opt in
        v) VERBOSE=1 ;;
        p) SHOW_PROGRESS=1 ;;
        s) SKIP_DOTFILES_FLAG=1 ;;
        D) DELETE_AFTER_UPLOAD_FLAG=1 ;;
        o) BACKUP_ROOT_OVERRIDE="$OPTARG" ;;
        r) RCLONE_REMOTE_OVERRIDE="$OPTARG" ;;
        d) DRIVE_DESTINATION_OVERRIDE="$OPTARG" ;;
        a) SINGLE_FILE="$OPTARG" ;;
        i) IGNORED_FOLDERS_OVERRIDE="$OPTARG" ;;
        *) echo "Usage: $0 [-v] [-p] [-s] [-D] [-o <origin>] [-r <rclone_remote>] [-d <drive_destination>] [-a <file>] [-i <ignored_folders>]"; exit 1 ;;
    esac
done

# ─────────────────────────────────────────────
# Configuration (from backup.env)
# ─────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/backup.env"

if [ ! -f "$ENV_FILE" ]; then
    echo "[ERROR] Configuration file $ENV_FILE not found." >&2
    exit 1
fi

# Load configuration
# shellcheck source=/dev/null
source "$ENV_FILE"

# Required variables validation
: "${BACKUP_ROOT:?Error: BACKUP_ROOT not defined in backup.env}"
: "${RCLONE_REMOTE:?Error: RCLONE_REMOTE not defined in backup.env}"
: "${RETENTION_DAYS:?Error: RETENTION_DAYS not defined in backup.env}"

# Drive destination folder (e.g.: Backups)
DRIVE_DESTINATION="${DRIVE_DESTINATION:-Backups}"

# CLI overrides take priority over backup.env
if [ -n "$BACKUP_ROOT_OVERRIDE" ]; then
    BACKUP_ROOT="$BACKUP_ROOT_OVERRIDE"
fi
if [ -n "$RCLONE_REMOTE_OVERRIDE" ]; then
    RCLONE_REMOTE="$RCLONE_REMOTE_OVERRIDE"
fi
if [ -n "$DRIVE_DESTINATION_OVERRIDE" ]; then
    DRIVE_DESTINATION="$DRIVE_DESTINATION_OVERRIDE"
fi

# Log file defaults to the script directory if not set in .env
LOG_FILE="${LOG_FILE:-$SCRIPT_DIR/sync.log}"

# Folders to ignore (loaded from .env or safe defaults)
IGNORED_FOLDERS="${IGNORED_FOLDERS:-scripts config bin logs lost+found}"

# Append CLI-specified folders to the ignore list
if [ -n "$IGNORED_FOLDERS_OVERRIDE" ]; then
    IGNORED_FOLDERS="$IGNORED_FOLDERS $IGNORED_FOLDERS_OVERRIDE"
fi

# Skip dotfiles/dotfolders (default: false; -s flag sets to true)
SKIP_DOTFILES="${SKIP_DOTFILES:-false}"
[ "$SKIP_DOTFILES_FLAG" = "1" ] && SKIP_DOTFILES="true"

# Delete local files immediately after successful upload (default: false; -D flag sets to true)
DELETE_AFTER_UPLOAD="${DELETE_AFTER_UPLOAD:-false}"
[ "$DELETE_AFTER_UPLOAD_FLAG" = "1" ] && DELETE_AFTER_UPLOAD="true"

UPLOAD_ERRORS=0
TOTAL_DELETED=0
DELETE_ERRORS=0
OVERALL_START=$(date +%s)

# ─────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────

_log() {
    local level="$1"; shift
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [$level] $*"
    echo "$msg"
    echo "$msg" >> "$LOG_FILE"
}

log_info()    { _log "INFO   " "$*"; }
log_warn()    { _log "WARN   " "$*" >&2; }
log_error()   { _log "ERROR  " "$*" >&2; }
log_verbose() {
    if [ "$VERBOSE" = "1" ]; then
        _log "VERBOSE" "$*"
    fi
}

elapsed() {
    local start="$1"
    echo $(( $(date +%s) - start ))
}

rclone_log_level() {
    [ "$VERBOSE" = "1" ] && echo "DEBUG" || echo "NOTICE"
}

# Trap for unexpected errors
cleanup_on_error() {
    local exit_code=$?
    if [ $exit_code -ne 0 ]; then
        log_error "Script terminated unexpectedly with exit code $exit_code"
    fi
}
trap cleanup_on_error EXIT

# ─────────────────────────────────────────────
# Initial validations
# ─────────────────────────────────────────────

if ! command -v rclone &>/dev/null; then
    log_error "rclone not found. Please install it before continuing."
    exit 1
fi

# ─────────────────────────────────────────────
# Single file mode (-a)
# ─────────────────────────────────────────────

if [ -n "$SINGLE_FILE" ]; then
    if [ ! -f "$SINGLE_FILE" ]; then
        log_error "File not found: $SINGLE_FILE"
        exit 1
    fi

    log_info "Uploading single file: $SINGLE_FILE"
    RCLONE_FLAGS=("--log-level" "$(rclone_log_level)" "--retries" "3")
    [ "$SHOW_PROGRESS" = "1" ] && RCLONE_FLAGS+=("-P")
    [ "$SKIP_DOTFILES" = "true" ] && RCLONE_FLAGS+=("--exclude" ".*" "--exclude" ".*/**")

    if rclone copy "$SINGLE_FILE" "${RCLONE_REMOTE}/${DRIVE_DESTINATION}/" "${RCLONE_FLAGS[@]}"; then
        log_info "✓ File uploaded successfully."
    else
        log_error "Failed to upload $SINGLE_FILE"
        trap - EXIT
        exit 1
    fi

    trap - EXIT
    exit 0
fi

# ─────────────────────────────────────────────
# Default mode (projects)
# ─────────────────────────────────────────────

if [ ! -d "$BACKUP_ROOT" ]; then
    log_error "Backup root directory not found: $BACKUP_ROOT"
    exit 1
fi

log_info "Starting backup synchronization..."
log_info "Settings: root=$BACKUP_ROOT | remote=$RCLONE_REMOTE | retention=${RETENTION_DAYS}d | skip_dotfiles=$SKIP_DOTFILES | delete_after_upload=$DELETE_AFTER_UPLOAD"

# ─────────────────────────────────────────────
# Processing per Project
# ─────────────────────────────────────────────

# Loop through each subdirectory in the backup root
# Using nullglob to avoid errors if the directory is empty
shopt -s nullglob
for project_path in "$BACKUP_ROOT"/*; do
    # Skip if not a directory
    [ -d "$project_path" ] || continue

    PROJECT_NAME=$(basename "$project_path")

    # SAFETY: Skip folders that are not backup projects
    # Ignores hidden folders (starting with .) and folders defined in IGNORED_FOLDERS
    if [[ "$PROJECT_NAME" == .* ]] || [[ " ${IGNORED_FOLDERS} " == *" ${PROJECT_NAME} "* ]]; then
        log_verbose "   - Skipping ignored/reserved folder: $PROJECT_NAME"
        continue
    fi

    log_info "→ Processing project: $PROJECT_NAME"
    STEP_START=$(date +%s)

    # 1. Upload to Drive (organized by project folder)
    RCLONE_FLAGS=("--log-level" "$(rclone_log_level)" "--stats-one-line" "--stats" "10s" "--update" "--use-mmap" "--retries" "3")
    [ "$SHOW_PROGRESS" = "1" ] && RCLONE_FLAGS+=("-P")
    [ "$SKIP_DOTFILES" = "true" ] && RCLONE_FLAGS+=("--exclude" ".*" "--exclude" ".*/**")

    if rclone copy "$project_path" "${RCLONE_REMOTE}/${DRIVE_DESTINATION}/${PROJECT_NAME}" \
        "${RCLONE_FLAGS[@]}"; then

        log_info "   ✓ Synchronized successfully."

        # 2. Local cleanup (ONLY after successful upload)
        if [ "$DELETE_AFTER_UPLOAD" = "true" ]; then
            log_verbose "   Deleting all uploaded local files..."
            DELETED_COUNT=0
            while IFS= read -r -d '' file; do
                if rm -- "$file"; then
                    DELETED_COUNT=$((DELETED_COUNT + 1))
                else
                    log_warn "   ⚠ Could not delete: $file"
                    DELETE_ERRORS=$((DELETE_ERRORS + 1))
                fi
            done < <(find "$project_path" -maxdepth 1 -type f -print0)
        else
            log_verbose "   Cleaning local files older than $RETENTION_DAYS days..."
            DELETED_COUNT=0
            while IFS= read -r -d '' file; do
                if rm -- "$file"; then
                    DELETED_COUNT=$((DELETED_COUNT + 1))
                else
                    log_warn "   ⚠ Could not delete: $file"
                    DELETE_ERRORS=$((DELETE_ERRORS + 1))
                fi
            done < <(find "$project_path" -maxdepth 1 -type f -mtime +"$RETENTION_DAYS" -print0)
        fi

        [ "$DELETED_COUNT" -gt 0 ] && log_info "   - Removed $DELETED_COUNT local files."
        TOTAL_DELETED=$((TOTAL_DELETED + DELETED_COUNT))
    else
        log_warn "   ⚠ Sync failed for project $PROJECT_NAME. Local cleanup SKIPPED."
        UPLOAD_ERRORS=$((UPLOAD_ERRORS + 1))
    fi

    log_verbose "   Project time: $(elapsed $STEP_START)s"
done
shopt -u nullglob

# ─────────────────────────────────────────────
# Final Summary
# ─────────────────────────────────────────────

TOTAL_DURATION=$(elapsed $OVERALL_START)
STATUS=$( [ "$UPLOAD_ERRORS" -eq 0 ] && echo "SUCCESS" || echo "PARTIAL" )

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "✅ Synchronization completed in ${TOTAL_DURATION}s"

# Summary block for the log (always at the end)
{
    echo "════════════════════════════════════════════════"
    echo "  SYNC SUMMARY — $(date '+%Y-%m-%d %H:%M:%S')"
    echo "════════════════════════════════════════════════"
    echo "  Status            : $STATUS"
    echo "  Duration          : ${TOTAL_DURATION}s"
    echo "  Cloud Destination : ${RCLONE_REMOTE}/${DRIVE_DESTINATION}/"
    echo "  Projects w/ Errors: $UPLOAD_ERRORS"
    echo "  Files Removed (Local): $TOTAL_DELETED"
    echo "  Delete Errors     : $DELETE_ERRORS"
    echo "  --- Flags ---"
    echo "  skip_dotfiles     : $SKIP_DOTFILES"
    echo "  delete_after_upload: $DELETE_AFTER_UPLOAD"
    echo "  retention_days    : $RETENTION_DAYS"
    echo "════════════════════════════════════════════════"
    echo ""
} >> "$LOG_FILE"

# Remove error trap for clean exit
trap - EXIT

[ "$UPLOAD_ERRORS" -gt 0 ] && exit 1 || exit 0
