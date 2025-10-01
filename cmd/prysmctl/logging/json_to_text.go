package logging

import (
	"encoding/json"
	"strings"
	"time"

	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	"github.com/sirupsen/logrus"
)

// TranslateFluentdtoUnstructuredLog accepts a JSON object as a string and converts it to Prysm's
// default unstructured text logger.
func TranslateFluentdtoUnstructuredLog(s string) (string, error) {
	// Parse the JSON input
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return "", err
	}

	// Create a logrus entry
	entry := &logrus.Entry{
		Time: time.Time{}, // Zero time since we don't have timestamp info
		Data: make(logrus.Fields),
	}

	// Extract message and severity
	if msg, ok := data["message"].(string); ok {
		entry.Message = msg
		delete(data, "message")
	}

	if severity, ok := data["severity"].(string); ok {
		// Convert severity to logrus level
		level, err := logrus.ParseLevel(strings.ToLower(severity))
		if err != nil {
			// Default to info if we can't parse the level
			entry.Level = logrus.InfoLevel
		} else {
			entry.Level = level
		}
		delete(data, "severity")
	} else {
		entry.Level = logrus.InfoLevel
	}

	// All remaining fields go into Data
	// Convert float64 to int64 if they're whole numbers to avoid scientific notation
	for k, v := range data {
		switch val := v.(type) {
		case float64:
			// Check if it's a whole number
			if val == float64(int64(val)) {
				entry.Data[k] = int64(val)
			} else {
				entry.Data[k] = val
			}
		case float32:
			// Check if it's a whole number
			if val == float32(int64(val)) {
				entry.Data[k] = int64(val)
			} else {
				entry.Data[k] = val
			}
		default:
			entry.Data[k] = v
		}
	}

	// Use the prefixed formatter to format the entry
	formatter := &prefixed.TextFormatter{
		DisableTimestamp: true,
		DisableColors:    false,
	}

	// Format the entry
	formatted, err := formatter.Format(entry)
	if err != nil {
		return "", err
	}

	return string(formatted), nil
}
