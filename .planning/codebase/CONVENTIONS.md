# Coding Conventions

**Analysis Date:** 2026-06-11

## Naming Patterns

**Go Files:**
- Package-private helpers prefixed with lowercase: `newLoopbackMux()`, `nodeRec()`, `fakeSource`
- Test helpers marked with `t.Helper()` to exclude them from call stack
- Constants in PascalCase (exported) or lowercase (package-private): `DefaultHTTPPort`, `EnvHTTPPort`, `Magic`, `TypeAudio`
- Error variables prefixed with `Err`: `ErrBadLength` (exported error sentinel)
- Types in PascalCase: `Config`, `ID`, `Cluster`, `GroupNameRecord`
- Interface-like receiver methods: `MarshalText()`, `UnmarshalText()`, `String()`, `IsZero()`

**Go Functions:**
- Exported functions PascalCase: `New()`, `Parse()`, `MustParse()`, `NewMux()`, `LoadDefaults()`
- Package-private functions lowercase: `newTestCluster()`, `envMap()`, `nodeRec()`
- Single-letter receivers for trivial methods: `func (i ID) String()`, `func (b *broadcast) Invalidates()`
- Constructor functions return bare type: `func New() ID` (not `NewID`)

**JavaScript/Svelte Files:**
- camelCase for functions: `shortId()`, `relTime()`, `joinTargets()`, `nodeById()`
- camelCase for exported constants: `ZERO_ID` (only for special constants; mostly use full case for clarity)
- PascalCase for classes: `ApiError`, `Toast`, `EditableText` (Svelte components)
- Lowercase for module filenames: `fmt.js`, `api.js`, `derive.js`, `ws.svelte.js`
- Component filenames PascalCase: `App.svelte`, `EditableText.svelte`, `MemberRow.svelte`
- Internal state variables with $ prefix in Svelte: `$state()`, `$derived()`, `$effect()`

**Variables:**
- Go: snake_case for package-private and local variables: `self`, `snap`, `peer`, `c.mu` (mutex)
- Go: Short names for loop variables: `i`, `n`, `h`, `p` (header, payload)
- JS: camelCase: `self`, `cluster`, `selectedMaster`, `statusLevel`, `showFallback`
- JS: Private module state with leading underscore: `_selfId`

**Types:**
- Go: Struct types in PascalCase, exported: `Config`, `Cluster`, `Node`
- Go: Map keys as type aliases: `type ID [16]byte`, `type PortFlags uint32`
- JS: Class/constructor-style for error types: `ApiError extends Error`

## Code Style

**Formatting:**
- Go: Follow `gofmt` (no custom config detected)
- JavaScript: No linter/formatter config detected; observed conventions are consistent spacing and indentation
- Svelte: 2-space indentation in templates and scripts

**Linting:**
- Go: Standard Go idioms (no `.golangci.yml` detected); errors unwrapped and checked immediately
- JavaScript: No ESLint or Prettier config; code follows standard conventions

**Imports Organization (Go):**

Order in all files:
1. Standard library (`fmt`, `os`, `time`, `testing`, `sync`, `net`)
2. Third-party (`github.com/...`, `github.com/labstack/echo/v4`)
3. Local imports (`ensemble/internal/...`)

Example from `internal/config/config_test.go`:
```go
import (
    "os"
    "path/filepath"
    "reflect"
    "testing"

    "ensemble/internal/id"
)
```

Example from `internal/api/handlers.go`:
```go
import (
    "net/http"
    "strings"
    "time"

    "github.com/labstack/echo/v4"

    "ensemble/internal/contracts"
    "ensemble/internal/id"
)
```

**Imports Organization (JavaScript):**

Order in all files:
1. Vitest/test framework (`import { describe, it, expect, vi } from "vitest"`)
2. Imports from modules under test (`import { ... } from "./api.js"`)
3. Helper functions (custom test utilities)

Example from `lib/api.test.js`:
```javascript
import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  base,
  setSelfId,
  ApiError,
  // ...
} from "./api.js";
```

**Path Aliases:**
- Go: Relative paths only (`ensemble/internal/...`), no path aliases
- JavaScript: Relative paths only (`./lib/api.js`, `./fmt.js`)
- Svelte: Import from sibling modules: `import { cluster, connect } from "./lib/ws.svelte.js"`

## Error Handling

**Go Patterns:**

Immediate check-and-return for every error:
```go
cfg, err := Load(Options{
    Args:   nil,
    Getenv: envMap(map[string]string{EnvDataDir: dir}),
})
if err != nil {
    t.Fatalf("Load: %v", err)
}
```

Custom errors as unexported package-level vars:
```go
var ErrBadLength = errors.New("id: want 32 hex chars")
```

MustParse for tests/constants (panics on error):
```go
func MustParse(s string) ID {
    i, err := Parse(s)
    if err != nil {
        panic(err)
    }
    return i
}
```

Functions return error as last argument (Go idiom):
```go
func Parse(s string) (ID, error)
func (i *ID) UnmarshalText(b []byte) error
```

**JavaScript/Svelte Patterns:**

Custom error classes extending Error:
```javascript
export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}
```

Try-catch in async functions with rethrow for caller handling:
```javascript
async function toasted(p) {
  try {
    return await p;
  } catch (e) {
    pushToast(e && e.message ? e.message : "action failed", "error");
    throw e;
  }
}
```

Network error handling in req():
```javascript
let resp;
try {
  resp = await fetch(path, opts);
} catch (e) {
  throw new ApiError(0, e && e.message ? e.message : "network error");
}
```

## Logging

**Go:**
- No structured logging library detected; uses `fmt.Printf` in main.go (not in libraries)
- Tests use `t.Fatalf()`, `t.Errorf()` for assertion failures
- Panics reserved for truly unrecoverable errors: `panic("id: crypto/rand failed: " + err.Error())`

**JavaScript/Svelte:**
- No console logging in production code; uses toast notifications for user-facing errors
- `pushToast(message, level)` centralizes error display
- Test assertions via vitest `expect()`

## Comments

**When to Comment:**

Go files have extensive doc comments at package level (explaining spec references):
```go
// Package config resolves flags + env fallbacks, the data/media directories,
// and node.json persistence (id, name, volume, outputDelayMs). It is pure and
// unit-testable: no sockets, no goroutines, no hardware.
package config
```

Inline comments for non-obvious logic:
```go
// Already pending: coalesce (do NOT reset, so a storm of changes still
// saves within ~one debounce window — bounded latency).
```

Constants documented with spec references (e.g., `D35`, `D57`):
```go
// Volume playback software gain 0.0–1.0; default 1.0 (D35)
Volume float64
```

JavaScript has inline comments for non-obvious derivations:
```javascript
// self's role follows its own `following` (the player target), D49+.
const view = (following, groups) => ({
  nodes: [{ id: A, following }],
  groups: groups || [],
});
```

**JSDoc/TSDoc:**

No JSDoc observed. Functions documented via inline comments or naming clarity.

## Function Design

**Size:** 
- Go: Functions range from 10–100 lines; complex logic like `saveLoop()` and `NotifyMsg()` are broken into helpers
- JavaScript: Utility functions are 5–30 lines; components use Svelte runes for derived state (`$derived`, `$state`) to avoid explicit state management

**Parameters:**
- Go: Explicit parameters, no option structs for functions; options structs used for config (`Options{}`)
- Go: Tests use dependency injection: `Options{Args: ..., Getenv: ...}` allows mocking environment
- JavaScript: Destructuring props in Svelte components: `let { value, onsave, placeholder = "", ... } = $props()`

**Return Values:**
- Go: Error as last return value (idiom); multiple returns common
  ```go
  func Load(opts Options) (*Config, error)
  func (c *Cluster) Subscribe() chan contracts.Delta
  ```
- JavaScript: Single return or throws; async functions return Promise
  ```javascript
  export async function getStatus() { return req("GET", "/api/status"); }
  ```

## Module Design

**Exports:**

Go packages are minimal and focused:
- `internal/id/`: ID type + New/Parse/XOR/Marshal/Unmarshal
- `internal/config/`: Config type + Load + mutation helpers (SetRole, SetSpotifyEndpoints)
- `internal/api/`: HTTP handler registration + request structs

JavaScript modules are pure (no side effects on import):
- `lib/fmt.js`: Pure formatters (no DOM, no state)
- `lib/derive.js`: Pure view-selectors over Snapshot (no state mutations)
- `lib/api.js`: REST helpers + ApiError class + base URL logic
- `lib/ws.svelte.js`: WebSocket connection state (uses Svelte stores)

**Barrel Files:**

Not observed. Modules imported directly:
```javascript
import { shortId } from "./fmt.js";
import { ZERO_ID, nodeById } from "./derive.js";
```

Svelte components follow file-per-component:
```
web/src/
  components/
    EditableText.svelte
    Toast.svelte
    MemberRow.svelte
  sections/
    Groups.svelte
    Nodes.svelte
  lib/
    api.js
    derive.js
    fmt.js
```

---

*Convention analysis: 2026-06-11*
