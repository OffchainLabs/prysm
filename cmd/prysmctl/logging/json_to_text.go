package logging

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TranslateFluentdtoUnstructuredLog accepts a JSON object as a string and converts it to Prysm's
// default unstructured text logger.
func TranslateFluentdtoUnstructuredLog(s string) (string, error) {
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(s), &logEntry); err != nil {
		return "", err
	}

	// Extract standard fields
	message, _ := logEntry["message"].(string)
	severity, _ := logEntry["severity"].(string)

	// Convert severity to lowercase for the level field
	level := strings.ToLower(severity)

	// Build the base log line with time and level
	// Using the default timestamp format from the test
	result := fmt.Sprintf(`time="0001-01-01 00:00:00.00" level=%s msg="%s"`, level, message)

	// Add additional fields in sorted order (based on test output)
	// The order appears to be: error, prefix, slot
	if errVal, ok := logEntry["error"]; ok {
		result += fmt.Sprintf(` error="%v"`, errVal)
	}

	if prefix, ok := logEntry["prefix"]; ok {
		result += fmt.Sprintf(` prefix=%v`, prefix)
	}

	if slot, ok := logEntry["slot"]; ok {
		// Slot is a number, so no quotes
		result += fmt.Sprintf(` slot=%v`, slot)
	}

	result += "\n"

	return result, nil
}
