#include "pcm5122.h"
#include "board.h"

#include "driver/i2c_master.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"

static const char *TAG = "pcm5122";

// PCM512x page-0 registers we touch (datasheet §7.6, "page 0").
#define REG_PAGE      0x00   // page select
#define REG_RESET     0x01   // bit4 RSTR (registers), bit0 RSTM (modules)
#define REG_STANDBY   0x02   // bit4 RQST (standby), bit0 RQPD (power down)
#define REG_MUTE      0x03   // bit4 L mute, bit0 R mute
#define REG_PLL_REF   0x0D   // bits[6:4] SREF: 0=SCK 1=BCK 2=GPIO
#define REG_ERR_DET   0x25   // clock-error ignore flags
#define REG_DAC_ROUTE 0x2A   // DAC data path (vendor init value)
#define REG_VOL_L     0x3D   // digital volume L (0x30 = 0 dB, 0xFF = mute)
#define REG_VOL_R     0x3E   // digital volume R

// Minimal power-on init, matching Sonocotta's field-tested Squeezelite
// `dac_controlset` for this board. The load-bearing write is REG_PLL_REF = 0x10
// (SREF=BCK): with no MCLK/SCK wired, the PLL must reference BCK or the DAC never
// clocks and stays mute. Volume 0x30 = 0 dB; the app still attenuates in software
// (volume.c), so the DAC runs at unity and the encoder/room volume works as usual.
// We wrap the clock config in a standby cycle (TI's recommended order) so the PLL
// re-locks cleanly against the running BCK when we wake it.
typedef struct { uint8_t reg, val; } regval_t;
static const regval_t INIT_SEQ[] = {
    { REG_PAGE,      0x00 },   // select page 0
    { REG_STANDBY,   0x10 },   // hold in standby while we reconfigure clocking
    { REG_PLL_REF,   0x10 },   // PLL reference clock = BCK  (MCLK-less)
    { REG_ERR_DET,   0x08 },   // ignore clock-error detection (no SCK present)
    { REG_DAC_ROUTE, 0x11 },   // DAC data path (vendor value)
    { REG_VOL_L,     0x30 },   // digital volume L = 0 dB
    { REG_VOL_R,     0x30 },   // digital volume R = 0 dB
    { REG_MUTE,      0x00 },   // un-mute both channels
    { REG_STANDBY,   0x00 },   // leave standby -> PLL locks to BCK, DAC plays
};

bool pcm5122_init(int sda, int scl) {
    if (sda < 0 || scl < 0) {
        ESP_LOGW(TAG, "no I2C pins configured (sda=%d scl=%d) — skipping DAC init", sda, scl);
        return false;
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

    // Probe first so a wiring/address problem is a clear log line, not a mystery
    // silence. The board straps the PCM5122 to DEF_DAC_I2C_ADDR.
    if (i2c_master_probe(bus, DEF_DAC_I2C_ADDR, 50) != ESP_OK) {
        ESP_LOGE(TAG, "no ACK from PCM5122 @ 0x%02X on sda=%d scl=%d — check wiring",
                 DEF_DAC_I2C_ADDR, sda, scl);
        i2c_del_master_bus(bus);
        return false;
    }

    i2c_device_config_t dev_cfg = {
        .dev_addr_length = I2C_ADDR_BIT_LEN_7,
        .device_address = DEF_DAC_I2C_ADDR,
        .scl_speed_hz = 100000,
    };
    i2c_master_dev_handle_t dev = NULL;
    if (i2c_master_bus_add_device(bus, &dev_cfg, &dev) != ESP_OK) {
        ESP_LOGE(TAG, "i2c add device failed");
        i2c_del_master_bus(bus);
        return false;
    }

    // Reset to a known state, then apply the init sequence.
    const uint8_t reset[] = { REG_PAGE, 0x00 };
    const uint8_t rst[]   = { REG_RESET, 0x11 };   // reset registers + modules
    i2c_master_transmit(dev, reset, sizeof reset, 50);
    i2c_master_transmit(dev, rst,   sizeof rst,   50);
    vTaskDelay(pdMS_TO_TICKS(10));

    for (size_t i = 0; i < sizeof INIT_SEQ / sizeof INIT_SEQ[0]; i++) {
        uint8_t buf[2] = { INIT_SEQ[i].reg, INIT_SEQ[i].val };
        if (i2c_master_transmit(dev, buf, sizeof buf, 50) != ESP_OK) {
            ESP_LOGE(TAG, "write reg 0x%02X=0x%02X failed", buf[0], buf[1]);
            return false;   // leave the bus up; a partial init is still diagnosable
        }
        if (INIT_SEQ[i].reg == REG_STANDBY) vTaskDelay(pdMS_TO_TICKS(10));
    }

    ESP_LOGI(TAG, "PCM5122 @ 0x%02X initialised (PLL ref=BCK, unmuted, 0 dB) on sda=%d scl=%d",
             DEF_DAC_I2C_ADDR, sda, scl);
    return true;
}
