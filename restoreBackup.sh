#!/usr/bin/env bash
# Restore Backups from Google Drive via rclone
# Usage: ./restoreBackup.sh [-p] [-v] [-n] [-o <output_path>]

set -euo pipefail

# ─────────────────────────────────────────────
# Arguments
# ─────────────────────────────────────────────

SHOW_PROGRESS=0
VERBOSE=0
DRY_RUN=0
OUTPUT_PATH_OVERRIDE=""
while getopts ":pvno:" opt; do
    case $opt in
        p) SHOW_PROGRESS=1 ;;
        v) VERBOSE=1 ;;
        n) DRY_RUN=1 ;;
        o) OUTPUT_PATH_OVERRIDE="$OPTARG" ;;
        *) echo "Usage: $0 [-p] [-v] [-n] [-o <output_path>]"; exit 1 ;;
    esac
done

# ─────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/backup.env"

if [ ! -f "$ENV_FILE" ]; then
    echo "[ERROR] Configuration file $ENV_FILE not found." >&2
    exit 1
fi

source "$ENV_FILE"

# Required variables validation
: "${BACKUP_ROOT:?Error: BACKUP_ROOT not defined in backup.env}"
: "${RCLONE_REMOTE:?Error: RCLONE_REMOTE not defined in backup.env}"
DRIVE_DESTINATION="${DRIVE_DESTINATION:-Backups}"

# ─────────────────────────────────────────────
# Initial validations
# ─────────────────────────────────────────────

if ! command -v rclone &>/dev/null; then
    echo "[ERROR] rclone not found. Please install it before continuing." >&2
    exit 1
fi

# ─────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────

log_info()    { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] [\e[34mINFO\e[0m] $*"; }
log_warn()    { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] [\e[33mWARN\e[0m] $*" >&2; }
log_error()   { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] [\e[31mERROR\e[0m] $*" >&2; }

# ─────────────────────────────────────────────
# Selection Interface
# ─────────────────────────────────────────────

select_from_list() {
    local title="$1"; shift
    local options=("$@")

    # Print menu to stderr so it appears on screen
    # and is not captured by the $(...) variable
    echo -e "\n\e[1m=== $title ===\e[0m" >&2
    for i in "${!options[@]}"; do
        echo "  [$((i+1))] ${options[$i]}" >&2
    done
    echo "" >&2

    while true; do
        read -p "Select an option (1-${#options[@]}): " choice >&2
        if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "${#options[@]}" ]; then
            # Only the final value goes to stdout to be captured
            echo "${options[$((choice-1))]}"
            return 0
        fi
        echo "Invalid option." >&2
    done
}

# ─────────────────────────────────────────────
# Execution
# ─────────────────────────────────────────────

# 1. Fetch Projects from Cloud
log_info "Fetching projects from Drive..."
# Robust capture handling spaces
mapfile -t PROJECTS < <(rclone lsf "${RCLONE_REMOTE}/${DRIVE_DESTINATION}/" --dirs-only)

if [ ${#PROJECTS[@]} -eq 0 ]; then
    log_error "No projects found at ${RCLONE_REMOTE}/${DRIVE_DESTINATION}/"
    exit 1
fi

SELECTED_PROJECT=$(select_from_list "SELECT PROJECT" "${PROJECTS[@]}")
SELECTED_PROJECT=${SELECTED_PROJECT%/} # Remove trailing slash

# 2. Fetch Project Files
log_info "Fetching files for '$SELECTED_PROJECT'..."
mapfile -t FILES < <(rclone lsf "${RCLONE_REMOTE}/${DRIVE_DESTINATION}/${SELECTED_PROJECT}/" --files-only | sort -r)

if [ ${#FILES[@]} -eq 0 ]; then
    log_error "No files found for this project."
    exit 1
fi

SELECTED_FILE=$(select_from_list "SELECT FILE TO DOWNLOAD" "${FILES[@]}")

# 3. Download
if [ -n "$OUTPUT_PATH_OVERRIDE" ]; then
    LOCAL_PATH="$OUTPUT_PATH_OVERRIDE"
else
    LOCAL_PATH="${BACKUP_ROOT}/${SELECTED_PROJECT}"
fi
mkdir -p "$LOCAL_PATH"

log_info "Downloading $SELECTED_FILE to $LOCAL_PATH..."
RCLONE_FLAGS=("--ignore-times")
[ "$SHOW_PROGRESS" = "1" ] && RCLONE_FLAGS+=("-P")
[ "$VERBOSE" = "1" ] && RCLONE_FLAGS+=("-v")
[ "$DRY_RUN" = "1" ] && RCLONE_FLAGS+=("--dry-run")
rclone copy "${RCLONE_REMOTE}/${DRIVE_DESTINATION}/${SELECTED_PROJECT}/${SELECTED_FILE}" "$LOCAL_PATH/" "${RCLONE_FLAGS[@]}"

log_info "Procedure completed."
