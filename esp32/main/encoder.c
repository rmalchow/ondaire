#include "encoder.h"
#include "volume.h"
#include "config.h"

#include "driver/gpio.h"
#include "driver/pulse_cnt.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "enc";

#define COUNTS_PER_DETENT  4     // 4x quadrature → one KY-040 detent ≈ 4 counts
#define VOL_PER_DETENT     3     // volume % per detent

static pcnt_unit_handle_t s_unit;
static int s_sw;

static void enc_task(void *arg) {
    (void)arg;
    int last = 0, residual = 0;
    int sw_stable = 1, sw_cnt = 0;
    pcnt_unit_get_count(s_unit, &last);
    for (;;) {
        vTaskDelay(pdMS_TO_TICKS(20));

        int cur = 0;
        pcnt_unit_get_count(s_unit, &cur);
        int delta = cur - last + residual;
        last = cur;
        int detents = delta / COUNTS_PER_DETENT;
        residual = delta - detents * COUNTS_PER_DETENT;
        if (detents != 0) {
            int v = (int)volume_get() + detents * VOL_PER_DETENT;
            if (v < 0) v = 0;
            if (v > 100) v = 100;
            volume_set((uint8_t)v, false);     // turning the knob also unmutes
        }

        // Debounced push switch (active-low) → toggle mute on a clean press.
        if (s_sw >= 0) {
            int level = gpio_get_level((gpio_num_t)s_sw);
            if (level != sw_stable) {
                if (++sw_cnt >= 2) { sw_stable = level; sw_cnt = 0;
                    if (level == 0) volume_set(volume_get(), !volume_muted()); }
            } else {
                sw_cnt = 0;
            }
        }
    }
}

bool encoder_init(int pin_a, int pin_b, int pin_sw) {
    s_sw = pin_sw;

    pcnt_unit_config_t ucfg = { .high_limit = 1000, .low_limit = -1000 };
    if (pcnt_new_unit(&ucfg, &s_unit) != ESP_OK) { ESP_LOGE(TAG, "pcnt unit"); return false; }
    pcnt_glitch_filter_config_t gf = { .max_glitch_ns = 1000 };
    pcnt_unit_set_glitch_filter(s_unit, &gf);

    pcnt_chan_config_t ca = { .edge_gpio_num = pin_a, .level_gpio_num = pin_b };
    pcnt_channel_handle_t cha;
    if (pcnt_new_channel(s_unit, &ca, &cha) != ESP_OK) { ESP_LOGE(TAG, "pcnt ch a"); return false; }
    pcnt_channel_set_edge_action(cha, PCNT_CHANNEL_EDGE_ACTION_DECREASE, PCNT_CHANNEL_EDGE_ACTION_INCREASE);
    pcnt_channel_set_level_action(cha, PCNT_CHANNEL_LEVEL_ACTION_KEEP, PCNT_CHANNEL_LEVEL_ACTION_INVERSE);

    pcnt_chan_config_t cb = { .edge_gpio_num = pin_b, .level_gpio_num = pin_a };
    pcnt_channel_handle_t chb;
    if (pcnt_new_channel(s_unit, &cb, &chb) != ESP_OK) { ESP_LOGE(TAG, "pcnt ch b"); return false; }
    pcnt_channel_set_edge_action(chb, PCNT_CHANNEL_EDGE_ACTION_INCREASE, PCNT_CHANNEL_EDGE_ACTION_DECREASE);
    pcnt_channel_set_level_action(chb, PCNT_CHANNEL_LEVEL_ACTION_KEEP, PCNT_CHANNEL_LEVEL_ACTION_INVERSE);

    pcnt_unit_enable(s_unit);
    pcnt_unit_clear_count(s_unit);
    pcnt_unit_start(s_unit);

    if (pin_sw >= 0) {
        gpio_config_t io = {
            .pin_bit_mask = 1ULL << pin_sw,
            .mode = GPIO_MODE_INPUT,
            .pull_up_en = GPIO_PULLUP_ENABLE,   // KY-040 also has an onboard pull-up
        };
        gpio_config(&io);
    }
    xTaskCreate(enc_task, "encoder", 2560, NULL, 8, NULL);
    ESP_LOGI(TAG, "encoder up: a=%d b=%d sw=%d", pin_a, pin_b, pin_sw);
    return true;
}
