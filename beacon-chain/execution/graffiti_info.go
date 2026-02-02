package execution

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

const (
	// CLCode is the two-letter client code for Prysm.
	CLCode = "PR"
)

// GraffitiInfo holds version information for generating block graffiti.
// It is thread-safe and can be updated by the execution service and read by the validator server.
type GraffitiInfo struct {
	mu       sync.RWMutex
	elCode   string // From engine_getClientVersionV1
	elCommit string // From engine_getClientVersionV1
	logOnce  sync.Once
}

// NewGraffitiInfo creates a new GraffitiInfo.
func NewGraffitiInfo() *GraffitiInfo {
	return &GraffitiInfo{}
}

// UpdateFromEngine updates the EL client information.
func (g *GraffitiInfo) UpdateFromEngine(code, commit string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.elCode = code
	g.elCommit = commit
}

// GenerateGraffiti generates graffiti using the flexible standard
// with the provided user graffiti from the validator client request.
// It packs as much client info as space allows, followed by user graffiti (no space separator).
//
// Available Space | Format
// ≥12 bytes       | EL(2)+commit(4)+CL(2)+commit(4)+user  e.g. "GEabcdPRxxxxSushi"
// 8-11 bytes      | EL(2)+commit(2)+CL(2)+commit(2)+user  e.g. "GEabPRxxSushi"
// 4-7 bytes       | EL(2)+CL(2)+user                      e.g. "GEPRSushi"
// 2-3 bytes       | code(2)+user                          e.g. "GESushi" or "PRSushi"
// <2 bytes        | user only                             e.g. "Sushi"
func (g *GraffitiInfo) GenerateGraffiti(userGraffiti []byte) [32]byte {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result [32]byte
	userStr := string(userGraffiti)
	// Trim trailing null bytes
	for len(userStr) > 0 && userStr[len(userStr)-1] == 0 {
		userStr = userStr[:len(userStr)-1]
	}

	available := 32 - len(userStr)

	g.logOnce.Do(func() {
		logGraffitiInfo(userStr, available)
	})

	clCommit := version.GetCommitPrefix()
	clCommit4 := truncateCommit(clCommit, 4)
	clCommit2 := truncateCommit(clCommit, 2)

	// If no EL info, clear EL commits but still include CL info
	var elCommit4, elCommit2 string
	if g.elCode != "" {
		elCommit4 = truncateCommit(g.elCommit, 4)
		elCommit2 = truncateCommit(g.elCommit, 2)
	}

	var graffiti string
	switch {
	case available >= 12:
		// Full: EL(2)+commit(4)+CL(2)+commit(4)+user
		graffiti = g.elCode + elCommit4 + CLCode + clCommit4 + userStr
	case available >= 8:
		// Reduced commits: EL(2)+commit(2)+CL(2)+commit(2)+user
		graffiti = g.elCode + elCommit2 + CLCode + clCommit2 + userStr
	case available >= 4:
		// Codes only: EL(2)+CL(2)+user
		graffiti = g.elCode + CLCode + userStr
	case available >= 2:
		// EL code only (or CL code if no EL): code(2)+user
		if g.elCode != "" {
			graffiti = g.elCode + userStr
		} else {
			graffiti = CLCode + userStr
		}
	default:
		// User graffiti only
		graffiti = userStr
	}

	copy(result[:], graffiti)
	return result
}

// logGraffitiInfo logs the graffiti format that will be used based on available space.
func logGraffitiInfo(userStr string, available int) {
	userLen := len(userStr)
	fields := log.WithField("userGraffiti", userStr).WithField("userGraffitiLength", userLen)

	switch {
	case available >= 12:
		fields.Info("Graffiti: full client version format (EL code + 4-char commit + CL code + 4-char commit)")
	case available >= 8:
		fields.Warn("Graffiti: user graffiti reduces client version to 2-char commits")
	case available >= 4:
		fields.Warn("Graffiti: user graffiti reduces client version to codes only (no commits)")
	case available >= 2:
		fields.Warn("Graffiti: user graffiti reduces client version to single code only")
	default:
		fields.Warn("Graffiti: user graffiti consumes all 32 bytes, no client version info will be included")
	}
}

// truncateCommit returns the first n characters of the commit string.
func truncateCommit(commit string, n int) string {
	if len(commit) <= n {
		return commit
	}
	return commit[:n]
}
