package execution

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGraffitiInfo_GenerateGraffiti(t *testing.T) {
	tests := []struct {
		name         string
		elCode       string
		elCommit     string
		userGraffiti []byte
		wantPrefix   string // user graffiti appears first
		wantSuffix   string // client version info appended after
	}{
		// No EL info cases (CL info "PR" + commit still included when space allows)
		{
			name:         "No EL - empty user graffiti",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte{},
			wantPrefix:   "PR", // Only CL code + commit (no user graffiti to prefix)
		},
		{
			name:         "No EL - short user graffiti",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("my validator"),
			wantPrefix:   "my validator",
			wantSuffix:   "PR", // CL code appended (within suffix that includes commit)
		},
		{
			name:         "No EL - 28 char user graffiti (4 bytes available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("1234567890123456789012345678"), // 28 chars, 4 bytes available = codes only
			wantPrefix:   "1234567890123456789012345678",
			wantSuffix:   "PR", // CL code (no EL, so just PR)
		},
		{
			name:         "No EL - 30 char user graffiti (2 bytes available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("123456789012345678901234567890"), // 30 chars, 2 bytes available = fits PR
			wantPrefix:   "123456789012345678901234567890",
			wantSuffix:   "PR",
		},
		{
			name:         "No EL - 31 char user graffiti (1 byte available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("1234567890123456789012345678901"), // 31 chars, 1 byte available = not enough for code
			wantPrefix:   "1234567890123456789012345678901",         // User only
		},
		{
			name:         "No EL - 32 char user graffiti (0 bytes available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("12345678901234567890123456789012"),
			wantPrefix:   "12345678901234567890123456789012", // User only
		},
		// With EL info - flexible standard format cases
		{
			name:         "With EL - full format (empty user graffiti)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte{},
			wantPrefix:   "GEabcdPR", // No user graffiti, starts with client info
		},
		{
			name:         "With EL - full format (short user graffiti)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("Bob"),
			wantPrefix:   "Bob",
			wantSuffix:   "GEabcdPR", // EL(2)+commit(4)+CL(2)+commit(4)
		},
		{
			name:         "With EL - full format (20 char user, 12 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("12345678901234567890"), // 20 chars, leaves 12 bytes = full format
			wantPrefix:   "12345678901234567890",
			wantSuffix:   "GEabcdPR", // Full format fits (12 bytes)
		},
		{
			name:         "With EL - reduced commits (24 char user, 8 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("123456789012345678901234"), // 24 chars, leaves 8 bytes = reduced format
			wantPrefix:   "123456789012345678901234",
			wantSuffix:   "GEabPR", // Reduced format (8 bytes)
		},
		{
			name:         "With EL - codes only (28 char user, 4 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("1234567890123456789012345678"), // 28 chars, leaves 4 bytes = codes only
			wantPrefix:   "1234567890123456789012345678",
			wantSuffix:   "GEPR", // Codes only (4 bytes)
		},
		{
			name:         "With EL - EL code only (30 char user, 2 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("123456789012345678901234567890"), // 30 chars, leaves 2 bytes = EL code only
			wantPrefix:   "123456789012345678901234567890",
			wantSuffix:   "GE", // EL code (2 bytes)
		},
		{
			name:         "With EL - user only (31 char user, 1 byte available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("1234567890123456789012345678901"), // 31 chars, leaves 1 byte = not enough for code
			wantPrefix:   "1234567890123456789012345678901",         // User only
		},
		{
			name:         "With EL - user only (32 char user, 0 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("12345678901234567890123456789012"),
			wantPrefix:   "12345678901234567890123456789012",
		},
		// Null byte handling
		{
			name:         "Null bytes - input with trailing nulls",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: append([]byte("test"), 0, 0, 0),
			wantPrefix:   "test",
			wantSuffix:   "GEabcdPR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGraffitiInfo()
			if tt.elCode != "" {
				g.UpdateFromEngine(tt.elCode, tt.elCommit)
			}

			result := g.GenerateGraffiti(tt.userGraffiti)
			resultStr := string(result[:])
			trimmed := trimNullBytes(resultStr)

			// Check prefix (user graffiti comes first)
			require.Equal(t, true, len(trimmed) >= len(tt.wantPrefix), "Result too short for prefix check")
			require.Equal(t, tt.wantPrefix, trimmed[:len(tt.wantPrefix)], "Prefix mismatch")

			// Check suffix if specified (client version info appended)
			if tt.wantSuffix != "" {
				require.Equal(t, true, len(trimmed) >= len(tt.wantSuffix), "Result too short for suffix check")
				// The suffix should appear somewhere after the prefix
				afterPrefix := trimmed[len(tt.wantPrefix):]
				require.Equal(t, true, len(afterPrefix) >= len(tt.wantSuffix), "Not enough room for suffix after prefix")
				require.Equal(t, tt.wantSuffix, afterPrefix[:len(tt.wantSuffix)], "Suffix mismatch")
			}
		})
	}
}

func TestGraffitiInfo_UpdateFromEngine(t *testing.T) {
	g := NewGraffitiInfo()

	// Initially no EL info - should still have CL info (PR + commit)
	result := g.GenerateGraffiti([]byte{})
	resultStr := trimNullBytes(string(result[:]))
	require.Equal(t, "PR", resultStr[:2], "Expected CL info before update")

	// Update with EL info
	g.UpdateFromEngine("GE", "1234abcd")

	result = g.GenerateGraffiti([]byte{})
	resultStr = trimNullBytes(string(result[:]))
	require.Equal(t, "GE1234PR", resultStr[:8], "Expected EL+CL info after update")
}

func TestTruncateCommit(t *testing.T) {
	tests := []struct {
		commit string
		n      int
		want   string
	}{
		{"abcd1234", 4, "abcd"},
		{"ab", 4, "ab"},
		{"", 4, ""},
		{"abcdef", 2, "ab"},
	}

	for _, tt := range tests {
		got := truncateCommit(tt.commit, tt.n)
		require.Equal(t, tt.want, got)
	}
}

func trimNullBytes(s string) string {
	for len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return s
}
