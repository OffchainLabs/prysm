package execution

import (
	"strings"
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
	g.elCommit = strings.TrimPrefix(commit, "0x")
}

// GenerateGraffiti generates graffiti using the flexible standard
// with the provided user graffiti from the validator client request.
// It places user graffiti first, then appends as much client info as space allows.
//
// A space separator is added between user graffiti and client info when it
// fits without reducing the client version tier.
//
// Available Space | Format
// ≥13 bytes       | user + space + EL(2)+commit(4)+CL(2)+commit(4)  e.g. "Sushi GEabcdPRe4f6"
// 12 bytes        | user + EL(2)+commit(4)+CL(2)+commit(4)          e.g. "12345678901234567890GEabcdPRe4f6"
// 9-11 bytes      | user + space + EL(2)+commit(2)+CL(2)+commit(2)  e.g. "12345678901234567890123 GEabPRe4"
// 8 bytes         | user + EL(2)+commit(2)+CL(2)+commit(2)          e.g. "123456789012345678901234GEabPRe4"
// 5-7 bytes       | user + space + EL(2)+CL(2)                      e.g. "123456789012345678901234567 GEPR"
// 4 bytes         | user + EL(2)+CL(2)                              e.g. "1234567890123456789012345678GEPR"
// 3 bytes         | user + space + code(2)                          e.g. "12345678901234567890123456789 GE"
// 2 bytes         | user + code(2)                                  e.g. "123456789012345678901234567890GE"
// <2 bytes        | user only                                       e.g. "1234567890123456789012345678901x"
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

	g.logOnce.Do(func() {
		logGraffitiInfo(graffiti, available)
	})

	copy(result[:], graffiti)
	return result
}

// logGraffitiInfo logs the graffiti that will be used.
func logGraffitiInfo(graffiti string, available int) {
	if available >= 2 {
		log.WithField("graffiti", graffiti).Info("Graffiti includes client version info appended after user graffiti")
		return
	}
	log.WithField("graffiti", graffiti).Info("Prysm adds consensus and execution debugging information to the end of the graffiti field when possible. To prevent deletion of debugging info, please consider using a shorter graffiti")
}

// truncateCommit returns the first n characters of the commit string.
func truncateCommit(commit string, n int) string {
	if len(commit) <= n {
		return commit
	}
	return commit[:n]
}
