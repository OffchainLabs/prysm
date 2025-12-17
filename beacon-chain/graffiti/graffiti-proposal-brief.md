# Graffiti Version Info Implementation

## Summary
Add automatic EL+CL version info to block graffiti following [ethereum/execution-apis#517](https://github.com/ethereum/execution-apis/pull/517). Uses the [flexible standard](https://hackmd.io/@wmoBhF17RAOH2NZ5bNXJVg/BJX2c9gja) to pack client info into leftover space after user graffiti.

More details: https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md 

## Implementation

### Core Component: GraffitiInfo Struct
Thread-safe struct holding version information:
```go
const clCode = "PR"

type GraffitiInfo struct {
    mu           sync.RWMutex
    userGraffiti string  // From --graffiti flag (set once at startup)
    clCommit     string  // From version.GetCommitPrefix() helper function
    elCode       string  // From engine_getClientVersionV1
    elCommit     string  // From engine_getClientVersionV1
}
```

### Flow
1. **Startup**: Parse flags, create GraffitiInfo with user graffiti and CL info.
2. **Wiring**: Pass struct to both execution service and RPC validator server
3. **Runtime**: Execution service goroutine periodically calls `engine_getClientVersionV1` and updates EL fields
4. **Block Proposal**: RPC validator server calls `GenerateGraffiti()` to get formatted graffiti

### Flexible Graffiti Format
Packs as much client info as space allows (after user graffiti):

| Available Space | Format | Example |
|----------------|--------|---------|
| ≥12 bytes | `EL(2)+commit(4)+CL(2)+commit(4)+user` | `GE168dPR63afBob` |
| 8-11 bytes | `EL(2)+commit(2)+CL(2)+commit(2)+user` | `GE16PR63my node here` |
| 4-7 bytes | `EL(2)+CL(2)+user` | `GEPRthis is my graffiti msg` |
| 2-3 bytes | `EL(2)+user` | `GEalmost full graffiti message` |
| <2 bytes | user only | `full 32 byte user graffiti here` |

```go
func (g *GraffitiInfo) GenerateGraffiti() [32]byte {
    available := 32 - len(userGraffiti)

    if elCode == "" {
        elCommit2 = elCommit4 = ""
    }

    switch {
    case available >= 12:
        return elCode + elCommit4 + clCode + clCommit4 + userGraffiti
    case available >= 8:
        return elCode + elCommit2 + clCode + clCommit2 + userGraffiti
    case available >= 4:
        return elCode + clCode + userGraffiti
    case available >= 2:
        return elCode + userGraffiti
    default:
        return userGraffiti
    }
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
