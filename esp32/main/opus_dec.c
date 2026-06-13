#include "opus_dec.h"
#include "wire.h"

#include "esp_log.h"
#include "opus.h"

static const char *TAG = "opus";
static OpusDecoder *dec;

bool opus_dec_init(void) {
    int err = 0;
    dec = opus_decoder_create(WIRE_SAMPLE_RATE, WIRE_CHANNELS, &err);
    if (err != OPUS_OK || !dec) {
        ESP_LOGE(TAG, "decoder create failed: %d", err);
        dec = NULL;
        return false;
    }
    return true;
}

void opus_dec_reset(void) {
    if (dec) opus_decoder_ctl(dec, OPUS_RESET_STATE);
}

int opus_dec_decode(const uint8_t *pkt, int len, int16_t *pcm) {
    if (!dec) return -1;
    int n = opus_decode(dec, pkt, len, pcm, WIRE_FRAME_SAMPLES, 0);
    return n < 0 ? -1 : n;
}

int opus_dec_plc(int16_t *pcm) {
    if (!dec) return -1;
    int n = opus_decode(dec, NULL, 0, pcm, WIRE_FRAME_SAMPLES, 0);
    return n < 0 ? -1 : n;
}
