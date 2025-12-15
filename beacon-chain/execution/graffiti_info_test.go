package execution

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGraffitiInfo_GenerateGraffiti_NoELInfo(t *testing.T) {
	g := NewGraffitiInfo()

	// Without EL info, should still include CL info (PR + commit)
	result := g.GenerateGraffiti([]byte{})
	resultStr := string(result[:])

	// Should start with "PR" (CL code) since EL is missing but CL info is still included
	require.Equal(t, true, len(resultStr) >= 2 && resultStr[:2] == "PR", "Expected graffiti to start with PR (CL code)")
}

func TestGraffitiInfo_GenerateGraffiti_WithUserGraffiti(t *testing.T) {
	g := NewGraffitiInfo()

	// Without EL info, should still include CL info + user graffiti
	// "my validator" = 12 chars, available = 20 bytes, so full CL format: PR + commit(4) + user
	result := g.GenerateGraffiti([]byte("my validator"))
	resultStr := trimNullBytes(string(result[:]))

	// Should start with "PR" and end with "my validator"
	require.Equal(t, true, len(resultStr) >= 2 && resultStr[:2] == "PR", "Expected graffiti to start with PR")
	require.Equal(t, true, len(resultStr) >= 12 && resultStr[len(resultStr)-12:] == "my validator", "Expected graffiti to end with user graffiti")
}

func TestGraffitiInfo_GenerateGraffiti_NoELInfo_LongUserGraffiti(t *testing.T) {
	g := NewGraffitiInfo()
	// No EL info set

	tests := []struct {
		name         string
		userGraffiti string
		wantPrefix   string
	}{
		{
			// 29 chars user graffiti, available = 3 bytes
			// Should still include CL code "PR" since 2 bytes fit
			name:         "29 char user graffiti - should include PR",
			userGraffiti: "12345678901234567890123456789",
			wantPrefix:   "PR",
		},
		{
			// 30 chars user graffiti, available = 2 bytes
			// Should still include CL code "PR" since exactly 2 bytes fit
			name:         "30 char user graffiti - should include PR",
			userGraffiti: "123456789012345678901234567890",
			wantPrefix:   "PR",
		},
		{
			// 31 chars user graffiti, available = 1 byte
			// Not enough space for PR, should be user only
			name:         "31 char user graffiti - user only",
			userGraffiti: "1234567890123456789012345678901",
			wantPrefix:   "1234567890123456789012345678901",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.GenerateGraffiti([]byte(tt.userGraffiti))
			resultStr := string(result[:])

			require.Equal(t, true, len(resultStr) >= len(tt.wantPrefix), "Result too short")
			require.Equal(t, tt.wantPrefix, resultStr[:len(tt.wantPrefix)], "Prefix mismatch")
		})
	}
}

func TestGraffitiInfo_GenerateGraffiti_FlexibleStandard(t *testing.T) {
	tests := []struct {
		name         string
		userGraffiti string
		elCode       string
		elCommit     string
		wantPrefix   string
	}{
		{
			name:         "Full format - empty user graffiti",
			userGraffiti: "",
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GEabcdPR", // GE + 4 char commit + PR + 4 char CL commit
		},
		{
			name:         "Full format - short user graffiti",
			userGraffiti: "Bob",
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GEabcdPR",
		},
		{
			name:         "Reduced format - 20 char user graffiti",
			userGraffiti: "12345678901234567890", // 20 chars, 12 bytes available
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GEabcdPR", // Still fits full format
		},
		{
			name:         "Reduced commits - 24 char user graffiti",
			userGraffiti: "123456789012345678901234", // 24 chars, 8 bytes available
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GEabPR", // EL(2)+commit(2)+CL(2)+commit(2), CL commit is dynamic
		},
		{
			name:         "EL+CL codes only - 28 char user graffiti",
			userGraffiti: "1234567890123456789012345678", // 28 chars, 4 bytes available
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GEPR", // EL(2)+CL(2)
		},
		{
			name:         "EL code only - 30 char user graffiti",
			userGraffiti: "123456789012345678901234567890", // 30 chars, 2 bytes available
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "GE", // EL(2) only
		},
		{
			name:         "User only - 32 char user graffiti",
			userGraffiti: "12345678901234567890123456789012", // 32 chars, 0 bytes available
			elCode:       "GE",
			elCommit:     "abcd1234",
			wantPrefix:   "12345678901234567890123456789012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGraffitiInfo()
			g.UpdateFromEngine(tt.elCode, tt.elCommit)

			result := g.GenerateGraffiti([]byte(tt.userGraffiti))
			resultStr := string(result[:])

			// Check that result starts with expected prefix
			require.Equal(t, true, len(resultStr) >= len(tt.wantPrefix), "Result too short")
			require.Equal(t, tt.wantPrefix, resultStr[:len(tt.wantPrefix)], "Prefix mismatch")
		})
	}
}

func TestGraffitiInfo_GenerateGraffiti_WithNullBytes(t *testing.T) {
	g := NewGraffitiInfo()
	g.UpdateFromEngine("GE", "abcd1234")

	tests := []struct {
		name       string
		userInput  []byte
		wantPrefix string
	}{
		{
			name:       "Empty input",
			userInput:  []byte{},
			wantPrefix: "GEabcdPR",
		},
		{
			name:       "Short input",
			userInput:  []byte("hello"),
			wantPrefix: "GEabcdPR",
		},
		{
			name:       "Input with null bytes",
			userInput:  append([]byte("test"), 0, 0, 0),
			wantPrefix: "GEabcdPR",
		},
		{
			name:       "Full 32 byte input",
			userInput:  []byte("12345678901234567890123456789012"),
			wantPrefix: "12345678901234567890123456789012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.GenerateGraffiti(tt.userInput)
			resultStr := string(result[:])

			require.Equal(t, true, len(resultStr) >= len(tt.wantPrefix), "Result too short")
			require.Equal(t, tt.wantPrefix, resultStr[:len(tt.wantPrefix)], "Prefix mismatch")
		})
	}
}

func TestGraffitiInfo_UpdateFromEngine(t *testing.T) {
	g := NewGraffitiInfo()

	// Initially no EL info - should still have CL info (PR + commit)
	result := g.GenerateGraffiti([]byte{})
	resultStr := string(result[:])
	require.Equal(t, true, resultStr[:2] == "PR", "Expected CL info before update")

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
