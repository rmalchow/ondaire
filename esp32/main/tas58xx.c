#include "tas58xx.h"
#include "board.h"

#include "driver/i2c_master.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"

static const char *TAG = "tas58xx";

// Meta-opcode used in INIT_SEQ below: not a real register — `val` is a delay in ms.
#define TAS_DELAY 0xFE

// TAS5805M/TAS5825M page-0 registers we touch (TI datasheet §7.6). Paging: the parts
// are organised into books (0x7F) and pages (0x00); all of this init lives in
// book 0 / page 0 except one page-1 vendor write.
#define REG_PAGE      0x00   // page select
#define REG_RESET     0x01   // reset (0x11 = reset core + registers)
#define REG_DEV_CTRL1 0x02   // BTL/PBTL, modulation
#define REG_DEV_CTRL2 0x03   // CTRL_STATE: 0=deep-sleep 2=Hi-Z 3=play; bit3 = mute
#define REG_DIG_VOL   0x4C   // digital volume (0x30 = 0 dB, -0.5 dB/LSB)
#define REG_AGAIN     0x54   // analog gain (0x00 = 0 dB)
#define REG_FAULT_CLR 0x78   // clear analog faults (0x80)
#define REG_BOOK      0x7F   // book select

typedef struct { uint8_t reg, val; } regval_t;

// Minimal power-on init for plain stereo I2S passthrough into the speakers: BTL,
// normal modulation, unity (0 dB) digital + analog gain, unmuted. Distilled from
// Sonocotta's field-tested tas58xx ESPHome driver (github://mrtoy-me/esphome-tas58xx,
// TAS58XX_CONFIG[]). The chip's EQ/DSP biquad banks (separate coefficient dumps in
// the driver) are intentionally omitted — we want a flat passthrough. The registers
// marked "vendor" (0x46/0x60/0x61/0x7D/0x7E, and 0x30/0x51/0x53) carry no datasheet
// meaning in the driver source; they are opaque bring-up values copied verbatim.
// Identical for the TAS5805M and TAS5825M — only the I2C address differs.
static const regval_t INIT_SEQ[] = {
    { REG_PAGE,      0x00 },  // page 0
    { REG_BOOK,      0x00 },  // book 0
    { REG_DEV_CTRL2, 0x02 },  // Hi-Z
    { REG_RESET,     0x11 },  // reset digital core + registers
    { REG_DEV_CTRL2, 0x02 },  // Hi-Z
    { TAS_DELAY,     0x05 },  // >>> wait 5 ms for the reset to settle
    { REG_DEV_CTRL2, 0x00 },  // deep sleep
    { 0x46,          0x11 },  // vendor clock/SR config (0x11 = 48 kHz per driver)
    { REG_DEV_CTRL2, 0x02 },  // Hi-Z
    { 0x61,          0x0B },  // vendor bring-up (opaque)
    { 0x60,          0x01 },  // vendor bring-up (opaque)
    { 0x7D,          0x11 },  // vendor bring-up (opaque)
    { 0x7E,          0xFF },  // vendor bring-up (opaque)
    { REG_PAGE,      0x01 },  // page 1
    { 0x51,          0x05 },  // vendor bring-up, page 1 (opaque)
    { REG_PAGE,      0x00 },  // page 0
    { REG_BOOK,      0x00 },  // book 0
    { REG_DEV_CTRL1, 0x00 },  // BTL, normal modulation, DSP not held in reset
    { 0x30,          0x00 },  // vendor SAP/data-format default (opaque)
    { REG_DIG_VOL,   0x30 },  // digital volume 0 dB (unity — sw volume in volume.c)
    { 0x53,          0x00 },  // vendor default (opaque)
    { REG_AGAIN,     0x00 },  // analog gain 0 dB (no attenuation)
    { REG_DEV_CTRL2, 0x03 },  // PLAY, unmuted (bit3 = 0)
    { REG_FAULT_CLR, 0x80 },  // clear analog faults
};

bool tas58xx_init(int sda, int scl, int en_pin) {
    if (sda < 0 || scl < 0) {
        ESP_LOGW(TAG, "no I2C pins configured (sda=%d scl=%d) — skipping DAC init", sda, scl);
        return false;
    }

    // Device PDN/enable: TI power-up timing is LOW → 1 ms → HIGH → 5 ms before I2C.
    if (en_pin >= 0) {
        gpio_config_t io = { .pin_bit_mask = 1ULL << en_pin, .mode = GPIO_MODE_OUTPUT };
        gpio_config(&io);
        gpio_set_level((gpio_num_t)en_pin, 0);
        vTaskDelay(pdMS_TO_TICKS(1));
        gpio_set_level((gpio_num_t)en_pin, 1);
        vTaskDelay(pdMS_TO_TICKS(5));
    }

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
        ESP_LOGE(TAG, "no ACK from TAS58xx @ 0x%02X on sda=%d scl=%d — check wiring",
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
        if (INIT_SEQ[i].reg == TAS_DELAY) {
            vTaskDelay(pdMS_TO_TICKS(INIT_SEQ[i].val));
            continue;
        }
        uint8_t buf[2] = { INIT_SEQ[i].reg, INIT_SEQ[i].val };
        if (i2c_master_transmit(dev, buf, sizeof buf, 50) != ESP_OK) {
            ESP_LOGE(TAG, "write reg 0x%02X=0x%02X failed", buf[0], buf[1]);
            return false;   // leave the bus up; a partial init is still diagnosable
        }
    }

    ESP_LOGI(TAG, "TAS58xx @ 0x%02X initialised (BTL, 0 dB, playing) on sda=%d scl=%d",
             DEF_DAC_I2C_ADDR, sda, scl);
    return true;
}
