# Proposal: Automatic Client Version Graffiti (Brief)

## TL;DR

Add automatic EL+CL version info to block graffiti following [ethereum/execution-apis#517](https://github.com/ethereum/execution-apis/pull/517). Format: `GE168dPM63af` (Geth + Prysm). User graffiti always takes precedence.

## Problem

**Prysm hasn't yet implemented the ecosystem standard.** Prysm currently leaves graffiti empty (when no user graffiti is configured), providing no on-chain visibility into client versions.

**Why it matters**: On-chain visibility of client diversity is much easier with standardized graffiti.

## Proposed Solution

### Format
```
<EL_CODE><EL_COMMIT><CL_CODE><CL_COMMIT>
GE      168d      PM      63af       = "GE168dPM63af"
```

- **EL_CODE**: 2-letter code (GE=Geth, NM=Nethermind, BU=Besu, RH=Reth, EG=Erigon)
- **EL_COMMIT**: First 4 hex chars of execution client's git commit (from `engine_getClientVersionV1` JSON-RPC response)
- **CL_CODE**: PM (Prysm)
- **CL_COMMIT**: First 4 hex chars of Prysm's git commit (from build-time ldflags via `version.GetCommitPrefix()`)

### Priority Order
```
1. User graffiti (VC flag, proposer settings, graffiti file)  ← Always wins
   ↓
2. Auto-generated "GE168dPM63af"                              ← New feature
   ↓
3. Default "Prysm/v6.1.0"                                     ← Fallback
```

**Key principle**: User control preserved. No opt-out flag needed.

### Examples

| Configuration | Result                             |
|--------------|------------------------------------|
| No graffiti | `GE168dPM63af` ✨ **NEW**           |
| `--graffiti "🚀"` | `🚀` (unchanged)                   |
| Proposer settings | Custom graffiti (unchanged)        |
| Old Geth (no API) | `Prysm/v6.1.0` (graceful fallback) |

## Key Concepts

### Where Commit Hashes Come From
The graffiti combines version info from **two different sources**:

| Component | Source | How Retrieved |
|-----------|--------|---------------|
| **EL Code + Commit** | Execution client (Geth/etc) | Runtime JSON-RPC call: `engine_getClientVersionV1` returns `{"code":"GE", "commit":"0x168d..."}` |
| **CL Code + Commit** | Prysm binary | Build-time: Bazel injects git commit via ldflags → `version.GetCommitPrefix()` returns "63af" |

### Component Relationships
```
Blockchain Service (contains and delegates)
    │
    ├─► EngineVersionCache (caches EL version, TTL = 6 epochs)
    │
    └─► Graffiti Service (resolution logic)
            │
            ├─► Uses: EngineClient (makes JSON-RPC calls)
            └─► Uses: EngineVersionCache (avoids repeated calls)

Pattern: Blockchain Service implements GraffitiResolver interface,
         delegates actual work to Graffiti Service
```

## Architecture Overview

### Component Flow (Simple)

```
┌─────────────────┐
│ Validator Client│
│  (unchanged)    │
└────────┬────────┘
         │ gRPC: GetBeaconBlock(graffiti="")
         │ "I need a block to sign"
         ↓
┌──────────────────────────────────────┐
│         Beacon Node                  │
│                                      │
│  1. RPC Handler                      │
│     → Receives VC graffiti           │
│                                      │
│  2. Graffiti Service [NEW]           │
│     → Resolves priority order        │
│        (VC > auto > default)         │
│                                      │
│  3. Engine Version Cache [NEW]       │
│     → Stores EL version info         │
│        (TTL: 6 epochs, ~38 min)      │
│                                      │
│  4. Background Refresh [NEW]         │
│     → Pre-warms cache                │
│        (Every 2 epochs, ~13 min)     │
│                                      │
│  5. Engine Client Extension [NEW]    │
│     → engine_getClientVersionV1      │
└──────────────┬───────────────────────┘
               │ JSON-RPC
               ↓
      ┌────────────────┐
      │ Geth/Nethermind│
      └────────────────┘
```

**How it works**:
1. **Validator Client** calls beacon node to get a block (existing behavior, unchanged)
2. **Beacon Node RPC** receives the request with VC's graffiti (empty if not set)
3. **Graffiti Service** decides which graffiti to use based on priority
4. **Cache** provides pre-fetched EL version (if needed for auto-generation)
5. **Block returned** to VC with resolved graffiti

### New Components

| Component | One-Line Summary |
|-----------|-----------------|
| **GraffitiResolver Interface** | Interface that defines `ResolveGraffiti(vcGraffiti)` method for decoupling RPC layer from implementation |
| **Graffiti Service** | Resolves graffiti based on priority: VC graffiti > auto-generated > default |
| **Engine Version Cache** | Thread-safe cache storing EL version with 6-epoch TTL to avoid RPC latency |
| **Background Refresh** | Goroutine that pre-warms cache every 2 epochs so block production never waits |
| **Engine Client Extension** | Adds `GetClientVersion()` RPC method calling `engine_getClientVersionV1` |
| **Version Helpers** | Utility functions to extract/normalize commit hashes without runtime git calls |

### Edited Components

| Component | What Changes                                                                                                  | Why |
|-----------|---------------------------------------------------------------------------------------------------------------|-----|
| **Blockchain Service** | Implement GraffitiResolver interface, initialize cache + graffiti service on startup, spawn refresh goroutine | Central place to manage lifecycle of new components and provide resolution logic |
| **RPC Validator Server** | Add `GraffitiResolver` field, call it (ResolveGraffiti()) in `GetBeaconBlock()`                               | Integration point where VC graffiti meets BN resolution logic |
| **Engine Client** | Add `GetClientVersion()` interface method and implementation                                                  | Extends existing engine API client with new standardized method |
| **Config Params** | Add cache TTL variables (6 epochs, 2 epochs) computed at init                                                 | Configuration for cache timing based on beacon chain slot duration |
| **Node Initialization** | Wire graffiti resolver through RPC config                    | Connects blockchain service to RPC layer via dependency injection |

**Critical Design Choices**:
- ✅ **All logic on beacon node** (VC unchanged, correct process boundary)
- ✅ **Cache with background refresh** (zero latency on block production - pre-warms every 2 epochs)
- ✅ **No runtime git operations** (works in containers - Prysm commit hash injected via Bazel ldflags at compile time)
- ✅ **Best-effort Engine API** (5s timeout, graceful fallback to "Prysm/vX.X.X" if EL doesn't support `engine_getClientVersionV1`)
- ✅ **Interface-based delegation** (RPC depends on `blockchain.GraffitiResolver` interface, not concrete implementation)

### Component Flow (Detailed)

Shows all components (new + edited) and their interactions:

```
┌───────────────────────────────────────────────────────────────┐
│                      VALIDATOR CLIENT                         │
│                       (unchanged)                             │
└────────────────────────┬──────────────────────────────────────┘
                         │
                         │ gRPC: GetBeaconBlock(graffiti="")
                         ↓
┌───────────────────────────────────────────────────────────────┐
│                      BEACON NODE                              │
│                                                               │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Node Initialization [EDITED]                   │          │
│  │  - Wires GraffitiResolver through RPC config    │          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                                     │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────┐          │
│  │  RPC Validator Server [EDITED]                  │          │
│  │  - GetBeaconBlock() receives VC graffiti        │          │
│  │  - Calls: graffitiResolver.ResolveGraffiti()    │          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                                     │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Blockchain Service [EDITED]                    │          │
│  │  - Implements GraffitiResolver interface        │          │
│  │  - Delegates to graffitiService                 │          │
│  │  - Manages lifecycle (cache + service + refresh)│          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                                     │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Graffiti Service [NEW]                         │          │
│  │  - Priority check:                              │          │
│  │    1. VC graffiti provided? → Use it            │          │
│  │    2. Can auto-generate? → generateAutoGraffiti()│         │
│  │    3. Else → defaultGraffiti()                  │          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                                     │
│         (if auto-generating)                                  │
│                         │                                     │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Engine Version Cache [NEW]                     │          │
│  │  - Get(client, maxAge=6epochs)                  │          │
│  │  - Cache hit? → Return cached data              │          │
│  │  - Cache miss? → Call client.GetClientVersion() │          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                          ↑          │
│         (if cache miss) │                          │          │
│                         ↓                          │          │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Engine Client [EDITED]                         │          │
│  │  - GetClientVersion() calls:                    │          │
│  │    engine_getClientVersionV1 (5s timeout)       │          │
│  │  - Returns: [{code:"GE", commit:"0x168d..."}]   │          │
│  └──────────────────────┬──────────────────────────┘          │
│                         │                                     │
│                         ↓                                     │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Version Helpers [NEW]                          │          │
│  │  - GetCommitPrefix() → "63af"                   │          │
│  │  - NormalizeCommitHash("0x168d...") → "168d"    │          │
│  │  - NormalizeClientCode("GE") → "GE"             │          │
│  └─────────────────────────────────────────────────┘          │
│                                                               │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Background Refresh Goroutine [NEW]             │ ┐        │
│  │  - Ticker: Every 2 epochs (~13 min)             │ │        │
│  │  - Calls: cache.Get() to pre-warm               │ │ Async  │
│  │  - Ensures cache always fresh for block prod    │ │ Refresh│
│  └─────────────────────────────────────────────────┘ │        │
│                                                      ↓        │
│  ┌─────────────────────────────────────────────────┐          │
│  │  Config Params [EDITED]                         │          │
│  │  - EngineVersionCacheMaxAge = 6 epochs          │          │
│  │  - EngineVersionRefreshInterval = 2 epochs      │          │
│  └─────────────────────────────────────────────────┘          │
│                                                               │
└─────────────────────────┬─────────────────────────────────────┘
                          │
                          │ JSON-RPC
                          │ engine_getClientVersionV1
                          ↓
                 ┌────────────────┐
                 │ Execution      │ ← Background refresh periodically
                 │ Client (Geth)  │   fetches version via same path
                 └────────────────┘
                   Returns: {"code":"GE", "commit":"0x168d..."}
```

**Legend**:
- `[NEW]` - New component being created
- `[EDITED]` - Existing component with modifications
- `─ ─ ─ ─` - Background refresh path (async, non-blocking)
- `────────` - Request path (synchronous)

**Files to be Changed**:
```
NEW:
- beacon-chain/execution/types/execution_data.go
- beacon-chain/execution/engine_version_cache.go
- beacon-chain/graffiti/graffiti_service.go
- runtime/version/version.go (helpers)
- Tests + metrics

MODIFY:
- beacon-chain/blockchain/service.go (init cache + service)
- beacon-chain/execution/engine_client.go (new RPC method)
- beacon-chain/rpc/.../proposer.go (call resolver)
- config/params/config.go (cache TTL)
```

## Open Questions

1. **Cache timing**: Uses mainnet values (6 epochs, 2 epochs) for all networks. Add env var overrides for testing?
2. **Missing EL commit**: Use "0000" placeholder or fall back to default?
