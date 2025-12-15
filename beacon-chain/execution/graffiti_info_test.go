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
		wantPrefix   string
		wantSuffix   string // for checking user graffiti is appended
	}{
		// No EL info cases
		{
			name:         "No EL - empty user graffiti",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte{},
			wantPrefix:   "PR", // CL code still included
		},
		{
			name:         "No EL - short user graffiti",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("my validator"),
			wantPrefix:   "PR",
			wantSuffix:   "my validator",
		},
		{
			name:         "No EL - 29 char user graffiti (3 bytes available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("12345678901234567890123456789"),
			wantPrefix:   "PR", // CL code fits in 2 bytes
		},
		{
			name:         "No EL - 30 char user graffiti (2 bytes available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("123456789012345678901234567890"),
			wantPrefix:   "PR", // CL code exactly fits
		},
		{
			name:         "No EL - 31 char user graffiti (1 byte available)",
			elCode:       "",
			elCommit:     "",
			userGraffiti: []byte("1234567890123456789012345678901"),
			wantPrefix:   "1234567890123456789012345678901", // No space for CL code
		},
		// With EL info - flexible standard format cases
		{
			name:         "With EL - full format (empty user graffiti)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte{},
			wantPrefix:   "GEabcdPR", // EL(2)+commit(4)+CL(2)+commit(4)
		},
		{
			name:         "With EL - full format (short user graffiti)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("Bob"),
			wantPrefix:   "GEabcdPR",
			wantSuffix:   "Bob",
		},
		{
			name:         "With EL - full format (20 char user, 12 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("12345678901234567890"),
			wantPrefix:   "GEabcdPR",
		},
		{
			name:         "With EL - reduced commits (24 char user, 8 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("123456789012345678901234"),
			wantPrefix:   "GEabPR", // EL(2)+commit(2)+CL(2)+commit(2)
		},
		{
			name:         "With EL - codes only (28 char user, 4 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("1234567890123456789012345678"),
			wantPrefix:   "GEPR", // EL(2)+CL(2)
		},
		{
			name:         "With EL - EL code only (30 char user, 2 bytes available)",
			elCode:       "GE",
			elCommit:     "abcd1234",
			userGraffiti: []byte("123456789012345678901234567890"),
			wantPrefix:   "GE", // EL(2) only
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
			wantPrefix:   "GEabcdPR",
			wantSuffix:   "test",
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

			// Check prefix
			require.Equal(t, true, len(resultStr) >= len(tt.wantPrefix), "Result too short for prefix check")
			require.Equal(t, tt.wantPrefix, resultStr[:len(tt.wantPrefix)], "Prefix mismatch")

			// Check suffix if specified
			if tt.wantSuffix != "" {
				trimmed := trimNullBytes(resultStr)
				require.Equal(t, true, len(trimmed) >= len(tt.wantSuffix), "Result too short for suffix check")
				require.Equal(t, tt.wantSuffix, trimmed[len(trimmed)-len(tt.wantSuffix):], "Suffix mismatch")
			}
		})
	}
}

func TestGraffitiInfo_UpdateFromEngine(t *testing.T) {
	g := NewGraffitiInfo()

	// Initially no EL info - should still have CL info (PR + commit)
	result := g.GenerateGraffiti([]byte{})
	resultStr := string(result[:])
	require.Equal(t, "PR", resultStr[:2], "Expected CL info before update")

	// Update with EL info
	g.UpdateFromEngine("GE", "1234abcd")

	result = g.GenerateGraffiti([]byte{})
	resultStr = string(result[:])
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
