#include "clock.h"

#include <stdlib.h>
#include <string.h>
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"

#define WINDOW   30      // keep the last N samples
#define BEST     5       // median over the 5 smallest-RTT
#define PENDING  48      // in-flight probes ring

typedef struct { int64_t offset, rtt; } sample_t;
typedef struct { uint64_t seq; int64_t t1; bool used; } pend_t;

static struct {
    SemaphoreHandle_t mu;
    uint32_t ip;
    uint16_t port;
    bool     have_ep;
    uint32_t gen;
    uint64_t next_seq;

    pend_t   pend[PENDING];
    sample_t samples[WINDOW];
    int      nsamples;       // count, capped at WINDOW (ring)
    int      head;           // ring write index
} c;

static inline void lock(void)   { xSemaphoreTakeRecursive(c.mu, portMAX_DELAY); }
static inline void unlock(void) { xSemaphoreGiveRecursive(c.mu); }

void clock_init(void) {
    memset(&c, 0, sizeof c);
    c.mu = xSemaphoreCreateRecursiveMutex();
}

int64_t clock_now_ns(void) { return esp_timer_get_time() * 1000; }

static void wipe_samples_locked(void) {
    c.nsamples = 0;
    c.head = 0;
    for (int i = 0; i < PENDING; i++) c.pend[i].used = false;
}

void clock_set_endpoint(uint32_t ip, uint16_t port, uint32_t gen) {
    lock();
    bool changed = !c.have_ep || c.ip != ip || c.port != port;
    c.ip = ip; c.port = port; c.have_ep = true; c.gen = gen;
    for (int i = 0; i < PENDING; i++) c.pend[i].used = false;  // clear in-flight
    if (changed) wipe_samples_locked();
    unlock();
}

void clock_reset(void) { lock(); c.have_ep = false; wipe_samples_locked(); unlock(); }

uint32_t clock_gen(void) { lock(); uint32_t g = c.gen; unlock(); return g; }
void clock_set_gen(uint32_t gen) {
    lock();
    c.gen = gen;
    for (int i = 0; i < PENDING; i++) c.pend[i].used = false;
    unlock();
}

bool clock_endpoint(uint32_t *ip, uint16_t *port) {
    lock();
    bool ok = c.have_ep;
    if (ok) { *ip = c.ip; *port = c.port; }
    unlock();
    return ok;
}

uint64_t clock_begin_probe(void) {
    lock();
    uint64_t seq = c.next_seq++;
    int64_t now = clock_now_ns();
    // Record t1 in a free/oldest pending slot; prune anything older than 5 s.
    int slot = -1;
    for (int i = 0; i < PENDING; i++) {
        if (c.pend[i].used && now - c.pend[i].t1 > (int64_t)5 * 1000000000LL) c.pend[i].used = false;
        if (slot < 0 && !c.pend[i].used) slot = i;
    }
    if (slot < 0) slot = (int)(seq % PENDING);   // overwrite oldest-ish
    c.pend[slot].seq = seq; c.pend[slot].t1 = now; c.pend[slot].used = true;
    unlock();
    return seq;
}

void clock_on_reply(uint32_t gen, uint64_t seq, int64_t t2, int64_t t3, int64_t t4) {
    lock();
    if (gen != c.gen) { unlock(); return; }
    int slot = -1;
    for (int i = 0; i < PENDING; i++)
        if (c.pend[i].used && c.pend[i].seq == seq) { slot = i; break; }
    if (slot < 0) { unlock(); return; }
    int64_t t1 = c.pend[slot].t1;
    c.pend[slot].used = false;

    sample_t s = {
        .offset = ((t2 - t1) + (t3 - t4)) / 2,
        .rtt    = (t4 - t1) - (t3 - t2),
    };
    c.samples[c.head] = s;
    c.head = (c.head + 1) % WINDOW;
    if (c.nsamples < WINDOW) c.nsamples++;
    unlock();
}

static int cmp_rtt(const void *a, const void *b) {
    int64_t d = ((const sample_t *)a)->rtt - ((const sample_t *)b)->rtt;
    return (d > 0) - (d < 0);
}
static int cmp_i64(const void *a, const void *b) {
    int64_t d = *(const int64_t *)a - *(const int64_t *)b;
    return (d > 0) - (d < 0);
}

bool clock_offset(int64_t *offset_ns) {
    lock();
    int n = c.nsamples;
    if (n == 0) { unlock(); return false; }
    sample_t tmp[WINDOW];
    memcpy(tmp, c.samples, sizeof(sample_t) * n);
    unlock();

    qsort(tmp, n, sizeof(sample_t), cmp_rtt);
    int m = n < BEST ? n : BEST;
    int64_t offs[BEST];
    for (int i = 0; i < m; i++) offs[i] = tmp[i].offset;
    qsort(offs, m, sizeof(int64_t), cmp_i64);
    *offset_ns = offs[(m - 1) / 2];   // lower-middle median (matches Go)
    return true;
}

bool clock_master_to_local(int64_t master_ns, int64_t *local_ns) {
    int64_t off;
    if (!clock_offset(&off)) return false;
    *local_ns = master_ns - off;
    return true;
}

int64_t clock_best_rtt_ns(void) {
    lock();
    int n = c.nsamples;
    int64_t best = 0;
    bool have = false;
    for (int i = 0; i < n; i++) {
        if (!have || c.samples[i].rtt < best) { best = c.samples[i].rtt; have = true; }
    }
    unlock();
    return have ? best : 0;
}
