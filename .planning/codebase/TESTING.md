# Testing Patterns

**Analysis Date:** 2026-06-11

## Test Framework

**Go:**
- Runner: `go test` (built-in)
- Framework: Standard `testing` package
- Assertion style: Manual with `t.Errorf()`, `t.Fatalf()`

**JavaScript/Svelte:**
- Runner: Vitest 2.1.0
- Config: `web/vitest.config.js`
- Environment: jsdom (browser-like)
- Assertion Library: Vitest `expect()`

**Run Commands:**

Go:
```bash
go test ./...              # Run all tests
go test -v ./...           # Verbose
go test -cover ./...       # Coverage
go test -run TestName ./pkg  # Single test
```

JavaScript:
```bash
npm test                   # Run all tests (vitest run)
npm run test               # Same
# No watch mode configured; tests run to completion
```

## Test File Organization

**Go:**

Location: Co-located with implementation
- `internal/config/config.go` → `internal/config/config_test.go`
- `internal/id/id.go` → `internal/id/id_test.go`
- `internal/stream/fec.go` → `internal/stream/fec_test.go`

93 test files across the codebase (files: `*_test.go`)

Naming: `TestFunctionName` (matches convention `Test{CamelCase}`)

**JavaScript:**

Location: Co-located with implementation
- `web/src/lib/fmt.js` → `web/src/lib/fmt.test.js`
- `web/src/lib/api.js` → `web/src/lib/api.test.js`
- `web/src/lib/derive.js` → `web/src/lib/derive.test.js`

8 test files in web directory (excluding node_modules)

Naming: `.test.js` suffix

## Test Structure

**Go Suite Organization:**

No test frameworks (Jest, testify, etc.); tests are single functions using `testing.T`.

```go
func TestLoadDefaults(t *testing.T) {
    dir := t.TempDir()
    cfg, err := Load(Options{
        Args:   nil,
        Getenv: envMap(map[string]string{EnvDataDir: dir}),
    })
    if err != nil {
        t.Fatalf("Load: %v", err)
    }
    if cfg.HTTPPort != DefaultHTTPPort {
        t.Errorf("http = %d, want %d", cfg.HTTPPort, DefaultHTTPPort)
    }
}
```

Patterns:
- **Arrange-Act-Assert:** Setup (e.g., `t.TempDir()`), call function, check result
- **Early fatality:** `t.Fatalf()` for fatal setup errors; `t.Errorf()` for assertion failures (allows multiple assertions per test)
- **Subtests:** Some tests use `t.Run()` for grouped assertions (observed in `config_test.go`, `stream/client_test.go`)

Helpers marked with `t.Helper()`:
```go
func newLoopbackMux(t *testing.T) *Mux {
    t.Helper()
    // ...
}
```

**JavaScript Suite Organization:**

Uses Vitest `describe()` for grouping, `it()` for individual tests.

```javascript
describe("shortId", () => {
  it("returns first 8 chars", () => {
    expect(shortId("0123456789abcdef0123456789abcdef")).toBe("01234567");
  });
  it("tolerates short input", () => {
    expect(shortId("ab")).toBe("ab");
    expect(shortId("")).toBe("");
    expect(shortId(undefined)).toBe("");
  });
});
```

Patterns:
- **Describe blocks:** Group related tests by function or feature
- **Clear test names:** "returns first 8 chars", "tolerates short input" describe the specific behavior
- **Arrange-Act-Assert:** Setup (mocking, constructing test data), call function, check with `expect()`

Multi-scenario tests group assertions:
```javascript
describe("relTime", () => {
  const now = Math.floor(Date.now() / 1000);
  it("just now", () => expect(relTime(now)).toBe("just now"));
  it("seconds", () => expect(relTime(now - 12)).toBe("12s ago"));
  it("minutes", () => expect(relTime(now - 180)).toBe("3m ago"));
  // ...
});
```

## Mocking

**Go:**

Mocking via dependency injection (no mocking library detected):
```go
// envMap returns a Getenv func backed by m.
func envMap(m map[string]string) func(string) string {
    return func(k string) string { return m[k] }
}

func TestLoadEnvFallback(t *testing.T) {
    cfg, err := Load(Options{
        Getenv: envMap(map[string]string{EnvDataDir: dir, EnvStreamPort: "9100"}),
    })
    // ...
}
```

Fake types for network testing:
```go
type fakeSource struct {
    conn *net.UDPConn
    addr netip.AddrPort
}

func newFakeSource(t *testing.T) *fakeSource { /* ... */ }

func (f *fakeSource) sendAudio(to netip.AddrPort, gen uint32, seq uint64, pts int64, pay []byte) {
    // writes UDP packets to mux under test
}
```

Helper collectors for assertions:
```go
type collector struct {
    mu    sync.Mutex
    heads []Header
    pays  [][]byte
}

func (c *collector) deliver(h Header, p []byte) {
    c.mu.Lock()
    defer c.mu.Unlock()
    // collect delivered frames
}
```

**JavaScript:**

Vitest `vi` for mocking:
```javascript
import { describe, it, expect, vi, beforeEach } from "vitest";

function mockFetch(status, body) {
  return vi.fn(async () => ({
    ok: status >= 200 && status < 300,
    status,
    statusText: "x",
    text: async () => (body === undefined ? "" : JSON.stringify(body)),
  }));
}

beforeEach(() => {
  setSelfId(SELF);
});

it("renameNode remote → PATCH /api/<remote>/node {name}", async () => {
  global.fetch = mockFetch(200, {});
  await renameNode(REMOTE, "kitchen");
  const [path, opts] = global.fetch.mock.calls[0];
  expect(path).toBe("/api/" + REMOTE + "/node");
  expect(opts.method).toBe("PATCH");
});
```

Patterns:
- `vi.fn()` creates spy functions that track calls and return values
- `global.fetch` replaced with mock for network tests
- `.mock.calls` inspects call arguments
- `expect().toHaveBeenCalledTimes()` asserts call count

Snapshot-like test data:
```javascript
function snap() {
  const caps = { capabilities: { playback: true } };
  return {
    nodes: [
      { id: A, name: "alice", alive: true, following: ZERO_ID, ...caps },
      { id: B, name: "bob", alive: true, following: A, ...caps },
      { id: C, name: "carol", alive: true, following: ZERO_ID, ...caps },
    ],
    groups: [],
  };
}
```

**What to Mock:**
- Network calls (fetch) via `vi.fn()` or `global.fetch` replacement
- External dependencies passed as parameters (Go: `Getenv` function)
- Fake implementations of network sources (Go: `fakeSource` for UDP)

**What NOT to Mock:**
- Pure functions (formatters, derivers) — test with real inputs/outputs
- Domain logic (group resolution, ID XOR) — test directly
- File I/O in Go tests — use `t.TempDir()` for real temp files

## Fixtures and Factories

**Go Test Data:**

Helper constructors:
```go
// nodeRec constructs a node record for testing.
func nodeRec(id id.ID, version int, name string) NodeRecord {
    return NodeRecord{ID: id, Version: int32(version), Name: name}
}
```

Temporary directories:
```go
func TestLoadDefaults(t *testing.T) {
    dir := t.TempDir()  // automatically cleaned up after test
    cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
}
```

**JavaScript Test Data:**

Factory functions (snap(), view()):
```javascript
function snap() {
  const caps = { capabilities: { playback: true } };
  return {
    nodes: [
      { id: A, name: "alice", alive: true, following: ZERO_ID, ...caps },
      { id: B, name: "bob", alive: true, following: A, ...caps },
      { id: C, name: "carol", alive: true, following: ZERO_ID, ...caps },
    ],
    groups: [],
  };
}

describe("nodeById", () => {
  it("finds and misses", () => {
    expect(nodeById(snap(), A).name).toBe("alice");
    expect(nodeById(snap(), "zzzz")).toBeUndefined();
  });
});
```

Constants for repeated IDs:
```javascript
const A = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const B = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const C = "cccccccccccccccccccccccccccccccc";
const SELF = "11111111111111111111111111111111";
const REMOTE = "22222222222222222222222222222222";
```

Location: Inline in test files (no separate fixtures directory)

## Coverage

**Go:**
- No coverage requirements enforced
- Run with `go test -cover ./...`

**JavaScript:**
- No coverage requirements enforced or configured in vitest.config.js
- View coverage: Not instrumented (no `--coverage` flag in vitest.config.js)

## Test Types

**Go:**

Unit Tests (vast majority):
- Package-focused (e.g., `internal/id/id_test.go` tests only the id package)
- No database, no sockets, no goroutines in basic unit tests
- Input validation, parsing, transformation

Example: `internal/config/config_test.go`
```go
func TestLoadDefaults(t *testing.T) { /* test config resolution */ }
func TestLoadFlagsOverrideEnv(t *testing.T) { /* test precedence */ }
func TestLoadEnvFallback(t *testing.T) { /* test fallback */ }
```

Integration-style Tests (network/cluster modules):
- `internal/stream/client_test.go`: Loopback UDP sockets, real network I/O
- `internal/cluster/delegate_test.go`: Encoded/decoded deltas, gossip message handling
- `internal/stream/fec_test.go`: FEC block construction and recovery

Example: `internal/stream/client_test.go`
```go
func newLoopbackMux(t *testing.T) *Mux {
    t.Helper()
    uc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
    // ...
}
```

**JavaScript:**

Unit Tests (all vitest tests):
- Pure function tests (formatters, derivers)
- API request/response handling
- State derivation

Example: `web/src/lib/fmt.test.js`
- Tests `shortId()`, `relTime()`, `position()`, `bytes()`, `cidrList()`, `ports()` formatters
- No DOM interaction, no component rendering

Example: `web/src/lib/api.test.js`
- Tests HTTP request construction
- Mocks fetch, asserts request paths/bodies/methods
- Error handling (ApiError throws)

Example: `web/src/lib/derive.test.js`
- Tests pure view-selectors over Snapshot
- No component state, no side effects

E2E Tests:
- Not detected in current test suite
- No Playwright/Cypress config

## Common Patterns

**Async Testing (Go):**

Goroutines and channels:
```go
// In cluster tests, collect async broadcasts
func TestBroadcastInvalidates(t *testing.T) {
    n := id.New()
    b1 := &broadcast{key: broadcastKey(kindNodeDelta, n)}
    b2 := &broadcast{key: broadcastKey(kindNodeDelta, n)}
    b3 := &broadcast{key: broadcastKey(kindNodeDelta, id.New())}
    if !b1.Invalidates(b2) {
        t.Fatal("same key should invalidate")
    }
}
```

Time-based operations:
```go
func (f *fakeSource) readControl(t *testing.T, timeout time.Duration) (Header, []byte, netip.AddrPort) {
    t.Helper()
    f.conn.SetReadDeadline(time.Now().Add(timeout))
    buf := make([]byte, 2048)
    n, from, err := f.conn.ReadFromUDPAddrPort(buf)
    if err != nil {
        t.Fatalf("readControl: %v", err)
    }
    // ...
}
```

**Async Testing (JavaScript):**

Async/await with expect:
```javascript
it("renameNode remote → PATCH /api/<remote>/node {name}", async () => {
  global.fetch = mockFetch(200, {});
  await renameNode(REMOTE, "kitchen");
  const [path, opts] = global.fetch.mock.calls[0];
  expect(path).toBe("/api/" + REMOTE + "/node");
});

it("non-2xx with {error} throws ApiError", async () => {
  global.fetch = mockFetch(409, { error: "not a master" });
  await expect(play(REMOTE, "file:x")).rejects.toMatchObject({
    status: 409,
    message: "not a master",
  });
});
```

**Error Testing:**

Go:
```go
func TestParseRejectsBadLength(t *testing.T) {
    for _, s := range []string{"", "abcd", strings.Repeat("a", 31), ...} {
        if _, err := Parse(s); err != ErrBadLength {
            t.Fatalf("Parse(%q): want ErrBadLength, got %v", s, err)
        }
    }
}
```

JavaScript:
```javascript
it("non-2xx with {error} throws ApiError carrying status+message", async () => {
  global.fetch = mockFetch(409, { error: "not a master" });
  await expect(play(REMOTE, "file:x")).rejects.toMatchObject({
    status: 409,
    message: "not a master",
  });
});

it("ApiError instanceof Error", async () => {
  global.fetch = mockFetch(500, { message: "boom" });
  let err;
  try {
    await unfollow(REMOTE);
  } catch (e) {
    err = e;
  }
  expect(err).toBeInstanceOf(ApiError);
  expect(err.message).toBe("boom");
});
```

---

*Testing analysis: 2026-06-11*
