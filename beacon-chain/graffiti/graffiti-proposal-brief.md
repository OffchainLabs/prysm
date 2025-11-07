# Graffiti Version Info Implementation

## Summary
Add automatic EL+CL version info to block graffiti following [ethereum/execution-apis#517](https://github.com/ethereum/execution-apis/pull/517). Format: `GE168dPR63af` (Geth commit 168d + Prysm commit 63af). User graffiti always takes precedence.
More details: https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md 

## Implementation

### Core Component: GraffitiInfo Struct
Thread-safe struct holding version information:
```go
type GraffitiInfo struct {
    mu           sync.RWMutex
    userGraffiti string  // From --graffiti flag (set once at startup)
    clCode       string  // "PR" (hardcoded)
    clCommit     string  // From version.GetCommitPrefix() helper function
    elCode       string  // From engine_getClientVersionV1
    elCommit     string  // From engine_getClientVersionV1
}
```

### Flow
1. **Startup**: Parse flags, create GraffitiInfo with user graffiti and CL info. If user graffiti is set, log an informational message that custom graffiti overrides automatic client version reporting which helps track client diversity.
2. **Wiring**: Pass struct to both execution service and RPC validator server
3. **Runtime**: Execution service goroutine periodically calls `engine_getClientVersionV1` and updates EL fields
4. **Block Proposal**: RPC validator server calls `GenerateGraffiti()` to get formatted graffiti

### Priority Order
```go
func (g *GraffitiInfo) GenerateGraffiti() [32]byte {
    if userGraffiti != "" {
        return userGraffiti           // User graffiti always wins
    }
    if elCode != "" {
        return elCode + elCommit + clCode + clCommit  // "GE168dPR63af"
    }
    return "Prysm/" + version       // Fallback if no EL info
}
```

### Update Logic
Single testable function in execution service:
```go
func (s *Service) updateGraffitiInfo() {
    versions, err := s.GetClientVersion(ctx)
    if err != nil {
        return  // Keep last good value
    }
    if len(versions) == 1 {
        s.graffitiInfo.UpdateFromEngine(versions[0].Code, versions[0].Commit)
    }
}
```

Goroutine calls this on `slot % 8 == 4` timing (4 times per epoch, avoids slot boundaries).

### Files Changes Required

**New:**
- `beacon-chain/execution/graffiti_info.go` - The struct and methods
- `beacon-chain/execution/graffiti_info_test.go` - Unit tests
- `runtime/version/version.go` - Add `GetCommitPrefix()` helper that extracts first 4 hex chars from the git commit injected via Bazel ldflags at build time

**Modified:**
- `beacon-chain/execution/service.go` - Add goroutine + updateGraffitiInfo()
- `beacon-chain/execution/engine_client.go` - Add GetClientVersion() method that does engine call
- `beacon-chain/rpc/.../validator/proposer.go` - Call GenerateGraffiti()
- `beacon-chain/node/node.go` - Wire GraffitiInfo to services

### Testing Strategy
- Unit test GraffitiInfo methods (priority logic, thread safety)
- Unit test updateGraffitiInfo() with mocked engine client
