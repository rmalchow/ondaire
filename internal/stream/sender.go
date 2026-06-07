package stream

// The audio source server and subscriber transport (HELLO/keepalive/RESTART,
// UDP+FEC / TCP reception) live here and in receiver.go. Implemented by piece
// G; this is the wave-0 stub. wire.go and mux.go in this package are S-owned
// contracts and read-only to G.
