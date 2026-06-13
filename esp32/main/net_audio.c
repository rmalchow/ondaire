#include "net_audio.h"
#include "wire.h"
#include "clock.h"
#include "player.h"

#include <string.h>
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"
#include "lwip/sockets.h"

static const char *TAG = "net";

#define HELLO_RETRIES   3
#define RETRY_MS        500
#define KEEPALIVE_MS    5000
#define CLOCK_BURST_N   15        // cold-start clock probes...
#define CLOCK_BURST_MS  13        // ...~every 13 ms (~200 ms total), then 1 Hz

static struct {
    SemaphoreHandle_t mu;
    int       udp;                // audio + clock socket (ephemeral)
    bool      have_sub;
    uint32_t  src_ip, clk_ip;
    uint16_t  src_port, clk_port;
    uint8_t   codec, transport;
    uint16_t  buffer_ms;

    int       tcp;                // >=0 when transport==tcp and connected
    int64_t   attach_ns;          // for cold-start burst window
    int       hellos_left;        // fast-retry budget while no frame yet
    int64_t   last_hello_ns;
    bool      restart_sent;
} n;

static inline void lock(void)   { xSemaphoreTakeRecursive(n.mu, portMAX_DELAY); }
static inline void unlock(void) { xSemaphoreGiveRecursive(n.mu); }

static void fill_sin(struct sockaddr_in *sa, uint32_t ip, uint16_t port) {
    memset(sa, 0, sizeof *sa);
    sa->sin_family = AF_INET;
    sa->sin_addr.s_addr = htonl(ip);
    sa->sin_port = htons(port);
}

// TCP length framing: u32-BE length prefix, then the frame.
static bool tcp_write_frame(int fd, const uint8_t *frame, int len) {
    uint8_t lp[4] = { (uint8_t)(len >> 24), (uint8_t)(len >> 16), (uint8_t)(len >> 8), (uint8_t)len };
    if (send(fd, lp, 4, 0) != 4) return false;
    return send(fd, frame, len, 0) == len;
}

// Send a data-plane control frame (HELLO/BYE/RESTART) to the source endpoint.
static void send_control(uint8_t type, bool prime) {
    uint8_t pl[1] = { prime ? WIRE_FLAG_PRIME : 0 };
    uint8_t frame[WIRE_HEADER_SIZE + 1];
    int len = wire_frame_encode(frame, sizeof frame, type, clock_gen(), 0, 0, pl, 1);
    lock();
    int tcp = n.tcp; uint8_t tr = n.transport;
    struct sockaddr_in dst; fill_sin(&dst, n.src_ip, n.src_port);
    int udp = n.udp;
    unlock();
    if (tr == WIRE_TRANSPORT_TCP) {
        if (tcp >= 0) tcp_write_frame(tcp, frame, len);
    } else {
        sendto(udp, frame, len, 0, (struct sockaddr *)&dst, sizeof dst);
    }
}

// --- inbound dispatch -------------------------------------------------------
static void on_audio(const wire_header_t *h, const uint8_t *payload, int len) {
    uint32_t cur = clock_gen();
    if (h->gen != cur) {
        if (h->gen > cur) {                 // new/replaced session → re-arm up
            lock(); uint8_t codec = n.codec; uint16_t bms = n.buffer_ms; unlock();
            clock_set_gen(h->gen);
            player_arm(codec, h->gen, bms);
        } else {
            return;                          // stale generation → drop
        }
    }
    player_push(h->seq, h->pts, payload, len);
}

static void on_reconfig(const wire_header_t *h, const uint8_t *payload, int len) {
    bool stop = len > 0 && (payload[0] & WIRE_FLAG_STOP);
    lock(); uint8_t codec = n.codec; uint16_t bms = n.buffer_ms; unlock();
    ESP_LOGI(TAG, "RECONFIG gen=%u stop=%d", (unsigned)h->gen, stop);
    if (stop) {
        player_arm(codec, h->gen, bms);      // drop playout, await next session
        return;
    }
    clock_set_gen(h->gen);
    player_arm(codec, h->gen, bms);
    send_control(WIRE_HELLO, true);          // re-subscribe under the new gen
}

static void dispatch(const uint8_t *buf, int nbytes, int64_t t4) {
    if (nbytes < WIRE_HEADER_SIZE || buf[0] != WIRE_MAGIC) return;
    wire_header_t h;
    if (!wire_header_decode(buf, nbytes, &h)) return;
    const uint8_t *payload = buf + WIRE_HEADER_SIZE;
    int plen = nbytes - WIRE_HEADER_SIZE;
    if ((int)h.payload_len < plen) plen = h.payload_len;
    switch (h.type) {
    case WIRE_CLOCK_RSP:
        if (plen >= 24) {
            int64_t t2 = 0, t3 = 0;
            for (int i = 0; i < 8; i++) { t2 = (t2 << 8) | payload[8 + i]; }
            for (int i = 0; i < 8; i++) { t3 = (t3 << 8) | payload[16 + i]; }
            clock_on_reply(h.gen, h.seq, t2, t3, t4);
        }
        break;
    case WIRE_AUDIO:    on_audio(&h, payload, plen); break;
    case WIRE_RECONFIG: on_reconfig(&h, payload, plen); break;
    case WIRE_FEC:      break;   // optional; gaps play silence/PLC
    default:            break;   // forward-compat: ignore unknown types
    }
}

// --- tasks ------------------------------------------------------------------
static uint8_t s_rxbuf[2048];

static void udp_read_task(void *arg) {
    (void)arg;
    for (;;) {
        struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
        setsockopt(n.udp, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof tv);
        int r = recv(n.udp, s_rxbuf, sizeof s_rxbuf, 0);
        int64_t t4 = clock_now_ns();
        if (r > 0) dispatch(s_rxbuf, r, t4);
    }
}

static int s_tcp_task_gen;   // bumped on each attach to retire stale TCP tasks

static void tcp_read_task(void *arg) {
    int my_gen = (int)(intptr_t)arg;
    int fd;
    lock(); fd = n.tcp; unlock();
    uint8_t hdr[4];
    for (;;) {
        if (my_gen != s_tcp_task_gen) break;     // a newer attach replaced us
        // read 4-byte length prefix
        int got = 0;
        while (got < 4) {
            int r = recv(fd, hdr + got, 4 - got, 0);
            if (r <= 0) goto done;
            got += r;
        }
        uint32_t flen = ((uint32_t)hdr[0] << 24) | ((uint32_t)hdr[1] << 16) |
                        ((uint32_t)hdr[2] << 8) | hdr[3];
        if (flen < WIRE_HEADER_SIZE || flen > sizeof s_rxbuf) goto done;
        got = 0;
        while (got < (int)flen) {
            int r = recv(fd, s_rxbuf + got, flen - got, 0);
            if (r <= 0) goto done;
            got += r;
        }
        dispatch(s_rxbuf, flen, clock_now_ns());
    }
done:
    if (fd >= 0) close(fd);
    lock(); if (n.tcp == fd) n.tcp = -1; unlock();
    vTaskDelete(NULL);
}

// Clock prober: cold-start burst, then steady 1 Hz (PLAYER §7).
static void clock_task(void *arg) {
    (void)arg;
    for (;;) {
        uint32_t ip; uint16_t port;
        if (clock_endpoint(&ip, &port)) {
            uint64_t seq = clock_begin_probe();
            uint8_t pl[24]; memset(pl, 0, sizeof pl);
            uint8_t frame[WIRE_HEADER_SIZE + 24];
            int len = wire_frame_encode(frame, sizeof frame, WIRE_CLOCK_REQ,
                                        clock_gen(), seq, 0, pl, sizeof pl);
            struct sockaddr_in dst; fill_sin(&dst, ip, port);
            sendto(n.udp, frame, len, 0, (struct sockaddr *)&dst, sizeof dst);
        }
        lock();
        bool burst = n.have_sub && (clock_now_ns() - n.attach_ns) < (int64_t)CLOCK_BURST_N * CLOCK_BURST_MS * 1000000LL;
        unlock();
        vTaskDelay(pdMS_TO_TICKS(burst ? CLOCK_BURST_MS : 1000));
    }
}

// Keepalive + fast-retry + RESTART watchdog (PLAYER §8).
static void keepalive_task(void *arg) {
    (void)arg;
    for (;;) {
        vTaskDelay(pdMS_TO_TICKS(250));
        lock();
        bool sub = n.have_sub;
        int64_t now = clock_now_ns();
        unlock();
        if (!sub) continue;

        if (!player_got_frame()) {
            lock();
            bool due = n.hellos_left > 0 && (now - n.last_hello_ns) >= RETRY_MS * 1000000LL;
            if (due) { n.hellos_left--; n.last_hello_ns = now; }
            unlock();
            if (due) send_control(WIRE_HELLO, true);   // re-prime
        } else {
            lock();
            bool due = (now - n.last_hello_ns) >= KEEPALIVE_MS * 1000000LL;
            if (due) n.last_hello_ns = now;
            unlock();
            if (due) send_control(WIRE_HELLO, false);   // keepalive
        }

        // RESTART on >2 s starvation; re-arm the flag once frames resume.
        if (player_starved()) {
            lock(); bool send = !n.restart_sent; n.restart_sent = true; unlock();
            if (send) { ESP_LOGW(TAG, "starved >2s: RESTART"); send_control(WIRE_RESTART, true); }
        } else {
            lock(); n.restart_sent = false; unlock();
        }
    }
}

// --- public API -------------------------------------------------------------
bool net_audio_init(void) {
    memset(&n, 0, sizeof n);
    n.mu = xSemaphoreCreateRecursiveMutex();
    n.tcp = -1;

    n.udp = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (n.udp < 0) { ESP_LOGE(TAG, "udp socket failed"); return false; }
    struct sockaddr_in local; memset(&local, 0, sizeof local);
    local.sin_family = AF_INET;
    local.sin_addr.s_addr = htonl(INADDR_ANY);
    local.sin_port = 0;   // ephemeral
    if (bind(n.udp, (struct sockaddr *)&local, sizeof local) < 0) {
        ESP_LOGE(TAG, "udp bind failed"); close(n.udp); return false;
    }
    xTaskCreate(udp_read_task, "net_rx", 3584, NULL, 20, NULL);
    xTaskCreate(clock_task,    "net_clk", 2560, NULL, 18, NULL);
    xTaskCreate(keepalive_task,"net_ka",  2560, NULL, 10, NULL);
    return true;
}

void net_audio_attach(uint32_t src_ip, uint16_t src_port,
                      uint32_t clk_ip, uint16_t clk_port,
                      uint8_t codec, uint8_t transport, uint16_t buffer_ms) {
    // Close any previous TCP connection and retire its reader.
    lock();
    int old_tcp = n.tcp; n.tcp = -1; s_tcp_task_gen++;
    n.have_sub = true;
    n.src_ip = src_ip; n.src_port = src_port;
    n.clk_ip = clk_ip; n.clk_port = clk_port;
    n.codec = codec; n.transport = transport; n.buffer_ms = buffer_ms;
    n.attach_ns = clock_now_ns();
    n.hellos_left = HELLO_RETRIES;
    n.last_hello_ns = 0;
    n.restart_sent = false;
    int my_tcp_gen = s_tcp_task_gen;
    unlock();
    if (old_tcp >= 0) close(old_tcp);

    clock_set_endpoint(clk_ip, clk_port, 0);   // gen rises from the audio stream
    player_arm(codec, 0, buffer_ms);

    if (transport == WIRE_TRANSPORT_TCP) {
        int fd = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
        if (fd >= 0) {
            struct sockaddr_in dst; fill_sin(&dst, src_ip, src_port);
            if (connect(fd, (struct sockaddr *)&dst, sizeof dst) == 0) {
                lock(); n.tcp = fd; unlock();
                uint8_t pl[1] = { WIRE_FLAG_PRIME };
                uint8_t frame[WIRE_HEADER_SIZE + 1];
                int len = wire_frame_encode(frame, sizeof frame, WIRE_HELLO, 0, 0, 0, pl, 1);
                tcp_write_frame(fd, frame, len);
                xTaskCreate(tcp_read_task, "net_tcp", 3584, (void *)(intptr_t)my_tcp_gen, 20, NULL);
            } else {
                ESP_LOGE(TAG, "tcp connect failed"); close(fd);
            }
        }
    } else {
        send_control(WIRE_HELLO, true);        // UDP subscribe + prime
        lock(); n.last_hello_ns = clock_now_ns(); n.hellos_left = HELLO_RETRIES; unlock();
    }
    ESP_LOGI(TAG, "attach src=%u.%u.%u.%u:%u transport=%s codec=%s buffer=%ums",
             (unsigned)((src_ip>>24)&0xff), (unsigned)((src_ip>>16)&0xff),
             (unsigned)((src_ip>>8)&0xff), (unsigned)(src_ip&0xff), (unsigned)src_port,
             transport == WIRE_TRANSPORT_TCP ? "tcp" : "udp",
             codec == WIRE_CODEC_OPUS ? "opus" : "pcm", (unsigned)buffer_ms);
}

void net_audio_detach(void) {
    lock();
    bool had = n.have_sub;
    n.have_sub = false;
    int tcp = n.tcp; n.tcp = -1; s_tcp_task_gen++;
    unlock();
    if (had) send_control(WIRE_BYE, false);
    if (tcp >= 0) close(tcp);
    clock_reset();
    player_detach();
}
