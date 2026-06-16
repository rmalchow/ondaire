#include "i2s_out.h"
#include "wire.h"

#include <string.h>
#include "driver/i2s_std.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "soc/soc_caps.h"

static const char *TAG = "i2s";
static i2s_chan_handle_t tx;
static int g_bclk, g_lrck, g_dout, g_mclk;

static i2s_std_config_t make_cfg(uint32_t hz) {
    i2s_std_config_t cfg = {
        .clk_cfg = I2S_STD_CLK_DEFAULT_CONFIG(hz),
        .slot_cfg = I2S_STD_PHILIPS_SLOT_DEFAULT_CONFIG(I2S_DATA_BIT_WIDTH_16BIT, I2S_SLOT_MODE_STEREO),
        .gpio_cfg = {
            .mclk = (g_mclk >= 0) ? (gpio_num_t)g_mclk : I2S_GPIO_UNUSED,
            .bclk = (gpio_num_t)g_bclk,
            .ws   = (gpio_num_t)g_lrck,
            .dout = (gpio_num_t)g_dout,
            .din  = I2S_GPIO_UNUSED,
            .invert_flags = { .mclk_inv = false, .bclk_inv = false, .ws_inv = false },
        },
    };
    // APLL gives a low-jitter, near-exact 48 kHz where the I2S peripheral has it
    // (ESP32 / S2); the S3's I2S has no APLL, so fall back to the default PLL.
#if SOC_I2S_SUPPORTS_APLL
    cfg.clk_cfg.clk_src = I2S_CLK_SRC_APLL;
#endif
    return cfg;
}

bool i2s_out_init(int bclk, int lrck, int dout, int mclk) {
    g_bclk = bclk; g_lrck = lrck; g_dout = dout; g_mclk = mclk;

    i2s_chan_config_t chan_cfg = I2S_CHANNEL_DEFAULT_CONFIG(I2S_NUM_0, I2S_ROLE_MASTER);
    // Short DMA queue: 6 x 5 ms = 30 ms of output latency, ~matching the ALSA
    // nodes (~36 ms) so L/R line up without huge cross-room equalization. (Was
    // 6 x 20 ms = 120 ms, which made this node play ~90 ms behind the Pis.) The
    // 640 ms PSRAM jitter buffer absorbs network jitter; the DMA only has to cover
    // scheduler hiccups between the audio task's 20 ms writes.
    chan_cfg.dma_desc_num  = I2S_DMA_DESC_NUM;
    chan_cfg.dma_frame_num = I2S_DMA_FRAME_NUM;   // 5 ms per DMA buffer @ 48 kHz
    // On underrun the driver must emit SILENCE, not replay the last DMA buffer —
    // otherwise the DAC loops stale samples as buzzing noise (very audible once the
    // queue is short). auto_clear zero-fills sent buffers so a gap plays as silence.
    chan_cfg.auto_clear = true;
    if (i2s_new_channel(&chan_cfg, &tx, NULL) != ESP_OK) {
        ESP_LOGE(TAG, "i2s_new_channel failed");
        return false;
    }
    i2s_std_config_t cfg = make_cfg(WIRE_SAMPLE_RATE);
    if (i2s_channel_init_std_mode(tx, &cfg) != ESP_OK) {
        ESP_LOGE(TAG, "init_std_mode failed");
        return false;
    }
    if (i2s_channel_enable(tx) != ESP_OK) {
        ESP_LOGE(TAG, "channel_enable failed");
        return false;
    }
    ESP_LOGI(TAG, "I2S up: bclk=%d ws=%d dout=%d mclk=%d", g_bclk, g_lrck, g_dout, g_mclk);
    return true;
}

size_t i2s_out_write(const int16_t *pcm, size_t bytes) {
    size_t wrote = 0;
    if (!tx) return 0;
    i2s_channel_write(tx, pcm, bytes, &wrote, portMAX_DELAY);
    return wrote;
}

bool i2s_out_set_rate_hz(uint32_t hz) {
    if (!tx) return false;
    i2s_std_clk_config_t clk = I2S_STD_CLK_DEFAULT_CONFIG(hz);
#if SOC_I2S_SUPPORTS_APLL
    clk.clk_src = I2S_CLK_SRC_APLL;
#endif
    if (i2s_channel_disable(tx) != ESP_OK) return false;
    bool ok = i2s_channel_reconfig_std_clock(tx, &clk) == ESP_OK;
    i2s_channel_enable(tx);
    return ok;
}

void i2s_out_stop(void) {
    if (!tx) return;
    // Flush a frame of silence so the line settles, then leave the channel up.
    static int16_t zero[WIRE_FRAME_BYTES / 2];
    memset(zero, 0, sizeof zero);
    size_t w = 0;
    i2s_channel_write(tx, zero, sizeof zero, &w, pdMS_TO_TICKS(50));
}
