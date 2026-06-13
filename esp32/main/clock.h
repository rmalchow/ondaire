// clock.h — master-anchored clock follower (PLAYER §7). NTP-style 4-stamp
// exchange, offset = ((t2-t1)+(t3-t4))/2, estimate = median offset of the 5
// best-RTT of the last 30 samples. All local times are esp_timer ns (monotonic).
#pragma once
#include <stdbool.h>
#include <stdint.h>

void     clock_init(void);

// Local monotonic time in nanoseconds (esp_timer based). The ONE clock used for
// t1/t4 and every playout deadline.
int64_t  clock_now_ns(void);

// (Re)point at a master clock endpoint under generation gen. An endpoint change
// wipes the sample window (unsynced); a gen-only change keeps it.
void     clock_set_endpoint(uint32_t ip, uint16_t port, uint32_t gen);
void     clock_reset(void);             // clear samples, go unsynced
uint32_t clock_gen(void);
void     clock_set_gen(uint32_t gen);   // keep samples, bump gen (RECONFIG/audio)
bool     clock_endpoint(uint32_t *ip, uint16_t *port);  // false if unset

// Probe bookkeeping: net calls clock_begin_probe() to get the seq to stamp into
// the CLOCK_REQ header (records t1=now internally), then sends the datagram.
uint64_t clock_begin_probe(void);

// Feed a CLOCK_RSP. t4 = local arrival time (ns). Ignored if gen mismatches or
// the seq is unknown/stale.
void     clock_on_reply(uint32_t gen, uint64_t seq, int64_t t2, int64_t t3, int64_t t4);

// Current estimate. Returns false (unsynced) until >=1 sample exists.
bool     clock_offset(int64_t *offset_ns);
bool     clock_master_to_local(int64_t master_ns, int64_t *local_ns);
int64_t  clock_best_rtt_ns(void);       // smallest RTT in the window (0 if none)
