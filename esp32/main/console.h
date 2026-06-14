// console.h — line-delimited JSON provisioning protocol over the USB-CDC console
// (docs/developer/esp32.md §6.2). The web flasher's "device settings" panel drives this to
// write Wi-Fi creds + I2S/encoder/DAC config and run a test tone. Also scriptable.
//
//   →  {"cmd":"get"}
//   ←  {"ok":true,"cfg":{...}}
//   →  {"cmd":"set","cfg":{"wifi_ssid":"...","i2s_dout":37,...}}
//   ←  {"ok":true}                       # validated + written to NVS
//   →  {"cmd":"test","what":"tone"}       # 1 kHz on the configured I2S
//   →  {"cmd":"reboot"}
#pragma once
#include <stdbool.h>

bool console_init(void);
