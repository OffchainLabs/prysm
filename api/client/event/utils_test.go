package event

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestScanLinesWithCarriage(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "LF line endings",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "CR line endings",
			input:    "line1\rline2\rline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "CRLF line endings",
			input:    "line1\r\nline2\r\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "Mixed line endings",
			input:    "line1\nline2\rline3\r\nline4",
			expected: []string{"line1", "line2", "line3", "line4"},
		},
		{
			name:     "Empty lines",
			input:    "line1\n\nline2\r\rline3",
			expected: []string{"line1", "", "line2", "", "line3"},
		},
		{
			name:     "Empty lines 2",
			input:    "line1\n\rline2\n\rline3",
			expected: []string{"line1", "", "line2", "", "line3"},
		},
		{
			name:     "No line endings",
			input:    "single line without ending",
			expected: []string{"single line without ending"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := bufio.NewScanner(bytes.NewReader([]byte(tc.input)))
			scanner.Split(scanLinesWithCarriage)

			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}

			require.NoError(t, scanner.Err())
			require.Equal(t, len(tc.expected), len(lines), "Number of lines does not match")
			for i, line := range lines {
				require.Equal(t, tc.expected[i], line, "Line %d does not match", i)
			}
		})
	}
}

// TestScanLinesWithCarriageEdgeCases tests edge cases and potential error scenarios
func TestScanLinesWithCarriageEdgeCases(t *testing.T) {
	t.Run("Empty input", func(t *testing.T) {
		scanner := bufio.NewScanner(bytes.NewReader([]byte("")))
		scanner.Split(scanLinesWithCarriage)
		require.Equal(t, scanner.Scan(), false)
		require.NoError(t, scanner.Err())
	})

	t.Run("Very long line", func(t *testing.T) {
		longLine := bytes.Repeat([]byte("a"), bufio.MaxScanTokenSize+1)
		scanner := bufio.NewScanner(bytes.NewReader(longLine))
		scanner.Split(scanLinesWithCarriage)
		require.Equal(t, scanner.Scan(), false)
		require.NotNil(t, scanner.Err())
	})

	t.Run("Line ending at max token size", func(t *testing.T) {
		input := append(bytes.Repeat([]byte("a"), bufio.MaxScanTokenSize-1), '\n')
		scanner := bufio.NewScanner(bytes.NewReader(input))
		scanner.Split(scanLinesWithCarriage)
		require.Equal(t, scanner.Scan(), true)
		require.Equal(t, string(bytes.Repeat([]byte("a"), bufio.MaxScanTokenSize-1)), scanner.Text())
	})
}
