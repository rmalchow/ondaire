// mdns_adv.h — advertise the _ensemble._tcp playback service with role=playback
// and the capability TXT keys masters browse (PLAYER §5). Never gossips.
#pragma once
#include <stdbool.h>

bool mdns_adv_start(void);   // uses config_get(); call after the netif has an IP
