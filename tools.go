//go:build tools

// This file blank-imports every direct dependency so `go mod tidy` keeps them
// in go.mod even while some are only used by later pieces' not-yet-written
// code. It is never compiled into the binary (the `tools` build tag is never
// set for normal builds). Do not remove imports here without removing the dep.
package tools

import (
	_ "github.com/ebitengine/purego"
	_ "github.com/go-audio/wav"
	_ "github.com/gorilla/websocket"
	_ "github.com/grandcat/zeroconf"
	_ "github.com/hajimehoshi/go-mp3"
	_ "github.com/hashicorp/memberlist"
	_ "github.com/labstack/echo/v4"
	_ "github.com/mewkiz/flac"
)
