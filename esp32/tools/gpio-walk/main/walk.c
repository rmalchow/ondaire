// GPIO continuity walk for the ensemble DAC wiring.
//
// Drives the three I2S pins high ONE AT A TIME, for distinct durations, in a
// loop, so you can probe the DAC's input pads with a multimeter on DC volts
// (referenced to GND) and watch ~3.3 V appear on exactly one pad at a time:
//
//   GP5 -> DAC BCK : HIGH for 1 s
//   GP6 -> DAC LCK : HIGH for 2 s
//   GP7 -> DAC DIN : HIGH for 3 s
//   (0.5 s all-low gap between each, and all-low at the start of each cycle)
//
// If a DAC pad never reads ~3.3 V during its window, that wire isn't carrying
// the signal (cold joint, wrong pad, or break). Same pins as board_esp32s3_zero.h.
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"

static const char *TAG = "gpio-walk";

#define PIN_BCLK 5   // -> DAC BCK
#define PIN_WS   6   // -> DAC LCK
#define PIN_DOUT 7   // -> DAC DIN

static void hold(const char *label, int pin, int secs)
{
    ESP_LOGI(TAG, "GP%d (-> DAC %s) HIGH for %d s", pin, label, secs);
    gpio_set_level(pin, 1);
    vTaskDelay(pdMS_TO_TICKS(secs * 1000));
    gpio_set_level(pin, 0);
    vTaskDelay(pdMS_TO_TICKS(500));   // all-low gap
}

void app_main(void)
{
    gpio_config_t io = {
        .pin_bit_mask = (1ULL << PIN_BCLK) | (1ULL << PIN_WS) | (1ULL << PIN_DOUT),
        .mode         = GPIO_MODE_OUTPUT,
        .pull_up_en   = GPIO_PULLUP_DISABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type    = GPIO_INTR_DISABLE,
    };
    gpio_config(&io);
    gpio_set_level(PIN_BCLK, 0);
    gpio_set_level(PIN_WS, 0);
    gpio_set_level(PIN_DOUT, 0);

    ESP_LOGI(TAG, "GPIO walk: meter DAC BCK/LCK/DIN to GND on DC volts.");
    ESP_LOGI(TAG, "Expect ~3.3 V on one pad at a time: BCK 1s, LCK 2s, DIN 3s.");

    while (1) {
        hold("BCK", PIN_BCLK, 1);
        hold("LCK", PIN_WS,   2);
        hold("DIN", PIN_DOUT, 3);
    }
}
