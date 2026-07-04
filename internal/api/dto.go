package api

import "ondaire/internal/contracts"

// --- GET /api/status (D19) ------------------------------------

// StatusResp is the GET /api/status envelope, pinned to D19. Field order/names
// are part of the wire contract the SPA (J) codes against.
type StatusResp struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Role    string                 `json:"role"`    // "master" | "follower" | "solo"
	GroupID string                 `json:"groupId"` // derived group this node is in
	Ports   PortsResp              `json:"ports"`
	Sink    SinkStatsResp          `json:"sink"`
	Clock   ClockStat              `json:"clock"`
	Source  *contracts.SourceStats `json:"source,omitempty"` // only on an active source
}

// SinkStatsResp is the lowercase-keyed §9.1/D19 projection of
// contracts.SinkStats (which carries no JSON tags — the SPA codes against these
// exact keys). Built from a contracts.SinkStats by sinkStatsResp.
type SinkStatsResp struct {
	Played   uint64  `json:"played"`
	Silence  uint64  `json:"silence"`
	LateDrop uint64  `json:"lateDrop"`
	StaleGen uint64  `json:"staleGen"`
	Synced   bool    `json:"synced"`
	RatePPM  float64 `json:"ratePPM"`
	Buffered int     `json:"buffered"`
}

// sinkStatsResp projects the contract stats onto the pinned wire shape.
func sinkStatsResp(s contracts.SinkStats) SinkStatsResp {
	return SinkStatsResp{
		Played:   s.Played,
		Silence:  s.Silence,
		LateDrop: s.LateDrop,
		StaleGen: s.StaleGen,
		Synced:   s.Synced,
		RatePPM:  s.RatePPM,
		Buffered: s.Buffered,
	}
}

// PortsResp is the actually-bound port set (§2), surfaced as ports.* (D19).
type PortsResp struct {
	HTTP   int `json:"http"`
	Stream int `json:"stream"`
	Source int `json:"source"`
	Gossip int `json:"gossip"`
}

// --- PATCH /api/node -------------------------------------------------------
// All three fields are OPTIONAL; at least one must be present. Pointers
// distinguish "absent" (leave unchanged) from a sent zero value.
type NodePatchReq struct {
	Name             *string                      `json:"name,omitempty"`
	Volume           *float64                     `json:"volume,omitempty"`           // 0.0–1.0 (D35)
	OutputDelayMs    *int                         `json:"outputDelayMs,omitempty"`    // ±500 ms (D36)
	OutputDevice     *string                      `json:"outputDevice,omitempty"`     // ALSA device id (D37)
	Channel          *string                      `json:"channel,omitempty"`          // "stereo" | "L" | "R" (dual-mono)
	Disabled         *[]string                    `json:"disabled,omitempty"`         // subset of {playback,opus,input} (D40)
	SpotifyEndpoints *[]contracts.SpotifyEndpoint `json:"spotifyEndpoints,omitempty"` // Spotify Connect presets (D57)
}

// --- GET /api/media (§6) ---------------------------------------------------

// MediaFile is one playable file, path relative to MEDIA_DIR. The tag-derived
// fields (artist/album/title/duration/hasArt) are populated only when a media
// index is active (§6); they are omitempty so the plain filesystem lister and
// older daemons stay wire-compatible, and every consumer reads keys defensively.
type MediaFile struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
	ModTime     int64  `json:"modTime"` // unix seconds
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	Title       string `json:"title,omitempty"`
	DurationSec int    `json:"durationSec,omitempty"`
	HasArt      bool   `json:"hasArt,omitempty"`
}

// --- POST /api/follow ------------------------------------------------------
type FollowReq struct {
	Target string `json:"target"` // 32-hex node id
}

// --- POST /api/node/forget -------------------------------------------------
// Delete an OFFLINE node from the cluster (tombstone + purge references). The
// receiving master handles it locally and gossips the deletion; it is never
// proxied to the (offline) target.
type ForgetNodeReq struct {
	Target string `json:"target"` // 32-hex node id, or an alive node's unique name
}

// --- POST /api/playback/assign ---------------------------------------------
type AssignPlaybackReq struct {
	Node   string `json:"node"`   // 32-hex playback-node id
	Master string `json:"master"` // 32-hex master id; "" = unassign (idle)
}

// --- POST /api/playback/patch ----------------------------------------------
// Master-side mutation of a non-gossiping playback node's record (D56/D59): any
// subset. A playback node has no HTTP API, so these are NOT proxied to it.
type PatchPlaybackReq struct {
	Node          string   `json:"node"`                    // 32-hex playback-node id
	Name          *string  `json:"name,omitempty"`          // room/speaker label
	Volume        *float64 `json:"volume,omitempty"`        // 0.0–1.0 (D35)
	OutputDelayMs *int     `json:"outputDelayMs,omitempty"` // ±500 (D36)
	Following     *string  `json:"following,omitempty"`     // 32-hex master id; "" = idle
	Channel       *string  `json:"channel,omitempty"`       // "stereo" | "L" | "R" (dual-mono)
}

// --- POST /api/group/name --------------------------------------------------
type GroupNameReq struct {
	Group string `json:"group"` // 32-hex group id
	Name  string `json:"name"`
}

// --- POST /api/stream/presets (cluster-wide HTTP stream presets) -----------
// Create (empty id) or update a named stream preset. Auth is optional; on an
// update a scheme with blank secret keeps the stored secret (write-only UI).
type StreamPresetReq struct {
	ID   string         `json:"id,omitempty"` // empty == create
	Name string         `json:"name"`
	URL  string         `json:"url"`
	Auth *StreamAuthDTO `json:"auth,omitempty"` // nil/"" scheme == no auth
}

// StreamAuthDTO carries optional credentials from the browser. Scheme is "",
// "basic" (User/Pass), or "bearer" (Token).
type StreamAuthDTO struct {
	Scheme string `json:"scheme"`
	User   string `json:"user,omitempty"`
	Pass   string `json:"pass,omitempty"`
	Token  string `json:"token,omitempty"`
}

// --- POST /api/stream/presets/delete ---------------------------------------
type StreamPresetDeleteReq struct {
	ID string `json:"id"`
}

// --- POST /api/play (§6, §9.1) ---------------------------------------------
// Body is {uri}; back-compat {file} folds to a "file:" URI. uri wins.
type PlayReq struct {
	URI  string `json:"uri,omitempty"`
	File string `json:"file,omitempty"`
}

// --- POST /api/calibrate/start (by-ear alignment signal) -------------------
// Plays a synchronized calibration signal to this node's group so the user can
// null the inter-speaker flam via each node's output-delay. Maps to an internal
// "calib:" media source (audio/synthetic.go). Mode is "click" (default) or "noise".
type CalibrateReq struct {
	Mode    string  `json:"mode,omitempty"`    // "click" | "noise"
	ClickHz int     `json:"clickHz,omitempty"` // clicks/sec for click mode (default 2)
	Level   float64 `json:"level,omitempty"`   // 0..1 peak (default 0.5)
}

// --- POST /api/queue (file-source play queue) ------------------------------
// Add one or more file URIs to the END of the queue (the [+] buttons). A bare
// scheme-less path folds to a "file:" URI, like /play.
type QueueAddReq struct {
	URIs []string `json:"uris"`
}

// --- POST /api/queue/remove ------------------------------------------------
// Remove the upcoming item at Index (0 == next). URI, when present, guards
// against an index race with a concurrent snapshot update.
type QueueRemoveReq struct {
	Index int    `json:"index"`
	URI   string `json:"uri,omitempty"`
}

// --- POST /api/queue/play --------------------------------------------------
// Promote the upcoming item at Index (0 == next) to play now: the current track
// is dropped and the promoted item plays immediately, vacating its slot. URI,
// when present, guards against an index race with a concurrent snapshot update.
type QueuePlayReq struct {
	Index int    `json:"index"`
	URI   string `json:"uri,omitempty"`
}

// --- POST /api/seek --------------------------------------------------------
// Jump the current track to PositionSec (seconds from the start). Master only;
// 409 not_seekable when the source can't seek (live/stream/line-in).
type SeekReq struct {
	PositionSec float64 `json:"positionSec"`
}

// --- error envelope (every 4xx/5xx) ----------------------------------------
type ErrorResp struct {
	Error string `json:"error"`          // machine-stable short code
	Hint  string `json:"hint,omitempty"` // human hint (§9.1)
}
