package event

import (
	"bytes"
)

// adapted from ScanLines in scan.go to handle carriage return characters as separators
func scanLinesWithCarriage(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i, j := bytes.IndexByte(data, '\n'), bytes.IndexByte(data, '\r'); i >= 0 || j >= 0 {
		in := i
		// Select the first index of \n or \r or the second index of \r if it is followed by \n
		if i < 0 || (i > j && i != j+1 && j >= 0) {
			in = j
		}

		// We have a full newline-terminated line.
		return in + 1, dropCR(data[0:in]), nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), dropCR(data), nil
	}
	// Request more data.
	return 0, nil, nil
}

// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}
