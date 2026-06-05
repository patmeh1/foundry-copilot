#!/usr/bin/env bash
# Foundry Copilot screenshot driver (v2).
#
# Drives the *isolated* VS Code launched under tmp/profile.
# Uses Swift helper to enumerate windows by PID so we never accidentally
# screenshot the developer's primary VS Code.
#
# Subcommands:
#   launch                          # spawn isolated VS Code with sample-workspace
#   wait <seconds>                  # sleep (helper for sequencing)
#   close                           # kill isolated instance
#   pid                             # print isolated VS Code PID
#   wid                             # print isolated VS Code window id (visible main window)
#   shot <slug>                     # capture isolated window -> docs/images/<slug>.png
#   focus                           # bring isolated VS Code to front
#   key <accel>                     # send keystroke (e.g. cmd+alt+i)
#   type "text"                     # type literal text
#   palette "Foundry Copilot: Ping Sidecar"
#   enter

set -euo pipefail
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROFILE_DATA="$REPO_ROOT/tmp/profile/user-data"
PROFILE_EXTS="$REPO_ROOT/tmp/profile/extensions"
WORKSPACE="/tmp/foundry-screenshots/sample-workspace"
SHOTS_DIR="$REPO_ROOT/docs/images"
FOCUS_SWIFT="$REPO_ROOT/scripts/screenshots/code_focus.swift"

mkdir -p "$SHOTS_DIR"

isolated_pid() {
  pgrep -f "Visual Studio Code.app/Contents/MacOS/Code .*user-data-dir.*$PROFILE_DATA" | head -1
}

isolated_wid() {
  local pid
  pid="$(isolated_pid)"
  [[ -z "$pid" ]] && return 1
  swift "$FOCUS_SWIFT" winid "$pid" 2>/dev/null
}

focus_isolated() {
  local pid
  pid="$(isolated_pid)"
  [[ -z "$pid" ]] && { echo "no isolated VS Code running" >&2; return 1; }
  swift "$FOCUS_SWIFT" activate "$pid" >/dev/null 2>&1 || true
  /usr/bin/osascript -e "tell application id \"com.microsoft.VSCode\" to activate" 2>/dev/null || true
  sleep 0.4
}

send_key() {
  local accel="$1"
  /usr/bin/python3 - "$accel" <<'PY'
import sys, subprocess
spec = sys.argv[1].lower()
parts = spec.split('+')
key = parts[-1]
mods = parts[:-1]
mod_map = {
  'cmd': 'command down', 'command': 'command down',
  'ctrl': 'control down', 'control': 'control down',
  'alt': 'option down', 'option': 'option down', 'opt': 'option down',
  'shift': 'shift down',
}
mod_list = ', '.join(mod_map[m] for m in mods)
key_codes = {
  'return': 36, 'enter': 36, 'tab': 48, 'space': 49, 'esc': 53, 'escape': 53,
  'left': 123, 'right': 124, 'down': 125, 'up': 126, 'delete': 51, 'backspace': 51,
  'f1': 122, 'f5': 96,
}
if key in key_codes:
  action = f'key code {key_codes[key]}'
else:
  action = f'keystroke "{key}"'
script = f'tell application "System Events" to {action}'
if mod_list:
  script = f'tell application "System Events" to {action} using {{{mod_list}}}'
subprocess.run(['osascript', '-e', script], check=True)
PY
}

case "${1:-}" in
  launch)
    pkill -f "user-data-dir.*tmp/profile/user-data" 2>/dev/null || true
    sleep 2
    open -na "Visual Studio Code" --args \
      --user-data-dir "$PROFILE_DATA" \
      --extensions-dir "$PROFILE_EXTS" \
      --new-window \
      --disable-workspace-trust \
      --goto "$WORKSPACE/app.py:14" \
      "$WORKSPACE"
    for _ in {1..15}; do
      sleep 1
      if isolated_pid >/dev/null && isolated_wid >/dev/null 2>&1; then
        echo "launched: pid=$(isolated_pid) wid=$(isolated_wid)"
        exit 0
      fi
    done
    echo "launch timed out" >&2
    exit 1
    ;;
  close)
    pkill -f "user-data-dir.*tmp/profile/user-data" 2>/dev/null || true
    echo "closed"
    ;;
  pid) isolated_pid ;;
  wid) isolated_wid ;;
  shot)
    slug="${2:?slug required}"
    out="$SHOTS_DIR/${slug}.png"
    wid="$(isolated_wid)"
    [[ -z "$wid" ]] && { echo "no isolated window" >&2; exit 1; }
    focus_isolated
    sleep 0.6
    /usr/sbin/screencapture -x -t png -l "$wid" "$out"
    echo "captured -> $out ($(/usr/bin/stat -f%z "$out") bytes)"
    ;;
  focus) focus_isolated ;;
  key)
    accel="${2:?accel required}"
    focus_isolated
    send_key "$accel"
    sleep 0.3
    ;;
  type)
    text="${2:?text required}"
    focus_isolated
    /usr/bin/osascript -e "tell application \"System Events\" to keystroke \"$text\""
    sleep 0.3
    ;;
  palette)
    cmd="${2:?command required}"
    focus_isolated
    send_key "cmd+shift+p"
    sleep 0.5
    /usr/bin/osascript -e "tell application \"System Events\" to keystroke \"$cmd\""
    sleep 0.5
    ;;
  enter)
    focus_isolated
    send_key "return"
    ;;
  wait) sleep "${2:-1}" ;;
  bounds)
    pid="$(isolated_pid)"
    [[ -z "$pid" ]] && { echo "no isolated VS Code running" >&2; exit 1; }
    swift "$FOCUS_SWIFT" bounds "$pid"
    ;;
  click)
    # click <fx> <fy>  — click at fraction (fx,fy) within window
    fx="${2:?fx required}"
    fy="${3:?fy required}"
    pid="$(isolated_pid)"
    [[ -z "$pid" ]] && { echo "no isolated VS Code running" >&2; exit 1; }
    read -r wx wy ww wh <<< "$(swift "$FOCUS_SWIFT" bounds "$pid")"
    px=$(/usr/bin/python3 -c "print(int($wx + $fx * $ww))")
    py=$(/usr/bin/python3 -c "print(int($wy + $fy * $wh))")
    focus_isolated
    /opt/homebrew/bin/cliclick "c:${px},${py}"
    sleep 0.3
    ;;
  click-abs)
    # click-abs <px> <py>  — click at absolute screen point (points)
    px="${2:?px}"
    py="${3:?py}"
    focus_isolated
    /opt/homebrew/bin/cliclick "c:${px},${py}"
    sleep 0.3
    ;;
  *)
    echo "Usage: $0 {launch|close|pid|wid|shot <slug>|focus|key <accel>|type <text>|palette <cmd>|enter|wait <s>}" >&2
    exit 2
    ;;
esac
