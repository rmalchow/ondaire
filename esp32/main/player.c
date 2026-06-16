#include "player.h"
#include "clock.h"
#include "opus_dec.h"
#include "i2s_out.h"
#include "volume.h"
#include "servo.h"

#include <stdlib.h>
#include <string.h>
#include "esp_log.h"
#include "esp_rom_sys.h"
#include "esp_heap_caps.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

static const char *TAG = "player";

#define JITTER_SLOTS   32                 // ~640 ms at 20 ms/frame (covers bufferMs up to 300 with headroom)
#define WATCHDOG_NS    ((int64_t)2 * 1000000000LL)
#define PCM_SAMPLES    (WIRE_FRAME_SAMPLES * WIRE_CHANNELS)   // 1920 int16

// The slot stores one queued frame's payload. On the Wi-Fi path that's the
// COMPRESSED opus packet (~320 B, decoded at playout); sizing it to a full raw
// PCM frame (WIRE_FRAME_BYTES) means the wired/bench PCM path is accepted too,
// instead of being dropped at push. The buffer lives in PSRAM (the S3FH4R2 has
// 2 MB), so there's no reason to cramp it the way the 320 KB no-PSRAM parts did.
#define SLOT_BYTES     WIRE_FRAME_BYTES   // 3840 B — opus packet or a full PCM frame

typedef struct {
    bool     used;
    uint64_t seq;
    int64_t  pts;
    int      len;                         // payload bytes (opus packet)
    uint8_t  data[SLOT_BYTES];
} slot_t;

static struct {
    SemaphoreHandle_t mu;
    bool     attached;
    uint8_t  codec;                       // WIRE_CODEC_*
    uint32_t gen;
    int64_t  buffer_ns, eq_ns;
    int64_t  delay_ns;                    // device-latency comp (signed)

    bool     has_next;
    uint64_t next_seq, origin_seq, last_seq;
    int64_t  origin_pts;

    uint64_t played, silence, late;
    int64_t  last_frame_ns;
    bool     got_frame;
} p;

static slot_t  *s_slots;                  // jitter buffer (PSRAM; see player_init)
static int16_t s_pcm[PCM_SAMPLES];        // audio-task scratch (decode/output)
static uint8_t s_stage[SLOT_BYTES];       // audio-task staging (copied under lock)

static inline void lock(void)   { xSemaphoreTakeRecursive(p.mu, portMAX_DELAY); }
static inline void unlock(void) { xSemaphoreGiveRecursive(p.mu); }

// --- jitter buffer (linear-scan map; JITTER_SLOTS is small) -----------------
static slot_t *find_slot(uint64_t seq) {
    for (int i = 0; i < JITTER_SLOTS; i++)
        if (s_slots[i].used && s_slots[i].seq == seq) return &s_slots[i];
    return NULL;
}
static slot_t *alloc_slot(uint64_t seq) {
    slot_t *victim = NULL;
    for (int i = 0; i < JITTER_SLOTS; i++) {
        slot_t *s = &s_slots[i];
        if (!s->used) return s;
        if (s->seq < p.next_seq) return s;            // stale → reuse immediately
        if (!victim || s->seq < victim->seq) victim = s;  // else evict lowest seq
    }
    return victim;
}
static void clear_slots(void) {
    for (int i = 0; i < JITTER_SLOTS; i++) s_slots[i].used = false;
}
static int count_slots(void) {
    int n = 0;
    for (int i = 0; i < JITTER_SLOTS; i++) if (s_slots[i].used) n++;
    return n;
}

// --- lifecycle --------------------------------------------------------------
static void audio_task(void *arg);

bool player_init(const ens_config_t *cfg) {
    memset(&p, 0, sizeof p);
    p.mu = xSemaphoreCreateRecursiveMutex();
    p.codec = WIRE_CODEC_OPUS;
    p.buffer_ns = (int64_t)cfg->buffer_ms * 1000000;

    // Jitter buffer in PSRAM (~120 KB at 32 slots — trivial in the S3FH4R2's
    // 2 MB). Fall back to internal RAM so the bare no-PSRAM bench board still boots.
    s_slots = heap_caps_calloc(JITTER_SLOTS, sizeof(slot_t), MALLOC_CAP_SPIRAM);
    if (!s_slots) s_slots = calloc(JITTER_SLOTS, sizeof(slot_t));
    if (!s_slots) { ESP_LOGE(TAG, "jitter buffer alloc failed"); return false; }

    volume_init(cfg->vol);
    if (!opus_dec_init()) return false;
    if (!i2s_out_init(cfg->i2s_bclk, cfg->i2s_lrck, cfg->i2s_dout, cfg->i2s_mclk)) return false;
    servo_init(false);

    // Audio task at high priority; generous stack for the opus decode path.
    xTaskCreatePinnedToCore(audio_task, "audio", 8192, NULL, 23, NULL, tskNO_AFFINITY);
    return true;
}

void player_arm(uint8_t codec, uint32_t gen, int buffer_ms) {
    lock();
    p.attached = true;
    p.codec = codec;
    p.gen = gen;
    p.buffer_ns = (int64_t)buffer_ms * 1000000;
    clear_slots();
    p.has_next = false;
    p.got_frame = false;
    unlock();
    opus_dec_reset();
    servo_reset();
    ESP_LOGI(TAG, "armed codec=%s gen=%u buffer=%dms",
             codec == WIRE_CODEC_OPUS ? "opus" : "pcm", (unsigned)gen, buffer_ms);
    if (codec != WIRE_CODEC_OPUS)
        ESP_LOGW(TAG, "group codec is PCM — fine on the wired/local path, but raw "
                      "PCM frames fragment over Wi-Fi (MTU); prefer opus for radio nodes.");
    if (buffer_ms > JITTER_SLOTS * 20)
        ESP_LOGW(TAG, "bufferMs=%d exceeds jitter capacity (%d ms) — may drop frames",
                 buffer_ms, JITTER_SLOTS * 20);
}

void player_detach(void) {
    lock();
    p.attached = false;
    clear_slots();
    p.has_next = false;
    unlock();
    i2s_out_stop();
    ESP_LOGI(TAG, "detached (idle)");
}

void player_push(uint64_t seq, int64_t pts, const uint8_t *payload, int len) {
    if (len <= 0 || len > SLOT_BYTES) return;   // oversized (e.g. raw PCM) → drop
    lock();
    if (!p.attached) { unlock(); return; }
    if (!p.has_next) {
        p.next_seq = p.origin_seq = seq;
        p.origin_pts = pts;
        p.has_next = true;
    }
    if (seq < p.next_seq) { p.late++; unlock(); return; }  // already passed
    slot_t *s = alloc_slot(seq);
    if (s) {
        s->used = true; s->seq = seq; s->pts = pts; s->len = len;
        memcpy(s->data, payload, len);
        p.last_frame_ns = clock_now_ns();
        p.got_frame = true;
    }
    unlock();
}

void player_set_buffer_ms(int ms) { lock(); p.buffer_ns = (int64_t)ms * 1000000; unlock(); }
void player_set_delay(int ms)     { lock(); p.delay_ns  = (int64_t)ms * 1000000; unlock(); }
void player_set_eq(int ms)        { lock(); p.eq_ns     = (int64_t)ms * 1000000; unlock(); }
void player_set_volume(uint8_t pct, bool mute) { volume_set(pct, mute); }
void player_set_cap(uint8_t cap_id, bool on) { (void)cap_id; (void)on; /* no caps yet */ }

bool player_attached(void)  { lock(); bool a = p.attached;  unlock(); return a; }
bool player_got_frame(void) { lock(); bool g = p.got_frame; unlock(); return g; }
bool player_starved(void) {
    lock();
    bool s = p.got_frame && (clock_now_ns() - p.last_frame_ns > WATCHDOG_NS);
    unlock();
    return s;
}

// --- the playout scheduler --------------------------------------------------
static void sleep_until(int64_t local_deadline) {
    int64_t now = clock_now_ns();
    int64_t d = local_deadline - now;
    if (d <= 0) return;
    int64_t ms = d / 1000000;
    if (ms > 1) vTaskDelay(pdMS_TO_TICKS(ms - 1));   // coarse sleep, leave ~1 ms
    // fine spin for the remainder (bounded; keeps us within a frame)
    while ((local_deadline - clock_now_ns()) > 0) {
        int64_t rem = (local_deadline - clock_now_ns());
        if (rem > 200000) { vTaskDelay(1); }         // >200 µs: yield
        else esp_rom_delay_us(20);
    }
}

static void audio_task(void *arg) {
    (void)arg;
    for (;;) {
        lock();
        if (!p.attached || !p.has_next) {
            unlock();
            vTaskDelay(pdMS_TO_TICKS(5));
            continue;
        }
        uint64_t seq = p.next_seq;
        int64_t slot_pts = p.origin_pts + (int64_t)(seq - p.origin_seq) * WIRE_FRAME_NANOS;
        int64_t target_master = slot_pts + p.buffer_ns + p.eq_ns;
        int64_t delay_ns = p.delay_ns;
        unlock();

        int64_t local;
        if (!clock_master_to_local(target_master, &local)) {  // unsynced → withhold
            vTaskDelay(pdMS_TO_TICKS(5));
            continue;
        }
        local -= delay_ns;   // positive device delay → emit earlier
        sleep_until(local);

        // Decide what to emit for this slot.
        bool present = false, skip = false;
        int  stage_len = 0;
        uint8_t codec;
        lock();
        codec = p.codec;
        slot_t *s = find_slot(seq);
        if (clock_now_ns() > local + WIRE_FRAME_NANOS) {
            // A full frame late: skip without writing (else every later frame is late).
            if (s) { s->used = false; p.late++; }
            p.last_seq = seq;
            p.next_seq++;
            unlock();
            continue;
        }
        if (s) {
            present = true;
            stage_len = s->len;
            memcpy(s_stage, s->data, s->len);
            s->used = false;
            p.played++;
        } else {
            p.silence++;
        }
        p.last_seq = seq;
        p.next_seq++;
        unlock();
        (void)skip;

        // Produce one 20 ms PCM frame (outside the lock; opus state is task-local).
        if (present) {
            if (codec == WIRE_CODEC_OPUS) {
                if (opus_dec_decode(s_stage, stage_len, s_pcm) < 0)
                    memset(s_pcm, 0, sizeof s_pcm);
            } else {
                // Raw PCM (wired/bench): a full 3840 B frame now fits a slot.
                // Copy it straight through, clamped to the output frame size.
                int nb = stage_len < (int)sizeof s_pcm ? stage_len : (int)sizeof s_pcm;
                memset(s_pcm, 0, sizeof s_pcm);
                memcpy(s_pcm, s_stage, nb);
            }
        } else {
            // Gap: opus PLC keeps decoder state continuous; pcm → silence.
            if (codec == WIRE_CODEC_OPUS) {
                if (opus_dec_plc(s_pcm) < 0) memset(s_pcm, 0, sizeof s_pcm);
            } else {
                memset(s_pcm, 0, sizeof s_pcm);
            }
        }

        volume_apply(s_pcm, PCM_SAMPLES);
        i2s_out_write(s_pcm, WIRE_FRAME_BYTES);

        // Phase error = how late the emission actually landed vs the deadline.
        if (present) servo_update(clock_now_ns() - local);
    }
}

void player_fill_status(wire_status_t *st) {
    lock();
    st->buffered = (uint16_t)count_slots();
    st->last_seq = p.last_seq;
    st->played   = p.played;
    st->silence  = p.silence;
    st->late     = p.late;
    bool playing = p.attached && p.got_frame;
    unlock();

    int64_t off;
    bool synced = clock_offset_reported(&off);   // drift since lock, not the epoch gap
    st->offset_ns = synced ? off : 0;
    st->rtt_ns = clock_best_rtt_ns();
    st->rate_ppm_x1000 = servo_rate_ppm_x1000();
    st->device_delay_ns = servo_device_delay_ns();
    st->phase_err_ns = servo_phase_err_ns();
    st->flags = 0;
    if (synced) st->flags |= WIRE_ST_SYNCED;
    if (playing) st->flags |= WIRE_ST_PLAYING;
    if (servo_calibrated()) st->flags |= WIRE_ST_CALIBRATED;
}
