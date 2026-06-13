// app_main.c — ensemble playback node entry point. Brings up config + the audio
// pipeline + local control (encoder, USB console) immediately so an unprovisioned
// board is configurable over USB; once Wi-Fi has an IP it starts the data plane
// (net_audio), the v2 control plane (control), and the mDNS advert, then idles
// until a master ATTACHes it (or self-attaches in fixed-master mode).
#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "esp_system.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include "config.h"
#include "clock.h"
#include "player.h"
#include "encoder.h"
#include "console.h"
#include "netif.h"
#include "net_audio.h"
#include "control.h"
#include "mdns_adv.h"
#include "wire.h"

static const char *TAG = "main";

// Parse "a.b.c.d" → host-order uint32. Returns 0 on failure.
static uint32_t parse_ipv4(const char *s) {
    unsigned a, b, c, d;
    if (sscanf(s, "%u.%u.%u.%u", &a, &b, &c, &d) != 4) return 0;
    if (a > 255 || b > 255 || c > 255 || d > 255) return 0;
    return (a << 24) | (b << 16) | (c << 8) | d;
}

// Brought up once the netif has an IP: data plane + control plane + mDNS.
static void services_task(void *arg) {
    (void)arg;
    netif_wait_ip(-1);   // block until connected
    net_audio_init();
    ens_config_t *cfg = config_get();
    control_init(cfg->control_port);
    mdns_adv_start();

    if (cfg->disc_mode == 1) {
        // Fixed-master bench fallback (PLAYER §11): self-attach, no master driving.
        uint32_t mip = parse_ipv4(cfg->master_ip);
        if (mip) {
            ESP_LOGI(TAG, "fixed-master mode: attaching %s", cfg->master_ip);
            net_audio_attach(mip, cfg->source_port, mip, cfg->stream_port,
                             cfg->codec_pref == 1 ? WIRE_CODEC_PCM : WIRE_CODEC_OPUS,
                             WIRE_TRANSPORT_UDP, cfg->buffer_ms);
        } else {
            ESP_LOGW(TAG, "disc_mode=1 but master_ip invalid");
        }
    } else {
        ESP_LOGI(TAG, "idle — waiting for a master to ATTACH (mDNS-discovered)");
    }
    vTaskDelete(NULL);
}

void app_main(void) {
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        nvs_flash_erase();
        nvs_flash_init();
    }

    ens_config_t *cfg = config_load();
    char idhex[33]; config_node_id_hex(idhex);
    ESP_LOGI(TAG, "ensemble playback node — id=%s name=%s", idhex, cfg->name);

    clock_init();

    // Bring Wi-Fi up FIRST: its driver allocates large contiguous RX/TX buffers
    // at init, so it must get first dibs on the heap — before the jitter buffer
    // + opus decoder carve it up. (Doing player_init first OOMs the Wi-Fi init on
    // the 320 KB S2: "malloc buffer fail / Expected to init N rx buffer".)
    bool have_wifi = cfg->wifi_ssid[0] != '\0';
    if (have_wifi) netif_wifi_start(cfg->wifi_ssid, cfg->wifi_pass);

    if (!player_init(cfg)) { ESP_LOGE(TAG, "player init failed"); }
    encoder_init(cfg->enc_a, cfg->enc_b, cfg->enc_sw);
    console_init();   // always available for USB provisioning

    if (have_wifi) {
        xTaskCreate(services_task, "services", 3072, NULL, 5, NULL);
    } else {
        ESP_LOGW(TAG, "no Wi-Fi configured — provision over USB "
                      "(send {\"cmd\":\"set\",\"cfg\":{\"wifi_ssid\":\"...\",\"wifi_pass\":\"...\"}} then reboot)");
    }
}
