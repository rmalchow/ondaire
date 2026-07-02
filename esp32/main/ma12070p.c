#include "ma12070p.h"
#include "board.h"

#include "driver/i2c_master.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"

static const char *TAG = "ma12070p";

// MA12070P registers we touch (Infineon datasheet). The part comes up on its
// power-on register defaults for the class-D stage; we only set the audio format
// and reset the error handler, then unmute in-register via master volume.
#define REG_AUDIO_FMT 0x35   // I2S format select + audio-proc enable
#define REG_EH_CFG    0x2D   // error-handler config / clear
#define REG_VOL       0x40   // master volume (0x18 = 0 dB, -1 dB/LSB)

typedef struct { uint8_t reg, val; } regval_t;

// Minimal bring-up (from Sonocotta's ma12070p ESPHome driver,
// github://sonocotta/esphome-ma12070p, ma12070p_init_seq[]): select I2S format with
// audio processing enabled, pulse the error handler clear, then set master volume to
// unity. Volume/mute otherwise stay at the part's power-on defaults; software volume
// (volume.c) does the actual attenuation, so we sit at 0 dB.
static const regval_t INIT_SEQ[] = {
    { REG_AUDIO_FMT, 0x08 },  // I2S format (bits[2:0]=0) + audio_proc_enable (bit3)
    { REG_EH_CFG,    0x00 },  // error handler: clear accumulated errors
    { REG_EH_CFG,    0x04 },  // error handler: pulse clear
    { REG_EH_CFG,    0x00 },  // error handler: back to baseline
    { REG_VOL,       0x18 },  // master volume 0 dB (unity — sw volume in volume.c)
};

bool ma12070p_init(int sda, int scl, int en_pin) {
    if (sda < 0 || scl < 0) {
        ESP_LOGW(TAG, "no I2C pins configured (sda=%d scl=%d) — skipping DAC init", sda, scl);
        return false;
    }

    // Power-up: mute LOW (muted) + enable LOW (active), then wait for PVDD to settle.
    if (DEF_MA_MUTE >= 0) {
        gpio_config_t m = { .pin_bit_mask = 1ULL << DEF_MA_MUTE, .mode = GPIO_MODE_OUTPUT };
        gpio_config(&m);
        gpio_set_level((gpio_num_t)DEF_MA_MUTE, 0);   // muted while we configure
    }
    if (en_pin >= 0) {
        gpio_config_t e = { .pin_bit_mask = 1ULL << en_pin, .mode = GPIO_MODE_OUTPUT };
        gpio_config(&e);
        gpio_set_level((gpio_num_t)en_pin, 0);        // enable is active-LOW
    }
    vTaskDelay(pdMS_TO_TICKS(100));   // let PVDD stabilise before I2C

    i2c_master_bus_config_t bus_cfg = {
        .i2c_port = I2C_NUM_0,
        .sda_io_num = (gpio_num_t)sda,
        .scl_io_num = (gpio_num_t)scl,
        .clk_source = I2C_CLK_SRC_DEFAULT,
        .glitch_ignore_cnt = 7,
        .flags.enable_internal_pullup = true,
    };
    i2c_master_bus_handle_t bus = NULL;
    if (i2c_new_master_bus(&bus_cfg, &bus) != ESP_OK) {
        ESP_LOGE(TAG, "i2c bus init failed (sda=%d scl=%d)", sda, scl);
        return false;
    }

    if (i2c_master_probe(bus, DEF_DAC_I2C_ADDR, 50) != ESP_OK) {
        ESP_LOGE(TAG, "no ACK from MA12070P @ 0x%02X on sda=%d scl=%d — check wiring",
                 DEF_DAC_I2C_ADDR, sda, scl);
        i2c_del_master_bus(bus);
        return false;
    }

    i2c_device_config_t dev_cfg = {
        .dev_addr_length = I2C_ADDR_BIT_LEN_7,
        .device_address = DEF_DAC_I2C_ADDR,
        .scl_speed_hz = 400000,   // Sonocotta runs this bus at 400 kHz
    };
    i2c_master_dev_handle_t dev = NULL;
    if (i2c_master_bus_add_device(bus, &dev_cfg, &dev) != ESP_OK) {
        ESP_LOGE(TAG, "i2c add device failed");
        i2c_del_master_bus(bus);
        return false;
    }

    for (size_t i = 0; i < sizeof INIT_SEQ / sizeof INIT_SEQ[0]; i++) {
        uint8_t buf[2] = { INIT_SEQ[i].reg, INIT_SEQ[i].val };
        if (i2c_master_transmit(dev, buf, sizeof buf, 50) != ESP_OK) {
            ESP_LOGE(TAG, "write reg 0x%02X=0x%02X failed", buf[0], buf[1]);
            return false;
        }
    }

    if (DEF_MA_MUTE >= 0) gpio_set_level((gpio_num_t)DEF_MA_MUTE, 1);   // unmute

    ESP_LOGI(TAG, "MA12070P @ 0x%02X initialised (I2S, 0 dB, unmuted) on sda=%d scl=%d",
             DEF_DAC_I2C_ADDR, sda, scl);
    return true;
}
