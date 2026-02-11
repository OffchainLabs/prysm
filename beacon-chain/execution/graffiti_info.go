package execution

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

const (
	// CLCode is the two-letter client code for Prysm.
	CLCode = "PR"
	Name   = "Prysm"
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
// It places user graffiti first, then appends as much client info as space allows.
//
// Available Space | Format
// ≥12 bytes       | user+EL(2)+commit(4)+CL(2)+commit(4)  e.g. "SushiGEabcdPRxxxx"
// 8-11 bytes      | user+EL(2)+commit(2)+CL(2)+commit(2)  e.g. "SushiGEabPRxx"
// 4-7 bytes       | user+EL(2)+CL(2)                      e.g. "SushiGEPR"
// 2-3 bytes       | user+code(2)                          e.g. "SushiGE" or "SushiPR"
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

	// Add a space separator between user graffiti and client info,
	// but only if it won't reduce the space available for client version info.
	space := func(minForTier int) string {
		if len(userStr) > 0 && available >= minForTier+1 {
			return " "
		}
		return ""
	}

	var graffiti string
	switch {
	case available >= 12:
		// Full: user+EL(2)+commit(4)+CL(2)+commit(4)
		graffiti = userStr + space(12) + g.elCode + elCommit4 + CLCode + clCommit4
	case available >= 8:
		// Reduced commits: user+EL(2)+commit(2)+CL(2)+commit(2)
		graffiti = userStr + space(8) + g.elCode + elCommit2 + CLCode + clCommit2
	case available >= 4:
		// Codes only: user+EL(2)+CL(2)
		graffiti = userStr + space(4) + g.elCode + CLCode
	case available >= 2:
		// Single code: user+code(2)
		if g.elCode != "" {
			graffiti = userStr + space(2) + g.elCode
		} else {
			graffiti = userStr + space(2) + CLCode
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
