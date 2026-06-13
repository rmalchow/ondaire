// net_audio.h — the audio/clock data plane (PLAYER §7-§8). Owns one
// ephemeral udp4 socket used for HELLO/BYE/RESTART, CLOCK_REQ/RSP, and inbound
// AUDIO/FEC/RECONFIG, so the master streams audio back to our HELLO source addr.
// On TCP transport, audio rides a dialed TCP connection; the clock stays on UDP.
#pragma once
#include <stdbool.h>
#include <stdint.h>

bool net_audio_init(void);   // create socket, spawn read + clock + keepalive tasks

// (Re)subscribe to a master (from ATTACH). ips/ports are host byte order.
void net_audio_attach(uint32_t src_ip, uint16_t src_port,
                      uint32_t clk_ip, uint16_t clk_port,
                      uint8_t codec, uint8_t transport, uint16_t buffer_ms);

// Leave: send BYE, stop the player, go idle.
void net_audio_detach(void);
