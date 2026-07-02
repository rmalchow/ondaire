#include "player.h"
#include "clock.h"
#include "opus_dec.h"
#include "i2s_out.h"
#include "volume.h"
#include "servo.h"
#include "pcm5122.h"
#include "tas58xx.h"
#include "ma12070p.h"

#include <stdlib.h>
#include <string.h>
#include "esp_log.h"
#include "esp_rom_sys.h"
#include "esp_heap_caps.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

static const char *TAG = "player";

#define JITTER_SLOTS   32                 // ~640 ms at 20 ms/frame (covers bufferMs up to 300 with headroom)
#define WATCHDOG_NS    ((int64_t)2 * 1000000000LL)
#define PCM_SAMPLES    (WIRE_FRAME_SAMPLES * WIRE_CHANNELS)   // 1920 int16

// Amp power gating (boards with an amp_en pin): drop the amp after this much
// CONTINUOUS silence so an idle node isn't burning the class-D stage (and any
// idle hiss goes away). Debounced well above a track gap because the TPA pops on
// each power transition — we accept one pop at play-start / idle-stop, not one
// per short gap. Detach cuts it immediately.
#define AMP_IDLE_NS    ((int64_t)5 * 1000000000LL)   // 5 s

// Consecutive gap (silence) frames before we declare the jitter buffer drained
// and re-anchor: drop has_next so the next arriving frame re-primes origin_seq to
// the live stream, instead of marching next_seq past every arrival forever (the
// old permanent-underrun trap). ~500 ms — under the master's 2 s starvation watchdog.
#define REANCHOR_FRAMES    25

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
    uint64_t injected, dropped;           // grounded servo: extra/removed sample-pairs
    int64_t  last_frame_ns;
    bool     got_frame;
} p;

static slot_t  *s_slots;                  // jitter buffer (PSRAM; see player_init)
// One extra sample-pair of headroom so a servo INSERT (+1) frame fits in place.
static int16_t s_pcm[(WIRE_FRAME_SAMPLES + 1) * WIRE_CHANNELS]; // audio-task scratch
static uint8_t s_stage[SLOT_BYTES];       // audio-task staging (copied under lock)

static inline void lock(void)   { xSemaphoreTakeRecursive(p.mu, portMAX_DELAY); }
static inline void unlock(void) { xSemaphoreGiveRecursive(p.mu); }

// --- onboard amp power gate (amp_en boards, e.g. Amped-ESP32-S3-Plus) --------
// s_amp_pin is the GPIO or -1 (no amp). amp_set() drives it and is idempotent, so
// the audio task can call it every frame cheaply. All amp writes go through here
// and only from the audio task + detach, so s_amp_on needs no lock.
static int  s_amp_pin = -1;
static bool s_amp_on  = false;
static void amp_set(bool on) {
    if (s_amp_pin < 0 || on == s_amp_on) return;
    gpio_set_level((gpio_num_t)s_amp_pin, on ? 1 : 0);
    s_amp_on = on;
    ESP_LOGI(TAG, "amp %s", on ? "on" : "off (idle)");
}

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

    // I2C-controlled DACs/amps: configure them over I2C now that the I2S clocks are
    // running, BEFORE un-muting a separate amp. These parts are MCLK-less and stay
    // silent until told to reference BCK / brought out of reset, so this is what
    // actually produces analog output. dac=0 (PCM5102A/PCM5100A/MAX98357) needs no
    // init. cfg->dac_en is the DAC/amp hard-enable pin (each driver drives it with
    // the part's own power-up timing). Failure just logs — the node still runs.
    switch (cfg->dac) {
    case 1: pcm5122_init(cfg->i2c_sda, cfg->i2c_scl, cfg->dac_en); break;  // PCM5122
    case 2: tas58xx_init(cfg->i2c_sda, cfg->i2c_scl, cfg->dac_en); break;  // TAS5805M/5825M
    case 3: ma12070p_init(cfg->i2c_sda, cfg->i2c_scl, cfg->dac_en); break; // MA12070P
    default: break;                                                        // dac=0: none
    }

    // Boards with an onboard class-D amp (e.g. Amped-ESP32-S3-Plus, TPA3110 on
    // GPIO17) gate the speaker output behind an active-high un-mute pin. Configure
    // it as an output and start LOW (amp off): the audio task powers it on when a
    // stream is actually playing and drops it after AMP_IDLE_NS of silence, so an
    // idle node draws no amp power. -1 = no such pin (all the PCM5102A boards). The
    // PCM5122's own soft-mute is hardwired on this board (rev C "No DAC_EN").
    if (cfg->amp_en >= 0) {
        s_amp_pin = cfg->amp_en;
        gpio_config_t io = {
            .pin_bit_mask = 1ULL << cfg->amp_en,
            .mode = GPIO_MODE_OUTPUT,
        };
        gpio_config(&io);
        gpio_set_level((gpio_num_t)s_amp_pin, 0);   // start muted; play powers it on
        ESP_LOGI(TAG, "amp gate on GPIO%d (idle-off after %llds)",
                 s_amp_pin, (long long)(AMP_IDLE_NS / 1000000000LL));
    }

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
    amp_set(false);   // left the room → cut the amp (no pop-inducing idle draw)
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
    const size_t frame_bytes = PCM_SAMPLES * sizeof(int16_t);
    int silence_run = 0;       // consecutive gap frames (for re-anchor)
    int64_t last_present_ns = 0;   // last real (non-silence) frame, for amp gating
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
        uint8_t codec = p.codec;
        unlock();

        // Master-paced playout: this frame's slot is due at master time
        // target_master; convert to local and sleep to it. This holds absolute phase
        // (so all nodes line up); the feed-forward sample insert/drop below keeps the
        // I2S DMA fed despite the local crystal's ppm offset (no APLL to trim it).
        int64_t local;
        if (!clock_master_to_local(target_master, &local)) {  // unsynced → withhold
            vTaskDelay(pdMS_TO_TICKS(5));
            continue;
        }
        local -= delay_ns;   // positive device delay → emit earlier
        sleep_until(local);

        // Decide what to emit for this slot.
        bool present = false;
        int  stage_len = 0;
        lock();
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

        // Produce one 20 ms PCM frame (outside the lock; opus state is task-local).
        if (present) {
            if (codec == WIRE_CODEC_OPUS) {
                if (opus_dec_decode(s_stage, stage_len, s_pcm) < 0)
                    memset(s_pcm, 0, frame_bytes);
            } else {
                // Raw PCM (wired/bench): a full 3840 B frame fits a slot.
                int nb = stage_len < (int)frame_bytes ? stage_len : (int)frame_bytes;
                memset(s_pcm, 0, frame_bytes);
                memcpy(s_pcm, s_stage, nb);
            }
            silence_run = 0;
        } else {
            // Gap: opus PLC keeps decoder state continuous; pcm → silence.
            if (codec == WIRE_CODEC_OPUS) {
                if (opus_dec_plc(s_pcm) < 0) memset(s_pcm, 0, frame_bytes);
            } else {
                memset(s_pcm, 0, frame_bytes);
            }
            // Jitter buffer drained too long → re-anchor so the next push re-primes us
            // to the live stream, instead of marching next_seq past every arrival.
            if (++silence_run >= REANCHOR_FRAMES) {
                lock(); p.has_next = false; unlock();
                opus_dec_reset();
                servo_reset();
                silence_run = 0;
                continue;
            }
        }

        // Power-gate the onboard amp (no-op if the board has no amp_en pin): on
        // while real audio is flowing, off after AMP_IDLE_NS of continuous silence.
        if (present) { last_present_ns = clock_now_ns(); amp_set(true); }
        else if (last_present_ns && clock_now_ns() - last_present_ns > AMP_IDLE_NS) amp_set(false);

        // Phase telemetry = how late the emission landed vs the deadline.
        if (present) servo_update(clock_now_ns() - local);

        // Feed-forward rate match: nudge the frame ±1 sample-pair per the MEASURED
        // crystal drift so the long-run output rate equals the master's and the DMA
        // never chronically drains. No phase feedback ⇒ it cannot rail on offset steps.
        servo_set_drift_ppm((float)clock_drift_ppm_x1000() / 1000.0f);
        int adj = servo_sample_adjust();
        int out_pairs = WIRE_FRAME_SAMPLES + adj;
        if (adj > 0) {
            s_pcm[WIRE_FRAME_SAMPLES * 2]     = s_pcm[(WIRE_FRAME_SAMPLES - 1) * 2];
            s_pcm[WIRE_FRAME_SAMPLES * 2 + 1] = s_pcm[(WIRE_FRAME_SAMPLES - 1) * 2 + 1];
            lock(); p.injected++; unlock();
        } else if (adj < 0) {
            lock(); p.dropped++; unlock();
        }

        volume_apply(s_pcm, out_pairs * WIRE_CHANNELS);
        i2s_out_write(s_pcm, (size_t)out_pairs * WIRE_CHANNELS * sizeof(int16_t));
    }
}

void player_fill_status(wire_status_t *st) {
    lock();
    st->buffered = (uint16_t)count_slots();
    st->last_seq = p.last_seq;
    st->played   = p.played;
    st->silence  = p.silence;
    st->late     = p.late;
    st->samples_injected = p.injected;
    st->samples_dropped  = p.dropped;
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
