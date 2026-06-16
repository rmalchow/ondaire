// DAC wiring bench test for the ESP32-S3 Super Mini → PCM5102A.
//
// Cycles a 480 Hz tone through LEFT-only, RIGHT-only, BOTH, then silence — so
// you can confirm both line-out channels are alive and not swapped. Uses the
// same I2S setup as the ensemble player: Philips/I2S, 16-bit stereo, 48 kHz,
// MCLK off (PCM5102A onboard PLL), APLL when the silicon has it (the S3 doesn't).
//
// Default pins = board_esp32s3_supermini.h:  BCK=GPIO5, LCK=GPIO6, DIN=GPIO7.

#include <math.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "driver/i2s_std.h"
#include "esp_log.h"
#include "soc/soc_caps.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

#define PIN_BCLK 5
#define PIN_WS   6
#define PIN_DOUT 7
#define SR       48000
#define TONE_HZ  480          // divides 48 kHz evenly → 100-sample period
#define AMP      8000         // ~ -12 dBFS, safe line level

static const char *TAG = "dac-tone";
static i2s_chan_handle_t tx;

static void i2s_up(void) {
    i2s_chan_config_t cc = I2S_CHANNEL_DEFAULT_CONFIG(I2S_NUM_0, I2S_ROLE_MASTER);
    cc.dma_desc_num  = 6;
    cc.dma_frame_num = 240;
    ESP_ERROR_CHECK(i2s_new_channel(&cc, &tx, NULL));

    i2s_std_config_t cfg = {
        .clk_cfg  = I2S_STD_CLK_DEFAULT_CONFIG(SR),
        .slot_cfg = I2S_STD_PHILIPS_SLOT_DEFAULT_CONFIG(I2S_DATA_BIT_WIDTH_16BIT, I2S_SLOT_MODE_STEREO),
        .gpio_cfg = {
            .mclk = I2S_GPIO_UNUSED,
            .bclk = PIN_BCLK,
            .ws   = PIN_WS,
            .dout = PIN_DOUT,
            .din  = I2S_GPIO_UNUSED,
            .invert_flags = { .mclk_inv = false, .bclk_inv = false, .ws_inv = false },
        },
    };
#if SOC_I2S_SUPPORTS_APLL
    cfg.clk_cfg.clk_src = I2S_CLK_SRC_APLL;
#endif
    ESP_ERROR_CHECK(i2s_channel_init_std_mode(tx, &cfg));
    ESP_ERROR_CHECK(i2s_channel_enable(tx));
    ESP_LOGI(TAG, "I2S up: bclk=%d ws=%d dout=%d mclk=off", PIN_BCLK, PIN_WS, PIN_DOUT);
}

// Emit `ms` of the tone, gating left/right so you can tell the channels apart.
static void emit(int ms, int left_on, int right_on) {
    static int phase = 0;                 // sample index within one period
    const int period = SR / TONE_HZ;      // 100 samples
    const int total  = (int)((int64_t)SR * ms / 1000);
    int16_t buf[480];                     // 240 stereo frames
    int done = 0;
    while (done < total) {
        int frames = (total - done < 240) ? (total - done) : 240;
        for (int i = 0; i < frames; i++) {
            int16_t v = (int16_t)(AMP * sin(2.0 * M_PI * phase / period));
            buf[2 * i]     = left_on  ? v : 0;
            buf[2 * i + 1] = right_on ? v : 0;
            phase = (phase + 1) % period;
        }
        size_t w = 0;
        i2s_channel_write(tx, buf, (size_t)frames * 2 * sizeof(int16_t), &w, portMAX_DELAY);
        done += frames;
    }
}

void app_main(void) {
    ESP_LOGI(TAG, "DAC tone test — %d Hz, %d Hz stereo, MCLK off", TONE_HZ, SR);
    ESP_LOGI(TAG, "wiring: VIN<-3V3 GND<-GND SCK<-GND BCK<-GPIO5 LCK<-GPIO6 DIN<-GPIO7 (XSMT high, FMT low)");
    i2s_up();
    for (;;) {
        ESP_LOGI(TAG, "LEFT only");  emit(1000, 1, 0);
        ESP_LOGI(TAG, "RIGHT only"); emit(1000, 0, 1);
        ESP_LOGI(TAG, "BOTH");       emit(1000, 1, 1);
        ESP_LOGI(TAG, "silence");    emit(500,  0, 0);
    }
}
