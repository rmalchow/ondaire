// encoder.h — KY-040/EC11 rotary encoder: PCNT quadrature for local volume,
// debounced push switch for mute (PLAYER-adjacent local control, esp32.md §4).
#pragma once
#include <stdbool.h>

bool encoder_init(int pin_a, int pin_b, int pin_sw);
