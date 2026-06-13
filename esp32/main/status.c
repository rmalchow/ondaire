#include "status.h"
#include "wire.h"
#include "config.h"
#include "player.h"

#include <string.h>

int status_build(uint8_t *out, size_t cap) {
    wire_status_t s;
    memset(&s, 0, sizeof s);
    memcpy(s.node_id, config_get()->node_id, 16);
    player_fill_status(&s);
    return wire_status_encode(out, cap, &s);
}
