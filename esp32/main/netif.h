// netif.h — Wi-Fi station bring-up from NVS credentials (2.4 GHz). Auto-reconnects.
#pragma once
#include <stdbool.h>

bool netif_wifi_start(const char *ssid, const char *pass);
bool netif_wait_ip(int timeout_ms);   // timeout_ms < 0 = wait forever
bool netif_has_ip(void);

// Stop auto-reconnecting the station: after a failed connect we hand the radio to
// the captive portal (provision.c), and the STA_DISCONNECTED auto-reconnect would
// otherwise keep re-issuing esp_wifi_connect() and fight the portal's AP+STA scan.
void netif_wifi_suppress_reconnect(void);
