#!/usr/bin/env bash
# ensemble installer — "for the lazy people".
#
#   curl -fsSL https://ensemble.rand0m.me/get.sh | sudo bash
#
# Detects the OS/arch, downloads the matching ensemble build, and then asks
# (interactively, even when piped from curl — it reads /dev/tty) whether to install
# Spotify support (go-librespot) and a boot-time systemd service. Binaries land in
# /usr/local/lib/ensemble (symlinked into /usr/local/bin).
#
# Override the download host with ENSEMBLE_BASE=... (e.g. for a local mirror).
set -euo pipefail

BASE="${ENSEMBLE_BASE:-https://ensemble.rand0m.me}"
LIBDIR="/usr/local/lib/ensemble"
BINDIR="/usr/local/bin"
DATADIR="/var/lib/ensemble"
UNIT="/etc/systemd/system/ensemble.service"
GLR_REPO="devgianlu/go-librespot"

c_info=$'\033[36m'; c_ok=$'\033[32m'; c_err=$'\033[31m'; c_off=$'\033[0m'
say()  { printf '%s==>%s %s\n' "$c_info" "$c_off" "$*"; }
ok()   { printf '%s ✓%s %s\n' "$c_ok" "$c_off" "$*"; }
err()  { printf '%serror:%s %s\n' "$c_err" "$c_off" "$*" >&2; exit 1; }

# ask reads from the TERMINAL, not stdin — so prompts work under `curl | bash`,
# where stdin is the script itself. Non-interactive (no tty) returns the default.
# $2 = default ("y" makes the prompt [Y/n] and a bare Enter mean yes).
ask() {
  local q="$1" def="${2:-n}" a="" hint='[y/N]'
  [ "$def" = y ] && hint='[Y/n]'
  printf '%s %s ' "$q" "$hint" >/dev/tty 2>/dev/null || { [ "$def" = y ]; return; }
  read -r a </dev/tty || a=""
  [ -z "$a" ] && a="$def"
  case "$a" in [yY] | [yY][eE][sS]) return 0 ;; *) return 1 ;; esac
}

# ask_value prompts for a free-text value, re-asking until non-empty. A bare Enter
# accepts $2 (the default) when one is given. Non-interactive falls back to $2.
ask_value() {
  local q="$1" def="${2:-}" a="" prompt="$1"
  [ -n "$def" ] && prompt="$q [$def]"
  while :; do
    printf '%s: ' "$prompt" >/dev/tty 2>/dev/null || { printf '%s' "$def"; return; }
    read -r a </dev/tty || a=""
    [ -z "$a" ] && a="$def"
    [ -n "$a" ] && { printf '%s' "$a"; return; }
  done
}

# current_node_name extracts the existing node name from node.json (if any), so a
# re-install defaults to the name the node already advertises. Best-effort grep —
# no jq dependency.
current_node_name() {
  local f="$DATADIR/node.json"
  [ -f "$f" ] || return 0
  sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$f" | head -1
}

fetch() { # fetch URL OUTFILE  (follows redirects; curl or wget)
  if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
  else err "need curl or wget"; fi
}

[ "$(id -u)" = 0 ] || err "run as root:  curl -fsSL $BASE/get.sh | sudo bash"
[ "$(uname -s)" = Linux ] || err "ensemble ships Linux binaries only (got $(uname -s))"
command -v tar >/dev/null 2>&1 || err "need tar"

# --- detect arch -------------------------------------------------------------
case "$(uname -m)" in
  x86_64 | amd64)        ARCH=amd64; GLARCH=x86_64 ;;
  aarch64 | arm64)       ARCH=arm64; GLARCH=arm64 ;;
  armv7l | armv6l | arm) ARCH=armv6; GLARCH=armv6 ;; # the armv6 build also runs on armv7
  *) err "unsupported architecture: $(uname -m)" ;;
esac
say "Detected linux/$ARCH"

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# --- ensemble ----------------------------------------------------------------
url="$BASE/assets/downloads/ensemble-linux-$ARCH.tar.gz"
say "Downloading ensemble — $url"
fetch "$url" "$tmp/ensemble.tar.gz" || err "download failed: $url"
tar -xzf "$tmp/ensemble.tar.gz" -C "$tmp"
[ -f "$tmp/ensemble" ] || err "archive did not contain an 'ensemble' binary"
install -d "$LIBDIR" "$DATADIR"
install -m 0755 "$tmp/ensemble" "$LIBDIR/ensemble"
ln -sf "$LIBDIR/ensemble" "$BINDIR/ensemble"
ok "installed $LIBDIR/ensemble  (→ $BINDIR/ensemble)"

# --- spotify (optional) ------------------------------------------------------
if ask "Install Spotify Connect support (go-librespot)?"; then
  glurl="https://github.com/$GLR_REPO/releases/latest/download/go-librespot_linux_${GLARCH}.tar.gz"
  say "Downloading go-librespot — $glurl"
  if fetch "$glurl" "$tmp/glr.tar.gz"; then
    tar -xzf "$tmp/glr.tar.gz" -C "$tmp"
    glr="$(find "$tmp" -type f -name go-librespot | head -1 || true)"
    if [ -n "$glr" ]; then
      install -m 0755 "$glr" "$LIBDIR/go-librespot"
      ln -sf "$LIBDIR/go-librespot" "$BINDIR/go-librespot"
      ok "installed $LIBDIR/go-librespot  (→ $BINDIR/go-librespot)"
    else
      err "go-librespot binary not found in the archive — install it manually"
    fi
  else
    err "go-librespot download failed — install it manually, then re-run"
  fi
else
  say "Skipping Spotify support."
fi

# --- systemd service (optional) ----------------------------------------------
if command -v systemctl >/dev/null 2>&1 && ask "Start ensemble at boot (systemd service)?"; then
  # Stop a previous instance first so the new unit + binary take over cleanly.
  if systemctl is-active --quiet ensemble.service 2>/dev/null; then
    say "Stopping the running ensemble.service…"
    systemctl stop ensemble.service
  fi

  # Role: a node that serves the web UI is a "master" (it gossips + owns cluster
  # state + serves the SPA) and also plays; a node that only plays is "playback"
  # (receive-only, driven by a master over the control plane).
  if ask "Run the web UI on this node (control the whole system from here)?" y; then
    ROLE="master,playback"
  else
    ROLE="playback"
  fi

  # Node name: shown in the UI. Mandatory (re-asks until non-empty); defaults to
  # the name in an existing node.json on re-install.
  NAME="$(ask_value "Node name (shown in the UI)" "$(current_node_name)")"

  say "Role: $ROLE   ·   Name: $NAME"
  cat >"$UNIT" <<UNITEOF
[Unit]
Description=ensemble — synchronized multiroom audio
After=network-online.target sound.target
Wants=network-online.target

[Service]
ExecStart=$LIBDIR/ensemble --data $DATADIR --role $ROLE --name "$NAME"
WorkingDirectory=$LIBDIR
# Under a system service there is no login session: give go-librespot and any
# audio-player subprocess a writable HOME so they can locate their config.
Environment=HOME=$DATADIR
Environment=XDG_CONFIG_HOME=$DATADIR
# Access to ALSA hardware devices for the audio output failover chain.
SupplementaryGroups=audio
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
UNITEOF
  systemctl daemon-reload
  systemctl enable --now ensemble.service   # enable at boot + start now (fresh binary)
  ok "ensemble.service enabled + started   ·   logs:  journalctl -u ensemble -f"
else
  say "No service installed. Run it yourself:  ensemble"
fi

printf '\n%s ✓%s ensemble is ready — open the web UI at  http://<this-host>:8080\n' "$c_ok" "$c_off"
