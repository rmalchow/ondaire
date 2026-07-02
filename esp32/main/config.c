#include "config.h"
#include "board.h"

#include <string.h>
#include <stdio.h>
#include "esp_log.h"
#include "esp_mac.h"
#include "esp_rom_crc.h"
#include "esp_system.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "soc/gpio_num.h"

static const char *TAG = "config";
static const char *NS = "ensemble";

static ens_config_t g;

// --- small NVS get-with-default helpers ------------------------------------
static uint8_t get_u8(nvs_handle_t h, const char *k, uint8_t def) {
    uint8_t v; return nvs_get_u8(h, k, &v) == ESP_OK ? v : def;
}
static int8_t get_i8(nvs_handle_t h, const char *k, int8_t def) {
    int8_t v; return nvs_get_i8(h, k, &v) == ESP_OK ? v : def;
}
static uint16_t get_u16(nvs_handle_t h, const char *k, uint16_t def) {
    uint16_t v; return nvs_get_u16(h, k, &v) == ESP_OK ? v : def;
}
static void get_str(nvs_handle_t h, const char *k, char *dst, size_t cap, const char *def) {
    size_t len = cap;
    if (nvs_get_str(h, k, dst, &len) != ESP_OK) {
        strncpy(dst, def, cap - 1);
        dst[cap - 1] = '\0';
    }
}

void config_node_id_hex(char out[33]) {
    static const char *hex = "0123456789abcdef";
    for (int i = 0; i < 16; i++) {
        out[i * 2] = hex[g.node_id[i] >> 4];
        out[i * 2 + 1] = hex[g.node_id[i] & 0xf];
    }
    out[32] = '\0';
}

ens_config_t *config_get(void) { return &g; }

ens_config_t *config_load(void) {
    nvs_handle_t h;
    esp_err_t err = nvs_open(NS, NVS_READWRITE, &h);
    if (err != ESP_OK) {
        ESP_LOGW(TAG, "nvs_open: %s — using all defaults", esp_err_to_name(err));
        memset(&g, 0, sizeof g);
    } else {
        // node_id: derive deterministically from the factory MAC (not random) so it
        // is STABLE across reflashes — a merged flash wipes NVS, and we want the same
        // id/hostname/AP name every time — and unique per board. Four CRC32s over the
        // 6-byte MAC with distinct seeds spread it across the 16-byte id (using the MAC
        // directly would leak the shared Espressif OUI into the leading bytes, so the
        // short `hex4` suffix wouldn't be unique). Still persisted, so a future
        // firmware keeps the value even if this derivation changes, and it stays
        // overridable.
        size_t idlen = sizeof g.node_id;
        if (nvs_get_blob(h, "node_id", g.node_id, &idlen) != ESP_OK || idlen != sizeof g.node_id) {
            uint8_t mac[6] = { 0 };
            esp_efuse_mac_get_default(mac);
            for (int k = 0; k < 4; k++) {
                uint32_t c = esp_rom_crc32_le(0xa5a5a5a5u + (uint32_t)k, mac, sizeof mac);
                memcpy(g.node_id + k * 4, &c, 4);
            }
            nvs_set_blob(h, "node_id", g.node_id, sizeof g.node_id);
            nvs_commit(h);
            ESP_LOGI(TAG, "derived node id from MAC %02x:%02x:%02x:%02x:%02x:%02x",
                     mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
        }
        // Default friendly name shares the AP/mDNS short id, e.g. "ensemble-8cd0",
        // so it too is stable per board (overwritten by the console / portal).
        char idhex[33]; config_node_id_hex(idhex);
        char defname[33]; snprintf(defname, sizeof defname, "ensemble-%.4s", idhex);
        get_str(h, "wifi_ssid", g.wifi_ssid, sizeof g.wifi_ssid, "");
        get_str(h, "wifi_pass", g.wifi_pass, sizeof g.wifi_pass, "");
        get_str(h, "name", g.name, sizeof g.name, defname);
        g.i2s_bclk = get_u8(h, "i2s_bclk", DEF_I2S_BCLK);
        g.i2s_lrck = get_u8(h, "i2s_lrck", DEF_I2S_LRCK);
        g.i2s_dout = get_u8(h, "i2s_dout", DEF_I2S_DOUT);
        g.i2s_mclk = get_i8(h, "i2s_mclk", DEF_I2S_MCLK);
        g.enc_a = get_u8(h, "enc_a", DEF_ENC_A);
        g.enc_b = get_u8(h, "enc_b", DEF_ENC_B);
        g.enc_sw = get_u8(h, "enc_sw", DEF_ENC_SW);
        g.led = get_i8(h, "led", DEF_LED);
        g.dac = get_u8(h, "dac", DEF_DAC);
        g.codec_pref = get_u8(h, "codec", 0);
        g.buffer_ms = get_u16(h, "buffer_ms", 150);
        g.control_port = get_u16(h, "control_port", CONFIG_ENSEMBLE_CONTROL_PORT);
        g.vol = get_u8(h, "vol", 60);
        g.disc_mode = get_u8(h, "disc_mode", 0);
        get_str(h, "master_ip", g.master_ip, sizeof g.master_ip, "");
        g.source_port = get_u16(h, "source_port", 0);
        g.stream_port = get_u16(h, "stream_port", 0);
        nvs_close(h);
    }
    g.has_apll = BOARD_HAS_APLL ? true : false;
    if (g.vol > 100) g.vol = 100;
    return &g;
}

static bool pin_ok(int p) { return p >= 0 && p < GPIO_NUM_MAX; }

bool config_validate(const ens_config_t *c, const char **reason) {
    int pins[] = { c->i2s_bclk, c->i2s_lrck, c->i2s_dout, c->enc_a, c->enc_b, c->enc_sw };
    for (size_t i = 0; i < sizeof pins / sizeof pins[0]; i++) {
        if (!pin_ok(pins[i])) { *reason = "gpio out of range"; return false; }
        for (size_t j = i + 1; j < sizeof pins / sizeof pins[0]; j++)
            if (pins[i] == pins[j]) { *reason = "duplicate gpio assignment"; return false; }
    }
    if (c->i2s_mclk >= 0 && !pin_ok(c->i2s_mclk)) { *reason = "i2s_mclk out of range"; return false; }
    if (c->control_port == 0) { *reason = "control_port must be > 0"; return false; }
    if (c->buffer_ms < 20 || c->buffer_ms > 2000) { *reason = "buffer_ms out of range (20..2000)"; return false; }
    if (c->dac > 1) { *reason = "dac must be 0 or 1"; return false; }
    if (c->codec_pref > 1) { *reason = "codec must be 0 (opus) or 1 (pcm)"; return false; }
    if (c->disc_mode == 1 && (c->master_ip[0] == '\0' || c->source_port == 0 || c->stream_port == 0)) {
        *reason = "disc_mode=1 needs master_ip + source_port + stream_port";
        return false;
    }
    return true;
}

bool config_save(void) {
    const char *reason = NULL;
    if (!config_validate(&g, &reason)) {
        ESP_LOGE(TAG, "refusing to save invalid config: %s", reason);
        return false;
    }
    nvs_handle_t h;
    if (nvs_open(NS, NVS_READWRITE, &h) != ESP_OK) return false;
    nvs_set_blob(h, "node_id", g.node_id, sizeof g.node_id);
    nvs_set_str(h, "wifi_ssid", g.wifi_ssid);
    nvs_set_str(h, "wifi_pass", g.wifi_pass);
    nvs_set_str(h, "name", g.name);
    nvs_set_u8(h, "i2s_bclk", g.i2s_bclk);
    nvs_set_u8(h, "i2s_lrck", g.i2s_lrck);
    nvs_set_u8(h, "i2s_dout", g.i2s_dout);
    nvs_set_i8(h, "i2s_mclk", g.i2s_mclk);
    nvs_set_u8(h, "enc_a", g.enc_a);
    nvs_set_u8(h, "enc_b", g.enc_b);
    nvs_set_u8(h, "enc_sw", g.enc_sw);
    nvs_set_i8(h, "led", g.led);
    nvs_set_u8(h, "dac", g.dac);
    nvs_set_u8(h, "codec", g.codec_pref);
    nvs_set_u16(h, "buffer_ms", g.buffer_ms);
    nvs_set_u16(h, "control_port", g.control_port);
    nvs_set_u8(h, "vol", g.vol);
    nvs_set_u8(h, "disc_mode", g.disc_mode);
    nvs_set_str(h, "master_ip", g.master_ip);
    nvs_set_u16(h, "source_port", g.source_port);
    nvs_set_u16(h, "stream_port", g.stream_port);
    esp_err_t err = nvs_commit(h);
    nvs_close(h);
    return err == ESP_OK;
}
