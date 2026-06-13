#include "mdns_adv.h"
#include "config.h"
#include "wire.h"

#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "mdns.h"

static const char *TAG = "mdns";

bool mdns_adv_start(void) {
    ens_config_t *cfg = config_get();
    if (mdns_init() != ESP_OK) { ESP_LOGE(TAG, "mdns_init failed"); return false; }

    char idhex[33];
    config_node_id_hex(idhex);

    // Hostname: ensemble-<first 4 hex of id> → unique on the LAN.
    char host[32];
    snprintf(host, sizeof host, "ensemble-%c%c%c%c", idhex[0], idhex[1], idhex[2], idhex[3]);
    mdns_hostname_set(host);

    // The mDNS INSTANCE NAME must be the node id (32 hex), exactly like the Go
    // nodes (discovery keys peers by a stable, unique instance). Using a friendly
    // name here breaks re-discovery: a master caches the instance under one id and
    // won't re-register it when the id changes. The friendly label rides the
    // `name` TXT key instead (parse.go → Peer.Name), matching the reference nodes.
    mdns_instance_name_set(idhex);

    char ctrl[8], rate[8];
    snprintf(ctrl, sizeof ctrl, "%u", (unsigned)cfg->control_port);
    snprintf(rate, sizeof rate, "%u", (unsigned)WIRE_SAMPLE_RATE);

    // opus only on the MCU (the Wi-Fi default; PCM frames don't fit the jitter
    // slots). queue=0 in v1 (no clean APLL actuation → accept drift, §9). hwvol=0
    // (software gain). input=0.
    mdns_txt_item_t txt[] = {
        { "id",      idhex },
        { "role",    "playback" },
        { "control", ctrl },
        { "name",    cfg->name },
        { "codecs",  "opus" },
        { "rate",    rate },
        { "hwvol",   "0" },
        { "delayms", "0" },
        { "queue",   cfg->has_apll ? "0" : "0" },   // v1: 0 until the servo actuates
        { "input",   "0" },
    };
    esp_err_t err = mdns_service_add(idhex, "_ensemble", "_tcp",
                                     cfg->control_port, txt, sizeof txt / sizeof txt[0]);
    if (err != ESP_OK) { ESP_LOGE(TAG, "service_add failed: %s", esp_err_to_name(err)); return false; }
    ESP_LOGI(TAG, "advertising _ensemble._tcp host=%s.local control=%u id=%s name=%s",
             host, (unsigned)cfg->control_port, idhex, cfg->name);
    return true;
}
