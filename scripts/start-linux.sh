#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dlna-audio-recorder"
PID_FILE="$STATE_DIR/dlna-recorder.pid"
LOCK_DIR="$STATE_DIR/start.lock"
LOG_FILE="$STATE_DIR/dlna-recorder.log"

mkdir -p "$STATE_DIR"

cleanup_lock() { rmdir "$LOCK_DIR" 2>/dev/null || true; }
acquire_lock() {
  local i
  for i in {1..50}; do
    if mkdir "$LOCK_DIR" 2>/dev/null; then
      trap cleanup_lock EXIT INT TERM
      return 0
    fi
    sleep 0.1
  done
  echo "Another start operation is still in progress." >&2
  exit 1
}

resolve_binary() {
  if [[ -n "${DLNA_RECORDER_BIN:-}" ]]; then
    printf '%s\n' "$DLNA_RECORDER_BIN"
  elif [[ -x "$ROOT_DIR/bin/dlna-recorder" ]]; then
    printf '%s\n' "$ROOT_DIR/bin/dlna-recorder"
  elif [[ -x "$ROOT_DIR/dlna-recorder" ]]; then
    printf '%s\n' "$ROOT_DIR/dlna-recorder"
  else
    echo "dlna-recorder binary not found. Run 'make build' or set DLNA_RECORDER_BIN." >&2
    exit 1
  fi
}

is_recorder_pid() {
  local pid="$1" cmd
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  [[ "$cmd" == *"dlna-recorder"* ]]
}

acquire_lock

if [[ -f "$PID_FILE" ]]; then
  pid="$(tr -d '[:space:]' < "$PID_FILE")"
  if is_recorder_pid "$pid"; then
    echo "dlna-recorder is already running (PID $pid)."
    exit 0
  fi
  rm -f "$PID_FILE"
fi

BIN="$(resolve_binary)"
nohup "$BIN" "$@" >>"$LOG_FILE" 2>&1 &
pid=$!
printf '%s\n' "$pid" > "$PID_FILE"

sleep 0.2
if ! is_recorder_pid "$pid"; then
  rm -f "$PID_FILE"
  echo "dlna-recorder failed to start. See $LOG_FILE" >&2
  exit 1
fi

echo "dlna-recorder started (PID $pid)."
echo "Log: $LOG_FILE"
