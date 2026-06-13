// netif.h — Wi-Fi station bring-up from NVS credentials (2.4 GHz). Auto-reconnects.
#pragma once
#include <stdbool.h>

bool netif_wifi_start(const char *ssid, const char *pass);
bool netif_wait_ip(int timeout_ms);   // timeout_ms < 0 = wait forever
bool netif_has_ip(void);
