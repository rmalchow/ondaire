// provision.h — Tasmota-style Wi-Fi captive portal. Brought up at boot when the
// node is unprovisioned OR its stored credentials fail to get an IP: an open AP
// (`ondaire-<hex4>`) + a captive portal that scans nearby networks and writes new
// creds + a speaker name to NVS. The portal lives CONFIG_ONDAIRE_PORTAL_TIMEOUT_MS
// then tears itself down and the node goes inert until power-cycled (the USB console
// stays available the whole time). See docs/developer/esp32.md §6.5.
#pragma once
#include <stdbool.h>

// Start the portal. `wifi_started` = true when netif_wifi_start() already inited and
// started the STA (the connect-failed fallback) — we just add the AP and switch to
// AP+STA; false for a fully unprovisioned boot, where we init Wi-Fi from scratch.
// AP+STA is used (not pure AP) so the portal page can scan for networks.
void provision_start(bool wifi_started);
