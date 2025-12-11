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
	mu           sync.RWMutex
	userGraffiti string // From --graffiti flag (set once at startup)
	elCode       string // From engine_getClientVersionV1
	elCommit     string // From engine_getClientVersionV1
}

// NewGraffitiInfo creates a new GraffitiInfo with the given user graffiti.
func NewGraffitiInfo(userGraffiti string) *GraffitiInfo {
	return &GraffitiInfo{
		userGraffiti: userGraffiti,
	}
}

// UpdateFromEngine updates the EL client information.
func (g *GraffitiInfo) UpdateFromEngine(code, commit string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.elCode = code
	g.elCommit = commit
}

// GenerateGraffiti generates graffiti using the flexible standard.
// It packs as much client info as space allows after user graffiti.
//
// Available Space | Format
// ≥12 bytes       | EL(2)+commit(4)+CL(2)+commit(4)+user
// 8-11 bytes      | EL(2)+commit(2)+CL(2)+commit(2)+user
// 4-7 bytes       | EL(2)+CL(2)+user
// 2-3 bytes       | EL(2)+user
// <2 bytes        | user only
func (g *GraffitiInfo) GenerateGraffiti() [32]byte {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result [32]byte
	userLen := len(g.userGraffiti)
	available := 32 - userLen

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
		graffiti = g.elCode + elCommit4 + CLCode + clCommit4 + g.userGraffiti
	case available >= 8:
		// Reduced commits: EL(2)+commit(2)+CL(2)+commit(2)+user
		graffiti = g.elCode + elCommit2 + CLCode + clCommit2 + g.userGraffiti
	case available >= 4:
		// Codes only: EL(2)+CL(2)+user
		graffiti = g.elCode + CLCode + g.userGraffiti
	case available >= 2:
		// EL code only: EL(2)+user
		graffiti = g.elCode + g.userGraffiti
	default:
		// User graffiti only
		graffiti = g.userGraffiti
	}

	copy(result[:], graffiti)
	return result
}

// truncateCommit returns the first n characters of the commit string.
func truncateCommit(commit string, n int) string {
	if len(commit) <= n {
		return commit
	}
	return commit[:n]
}

// GenerateGraffitiWithUserInput generates graffiti using the flexible standard
// with the provided user graffiti from the validator client request.
// This is used when the validator client sends custom graffiti per block.
func (g *GraffitiInfo) GenerateGraffitiWithUserInput(userGraffiti []byte) [32]byte {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result [32]byte
	userStr := string(userGraffiti)
	// Trim trailing null bytes
	for len(userStr) > 0 && userStr[len(userStr)-1] == 0 {
		userStr = userStr[:len(userStr)-1]
	}

	userLen := len(userStr)
	available := 32 - userLen

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
		// EL code only: EL(2)+user
		graffiti = g.elCode + userStr
	default:
		// User graffiti only
		graffiti = userStr
	}

	copy(result[:], graffiti)
	return result
}
