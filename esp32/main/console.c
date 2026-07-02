#include "console.h"
#include "config.h"
#include "wire.h"
#include "i2s_out.h"
#include "player.h"

#include <fcntl.h>
#include <math.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include "cJSON.h"
#include "esp_log.h"
#include "esp_system.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "sdkconfig.h"
#if CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG
#include "driver/usb_serial_jtag.h"
#include "driver/usb_serial_jtag_vfs.h"
#endif

static const char *TAG = "console";
#define CMD_LINE_MAX 512

static void reply_ok(void)            { printf("{\"ok\":true}\n"); fflush(stdout); }
static void reply_err(const char *m)  { printf("{\"ok\":false,\"err\":\"%s\"}\n", m); fflush(stdout); }

// --- helpers to pull typed fields out of a cJSON "cfg" object ---------------
static void set_str(cJSON *o, const char *k, char *dst, size_t cap) {
    cJSON *v = cJSON_GetObjectItem(o, k);
    if (cJSON_IsString(v)) { strncpy(dst, v->valuestring, cap - 1); dst[cap - 1] = '\0'; }
}
static void set_u8(cJSON *o, const char *k, uint8_t *dst) {
    cJSON *v = cJSON_GetObjectItem(o, k);
    if (cJSON_IsNumber(v)) *dst = (uint8_t)v->valueint;
}
static void set_i8(cJSON *o, const char *k, int8_t *dst) {
    cJSON *v = cJSON_GetObjectItem(o, k);
    if (cJSON_IsNumber(v)) *dst = (int8_t)v->valueint;
}
static void set_u16(cJSON *o, const char *k, uint16_t *dst) {
    cJSON *v = cJSON_GetObjectItem(o, k);
    if (cJSON_IsNumber(v)) *dst = (uint16_t)v->valueint;
}

static void do_get(void) {
    ens_config_t *g = config_get();
    char idhex[33]; config_node_id_hex(idhex);
    cJSON *root = cJSON_CreateObject();
    cJSON_AddTrueToObject(root, "ok");
    cJSON *c = cJSON_AddObjectToObject(root, "cfg");
    cJSON_AddStringToObject(c, "id", idhex);
    cJSON_AddStringToObject(c, "name", g->name);
    cJSON_AddStringToObject(c, "wifi_ssid", g->wifi_ssid);
    cJSON_AddBoolToObject(c, "wifi_set", g->wifi_pass[0] != '\0');   // never echo the password
    cJSON_AddNumberToObject(c, "i2s_bclk", g->i2s_bclk);
    cJSON_AddNumberToObject(c, "i2s_lrck", g->i2s_lrck);
    cJSON_AddNumberToObject(c, "i2s_dout", g->i2s_dout);
    cJSON_AddNumberToObject(c, "i2s_mclk", g->i2s_mclk);
    cJSON_AddNumberToObject(c, "enc_a", g->enc_a);
    cJSON_AddNumberToObject(c, "enc_b", g->enc_b);
    cJSON_AddNumberToObject(c, "enc_sw", g->enc_sw);
    cJSON_AddNumberToObject(c, "led", g->led);
    cJSON_AddNumberToObject(c, "amp_en", g->amp_en);
    cJSON_AddNumberToObject(c, "i2c_sda", g->i2c_sda);
    cJSON_AddNumberToObject(c, "i2c_scl", g->i2c_scl);
    cJSON_AddNumberToObject(c, "dac", g->dac);
    cJSON_AddNumberToObject(c, "codec", g->codec_pref);
    cJSON_AddNumberToObject(c, "buffer_ms", g->buffer_ms);
    cJSON_AddNumberToObject(c, "control_port", g->control_port);
    cJSON_AddNumberToObject(c, "vol", g->vol);
    cJSON_AddNumberToObject(c, "disc_mode", g->disc_mode);
    cJSON_AddStringToObject(c, "master_ip", g->master_ip);
    cJSON_AddNumberToObject(c, "source_port", g->source_port);
    cJSON_AddNumberToObject(c, "stream_port", g->stream_port);
    char *s = cJSON_PrintUnformatted(root);
    printf("%s\n", s); fflush(stdout);
    cJSON_free(s);
    cJSON_Delete(root);
}

static void do_set(cJSON *cmd) {
    cJSON *c = cJSON_GetObjectItem(cmd, "cfg");
    if (!cJSON_IsObject(c)) { reply_err("missing cfg"); return; }
    ens_config_t *g = config_get();
    set_str(c, "name", g->name, sizeof g->name);
    set_str(c, "wifi_ssid", g->wifi_ssid, sizeof g->wifi_ssid);
    set_str(c, "wifi_pass", g->wifi_pass, sizeof g->wifi_pass);
    set_u8(c, "i2s_bclk", &g->i2s_bclk);
    set_u8(c, "i2s_lrck", &g->i2s_lrck);
    set_u8(c, "i2s_dout", &g->i2s_dout);
    set_i8(c, "i2s_mclk", &g->i2s_mclk);
    set_u8(c, "enc_a", &g->enc_a);
    set_u8(c, "enc_b", &g->enc_b);
    set_u8(c, "enc_sw", &g->enc_sw);
    set_i8(c, "led", &g->led);
    set_i8(c, "amp_en", &g->amp_en);
    set_i8(c, "i2c_sda", &g->i2c_sda);
    set_i8(c, "i2c_scl", &g->i2c_scl);
    set_u8(c, "dac", &g->dac);
    set_u8(c, "codec", &g->codec_pref);
    set_u16(c, "buffer_ms", &g->buffer_ms);
    set_u16(c, "control_port", &g->control_port);
    set_u8(c, "vol", &g->vol);
    set_u8(c, "disc_mode", &g->disc_mode);
    set_str(c, "master_ip", g->master_ip, sizeof g->master_ip);
    set_u16(c, "source_port", &g->source_port);
    set_u16(c, "stream_port", &g->stream_port);

    const char *reason = NULL;
    if (!config_validate(g, &reason)) { reply_err(reason ? reason : "invalid"); return; }
    if (!config_save()) { reply_err("nvs write failed"); return; }
    reply_ok();
}

// 1 kHz test tone on the configured I2S — confirms DAC wiring before joining a group.
static void do_tone(void) {
    if (player_attached()) { reply_err("busy (attached to a group)"); return; }
    static int16_t frame[WIRE_FRAME_SAMPLES * WIRE_CHANNELS];
    const float step = 2.0f * (float)M_PI * 1000.0f / (float)WIRE_SAMPLE_RATE;
    float ph = 0;
    for (int f = 0; f < 50; f++) {   // ~1 s
        for (int i = 0; i < WIRE_FRAME_SAMPLES; i++) {
            int16_t s = (int16_t)(8000.0f * sinf(ph));
            ph += step; if (ph > 2 * (float)M_PI) ph -= 2 * (float)M_PI;
            frame[i * 2] = s; frame[i * 2 + 1] = s;
        }
        i2s_out_write(frame, sizeof frame);
    }
    reply_ok();
}

static void handle_line(char *line) {
    cJSON *cmd = cJSON_Parse(line);
    if (!cmd) return;   // not JSON (log noise / Improv frames) → ignore
    cJSON *c = cJSON_GetObjectItem(cmd, "cmd");
    if (cJSON_IsString(c)) {
        if (!strcmp(c->valuestring, "get")) do_get();
        else if (!strcmp(c->valuestring, "set")) do_set(cmd);
        else if (!strcmp(c->valuestring, "test")) do_tone();
        else if (!strcmp(c->valuestring, "reboot")) { reply_ok(); vTaskDelay(pdMS_TO_TICKS(100)); esp_restart(); }
        else reply_err("unknown cmd");
    }
    cJSON_Delete(cmd);
}

static void console_task(void *arg) {
    (void)arg;
    fcntl(STDIN_FILENO, F_SETFL, fcntl(STDIN_FILENO, F_GETFL) | O_NONBLOCK);
    char line[CMD_LINE_MAX];
    int len = 0;
    for (;;) {
        char ch;
        int r = read(STDIN_FILENO, &ch, 1);
        if (r != 1) { vTaskDelay(pdMS_TO_TICKS(20)); continue; }
        if (ch == '\r') continue;
        if (ch == '\n') {
            line[len] = '\0';
            if (len) handle_line(line);
            len = 0;
        } else if (len < CMD_LINE_MAX - 1) {
            line[len++] = ch;
        } else {
            len = 0;   // overrun → drop the line
        }
    }
}

bool console_init(void) {
#if CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG
    // Native USB-Serial-JTAG console (Super Mini / S3-Zero / DevKitC): install the
    // driver and route the VFS through it, or read(STDIN) never receives bytes —
    // the device never drains the USB OUT endpoint and host writes time out, so
    // provisioning is impossible. UART0 consoles (e.g. the S2 CH341 board) already
    // have a working stdin, hence the guard.
    usb_serial_jtag_driver_config_t ucfg = USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    usb_serial_jtag_driver_install(&ucfg);
    usb_serial_jtag_vfs_use_driver();
#endif
    xTaskCreate(console_task, "console", 4096, NULL, 7, NULL);
    ESP_LOGI(TAG, "USB JSON console ready ({\"cmd\":\"get\"})");
    return true;
}
