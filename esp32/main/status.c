#include "status.h"
#include "wire.h"
#include "config.h"
#include "player.h"
#include "netif.h"

#include <string.h>
#include "esp_system.h"
#include "esp_heap_caps.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

int status_build(uint8_t *out, size_t cap) {
    wire_status_t s;
    memset(&s, 0, sizeof s);
    memcpy(s.node_id, config_get()->node_id, 16);
    player_fill_status(&s);

    // v2 health telemetry — cheap probes so a weak/overloaded node is visible at
    // the master (RSSI, heap headroom, CPU idle %, last reset reason).
    s.rssi = netif_rssi();
    s.free_heap_kb = (uint16_t)(esp_get_free_heap_size() / 1024);
    s.reset_reason = (uint8_t)esp_reset_reason();
#if defined(CONFIG_FREERTOS_GENERATE_RUN_TIME_STATS) && CONFIG_FREERTOS_GENERATE_RUN_TIME_STATS
    s.cpu_idle_pct = (uint8_t)ulTaskGetIdleRunTimePercent();   // 0 during a flood = pegged
#endif

    return wire_status_encode(out, cap, &s);
}
