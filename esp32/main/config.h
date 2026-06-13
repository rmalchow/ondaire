// config.h — persistent node configuration (NVS namespace "ensemble"). Every
// field has a board default (boards/board.h) so an unprovisioned board still
// boots; the USB JSON console (console.c) overwrites them. See docs/esp32.md §5.
#pragma once
#include <stdbool.h>
#include <stdint.h>

typedef struct {
    uint8_t  node_id[16];      // immutable, minted on first boot; mDNS `id`

    char     wifi_ssid[33];
    char     wifi_pass[64];

    char     name[33];         // friendly label (logs / mDNS instance)

    // I2S pins to the DAC. mclk = -1 means MCLK-less (PCM5102A).
    uint8_t  i2s_bclk, i2s_lrck, i2s_dout;
    int8_t   i2s_mclk;

    // Rotary encoder pins.
    uint8_t  enc_a, enc_b, enc_sw;
    int8_t   led;              // status LED, -1 if none

    uint8_t  dac;              // 0 = PCM5102A (sw gain), 1 = PCM5122 (I2C)
    uint8_t  codec_pref;       // advertised codec pref: 0 = opus, 1 = pcm
    uint16_t buffer_ms;        // playout lead default (master overrides via ATTACH)
    uint16_t control_port;     // CONTROL_PORT advertised over mDNS
    uint8_t  vol;              // last volume 0..100, restored on boot

    // Fixed-master bench fallback (disc_mode=1): self-attach with no master
    // driving us. disc_mode=0 (default) = wait for ATTACH (mDNS-discovered).
    uint8_t  disc_mode;
    char     master_ip[16];
    uint16_t source_port;
    uint16_t stream_port;

    bool     has_apll;         // board capability (queue=1 servo); not persisted
} ens_config_t;

// Load config from NVS, filling missing keys from board defaults and minting a
// node_id on first boot. Returns the shared in-RAM config (also via config_get).
ens_config_t *config_load(void);

// Pointer to the shared in-RAM config.
ens_config_t *config_get(void);

// Validate (pin ranges, no pin collisions, sane ports). false + reason on error.
bool config_validate(const ens_config_t *c, const char **reason);

// Persist the in-RAM config to NVS (after validation). Returns false on error.
bool config_save(void);

// Hex-encode node_id into out (>=33 bytes) -> 32 hex chars + NUL.
void config_node_id_hex(char out[33]);
