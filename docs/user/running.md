# Running ondaire

> **You are here:** [User Guide](README.md) › **Running ondaire**
> How to *start* a node and keep it running. What to put in `--name`, `--role`,
> and friends is in the [Configuration Reference](config-reference.md); which kind
> of node to run where is in the [scenarios](README.md#pick-your-setup).

Ondaire is a single process. Every method below runs the **same** binary (or
image) with the **same** flags — they differ only in how the process is supervised
and how long it lives. Pick by how permanent you need it:

| You want… | Use |
|-----------|-----|
| To try it / watch the logs | [Foreground](#foreground) |
| A quick background process on a box you're SSH'd into | [`nohup &`](#background-with-nohup) |
| A permanent speaker on a Pi / headless Linux (ALSA) | [systemd **system** service](#systemd-system-service) |
| A permanent node on your own desktop (PipeWire/PulseAudio) | [systemd **user** service](#systemd-user-service) |
| A master on a NAS / server / container host | [Docker](#docker) or [Docker Compose](#docker-compose) |

---

## Quick install (the one-liner)

In a hurry on a Linux box? The installer detects your OS/CPU, downloads the
matching build, and asks what you want:

```sh
curl -fsSL https://ondaire.rand0m.me/get.sh | sudo bash
```

It runs as root and:

1. **Detects** your architecture (`amd64` or `arm64`) and downloads that `ondaire`
   build. (32-bit ARM is no longer supported — use Raspberry Pi OS 64-bit.)
2. Installs it to **`/usr/local/lib/ondaire/ondaire`**, symlinked into
   `/usr/local/bin` so `ondaire` is on your `PATH`.
3. Asks **"Install Spotify Connect support?"** — if yes, downloads the latest
   [`go-librespot`](https://github.com/devgianlu/go-librespot) for your arch
   alongside it.
4. Asks **"Start ondaire at boot?"** — if yes, writes an `ondaire.service`
   systemd unit (data in `/var/lib/ondaire`), reloads systemd, and enables +
   starts it. Re-running the script stops the old service first, so it upgrades
   cleanly in place.

The prompts work even through the `curl … | bash` pipe (it reads your terminal
directly); a non-interactive run just answers "no" to both. Prefer to do it by
hand, or not use systemd? The methods below are exactly what the script wraps.

> Override the download host (e.g. a mirror) with `ONDAIRE_BASE=…` before running.

---

## Foreground

The simplest way — run it in your terminal:

```sh
./ondaire
```

It prints a startup banner (bound ports, audio backend, whether go-librespot was
found) and keeps running until you press **Ctrl-C**. Add `-v` for debug logging.
Best for the first run, a quick demo, or debugging — see the
[desktop quick start](scenarios/desktop.md#quick-start-one-machine).

---

## Background with `nohup`

To leave it running after you close the terminal or log out, without setting up a
service:

```sh
nohup ./ondaire --name kitchen > ondaire.log 2>&1 &
```

- `nohup … &` detaches it from the shell; `> ondaire.log 2>&1` captures its output.
- Find it later with `pgrep -af ondaire`; stop it with `kill <pid>` (or
  `pkill -f '/ondaire'`).

> **Caveat:** `nohup` does **not** restart ondaire after a crash or a reboot. For
> anything you want to "just always be there," use a systemd service or Docker with
> a restart policy instead.

---

## systemd (system service)

The right choice for an **always-on speaker** on a Raspberry Pi or any headless
Linux box using **ALSA** directly. It starts on boot, restarts on failure, and
logs to the journal.

Install the binary and a data dir, then drop in a unit:

```sh
sudo install -Dm755 ondaire /opt/ondaire/ondaire
sudo mkdir -p /var/lib/ondaire
```

`/etc/systemd/system/ondaire.service`:

```ini
[Unit]
Description=ondaire multiroom audio node
After=network-online.target sound.target
Wants=network-online.target

[Service]
Type=simple
# A user with access to the sound card. On Raspberry Pi OS, `pi` is in `audio`
# already; otherwise create a dedicated user and add it to the audio group:
#   sudo useradd --system --home /var/lib/ondaire --groups audio ondaire
User=pi
SupplementaryGroups=audio
WorkingDirectory=/opt/ondaire
Environment=ONDAIRE_DATA_DIR=/var/lib/ondaire
# Default name = the machine's hostname; replace with --name kitchen if you like.
ExecStart=/opt/ondaire/ondaire --role playback --name %H
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
```

Enable and manage it:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now ondaire      # start now + on every boot
systemctl status ondaire                 # is it up?
journalctl -u ondaire -f                 # follow its logs / banner
sudo systemctl restart ondaire           # after editing the unit / upgrading
sudo systemctl disable --now ondaire     # stop + don't start on boot
```

Notes:
- Drop `--role playback` if this box should also host a library / be a master
  (see [roles](config-reference.md#3-roles)).
- The unit pins `ONDAIRE_DATA_DIR` so the node's identity lives at a stable path
  regardless of the working directory. To use **Spotify** on a system service, put
  the `go-librespot` binary in `WorkingDirectory` (or on `$PATH`) —
  [details](spotify.md#native-install-go-librespot).

> **Why a *system* service for a Pi but a *user* service for your desktop?** A
> system service runs outside any login session, which is perfect for raw ALSA but
> has **no access to a per-user PipeWire/PulseAudio session**. If your machine
> routes audio through PipeWire/Pulse (most desktops), use the user service below.

---

## systemd (user service)

The right choice on a **desktop/laptop** whose audio goes through **PipeWire or
PulseAudio**: a user service inherits your audio session automatically — no
`audio`-group fiddling, no backend surprises.

`~/.config/systemd/user/ondaire.service`:

```ini
[Unit]
Description=ondaire multiroom audio node (user)
After=default.target

[Service]
Type=simple
# Runs from your home dir; ./data and ./go-librespot resolve there.
WorkingDirectory=%h/ondaire
ExecStart=%h/ondaire/ondaire --name %u-desktop
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
```

Enable and manage it (note: **`--user`**, and no `sudo`):

```sh
mkdir -p ~/.config/systemd/user
# (save the unit above, with your ondaire binary at ~/ondaire/ondaire)
systemctl --user daemon-reload
systemctl --user enable --now ondaire
systemctl --user status ondaire
journalctl --user -u ondaire -f
systemctl --user restart ondaire
```

To keep it running **after you log out / before you log in** (e.g. a headless
desktop that should be a speaker at boot), enable lingering once:

```sh
sudo loginctl enable-linger "$USER"
```

---

## Docker

The official image is **master-only** and bakes in go-librespot. Run it on a NAS,
mini-PC, or server (full rationale in the
[NAS / server guide](scenarios/nas-master.md)):

```sh
docker run -d --name ondaire-master \
  --network host \
  -v /srv/music:/media:ro \
  -v ondaire-data:/data \
  --restart unless-stopped \
  harbor.rand0m.me/public/ondaire:latest --name living-room
```

- `--network host` is **required** — playback nodes find the master over mDNS and
  go-librespot advertises Spotify over zeroconf; neither survives a NAT bridge
  ([why](scenarios/nas-master.md#why-host-networking)).
- `--restart unless-stopped` brings it back after a crash or host reboot.

Manage it:

```sh
docker logs -f ondaire-master
docker restart ondaire-master
docker stop ondaire-master && docker rm ondaire-master
```

---

## Docker Compose

The same thing, declarative. A ready-to-use file ships at
[`docker/docker-compose.yml`](../../docker/docker-compose.yml); a minimal version:

```yaml
services:
  ondaire:
    image: harbor.rand0m.me/public/ondaire:latest
    container_name: ondaire-master
    network_mode: host          # required — see above
    restart: unless-stopped
    environment:
      ONDAIRE_ROLE: master      # this image is master-only
      ONDAIRE_MEDIA_DIR: /media
      ONDAIRE_DATA_DIR: /data
    command: ["--name", "living-room"]
    volumes:
      - /srv/music:/media:ro     # your library — READ-ONLY
      - ondaire-data:/data      # node identity, cluster state, Spotify creds
volumes:
  ondaire-data:
```

```sh
docker compose up -d            # start in the background
docker compose logs -f          # follow logs
docker compose pull && docker compose up -d   # upgrade to a newer image
docker compose down             # stop + remove (named volume persists)
```

---

**See also:** [Configuration Reference](config-reference.md) ·
[Pick your setup](README.md#pick-your-setup) ·
[NAS / server master](scenarios/nas-master.md)
