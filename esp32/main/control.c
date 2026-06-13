#include "control.h"
#include "wire.h"
#include "net_audio.h"
#include "player.h"
#include "status.h"

#include <string.h>
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"
#include "lwip/sockets.h"

static const char *TAG = "ctrl";

// applied = last command state forwarded to the player, so a repeated soft-state
// assertion with identical values is a no-op (mirrors playback/control.go).
static struct {
    SemaphoreHandle_t mu;
    int      sock;

    bool     attached;
    uint32_t src_ip, clk_ip;
    uint16_t src_port, clk_port;
    uint8_t  codec, transport;
    uint16_t buffer_ms;

    bool     have_vol;  uint8_t vol_pct;  bool mute;
    bool     have_delay; int16_t delay_ms;
    bool     have_eq;    uint16_t eq_ms;

    bool     have_status_dst;
    uint32_t status_ip;
    uint16_t status_port;
} c;

static inline void lock(void)   { xSemaphoreTakeRecursive(c.mu, portMAX_DELAY); }
static inline void unlock(void) { xSemaphoreGiveRecursive(c.mu); }

static void send_status_to(uint32_t ip, uint16_t port) {
    if (port == 0) return;
    uint8_t pl[WIRE_STATUS_LEN];
    int n = status_build(pl, sizeof pl);
    if (n < 0) return;
    uint8_t frame[WIRE_HEADER_SIZE + WIRE_STATUS_LEN];
    int len = wire_frame_encode(frame, sizeof frame, WIRE_STATUS, 0, 0, 0, pl, n);
    struct sockaddr_in dst;
    memset(&dst, 0, sizeof dst);
    dst.sin_family = AF_INET;
    dst.sin_addr.s_addr = htonl(ip);
    dst.sin_port = htons(port);
    sendto(c.sock, frame, len, 0, (struct sockaddr *)&dst, sizeof dst);
}

static void on_attach(const wire_attach_t *a) {
    lock();
    c.have_status_dst = true; c.status_ip = a->source_ip; c.status_port = a->source_port;
    bool changed = !c.attached ||
        c.src_ip != a->source_ip || c.src_port != a->source_port ||
        c.clk_ip != a->clock_ip  || c.clk_port != a->clock_port ||
        c.codec != a->codec || c.transport != a->transport;
    c.attached = true;
    c.src_ip = a->source_ip; c.src_port = a->source_port;
    c.clk_ip = a->clock_ip;  c.clk_port = a->clock_port;
    c.codec = a->codec; c.transport = a->transport; c.buffer_ms = a->buffer_ms;
    unlock();

    if (!changed) {                  // soft-state heartbeat: just refresh the lead
        player_set_buffer_ms(a->buffer_ms);
        return;
    }
    net_audio_attach(a->source_ip, a->source_port, a->clock_ip, a->clock_port,
                     a->codec, a->transport, a->buffer_ms);
}

static void on_detach(void) {
    lock(); bool was = c.attached; c.attached = false; unlock();
    if (was) net_audio_detach();
}

static void on_setvol(uint8_t pct, bool mute) {
    lock();
    bool changed = !c.have_vol || c.vol_pct != pct || c.mute != mute;
    c.have_vol = true; c.vol_pct = pct; c.mute = mute;
    unlock();
    if (changed) player_set_volume(pct, mute);
}

static void on_setdelay(int16_t ms) {
    lock();
    bool changed = !c.have_delay || c.delay_ms != ms;
    c.have_delay = true; c.delay_ms = ms;
    unlock();
    if (changed) player_set_delay(ms);   // re-anchoring every heartbeat is audible
}

static void on_seteq(uint16_t ms) {
    lock();
    bool changed = !c.have_eq || c.eq_ms != ms;
    c.have_eq = true; c.eq_ms = ms;
    unlock();
    if (changed) player_set_eq(ms);
}

static void handle(uint8_t type, const uint8_t *payload, int plen,
                   uint32_t from_ip, uint16_t from_port) {
    switch (type) {
    case WIRE_ATTACH: {
        wire_attach_t a;
        if (wire_attach_decode(payload, plen, &a)) on_attach(&a);
        break;
    }
    case WIRE_DETACH:  on_detach(); break;
    case WIRE_SETVOL:  if (plen >= 2) on_setvol(payload[0], payload[1] & 0x01); break;
    case WIRE_SETDELAY: if (plen >= 2) on_setdelay((int16_t)((payload[0] << 8) | payload[1])); break;
    case WIRE_SETEQ:   if (plen >= 2) on_seteq((uint16_t)((payload[0] << 8) | payload[1])); break;
    case WIRE_SETCAP:  if (plen >= 2) player_set_cap(payload[0], payload[1] != 0); break;
    case WIRE_STATUSREQ:
        // Liveness poll (D60): reply to the requester even while idle.
        send_status_to(from_ip, from_port);
        break;
    default: break;   // forward-compat ignore
    }
}

static uint8_t s_buf[2048];

static void read_task(void *arg) {
    (void)arg;
    for (;;) {
        struct sockaddr_in from; socklen_t fl = sizeof from;
        int r = recvfrom(c.sock, s_buf, sizeof s_buf, 0, (struct sockaddr *)&from, &fl);
        if (r < WIRE_HEADER_SIZE || s_buf[0] != WIRE_MAGIC) continue;
        wire_header_t h;
        if (!wire_header_decode(s_buf, r, &h)) continue;
        int plen = r - WIRE_HEADER_SIZE;
        if ((int)h.payload_len < plen) plen = h.payload_len;
        handle(h.type, s_buf + WIRE_HEADER_SIZE, plen,
               ntohl(from.sin_addr.s_addr), ntohs(from.sin_port));
    }
}

static void status_task(void *arg) {
    (void)arg;
    for (;;) {
        vTaskDelay(pdMS_TO_TICKS(1000));
        lock();
        bool attached = c.attached;
        uint32_t ip = c.status_ip; uint16_t port = c.status_port;
        bool have = c.have_status_dst;
        unlock();
        if (attached && have) send_status_to(ip, port);
    }
}

bool control_init(uint16_t control_port) {
    memset(&c, 0, sizeof c);
    c.mu = xSemaphoreCreateRecursiveMutex();
    c.sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (c.sock < 0) { ESP_LOGE(TAG, "socket failed"); return false; }
    struct sockaddr_in a; memset(&a, 0, sizeof a);
    a.sin_family = AF_INET;
    a.sin_addr.s_addr = htonl(INADDR_ANY);
    a.sin_port = htons(control_port);
    if (bind(c.sock, (struct sockaddr *)&a, sizeof a) < 0) {
        ESP_LOGE(TAG, "bind :%u failed", (unsigned)control_port); close(c.sock); return false;
    }
    xTaskCreate(read_task,   "ctrl_rx", 3584, NULL, 19, NULL);
    xTaskCreate(status_task, "ctrl_st", 2560, NULL, 9,  NULL);
    ESP_LOGI(TAG, "control plane on :%u", (unsigned)control_port);
    return true;
}
