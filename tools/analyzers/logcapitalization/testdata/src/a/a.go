package testdata

import (
	logrus "log" // Use standard log package as alias to simulate logrus
)

func BadCapitalization() {
	// These should trigger the analyzer
	logrus.Print("hello world")           // want "log message should start with a capital letter"
	logrus.Printf("starting the process") // want "format string should start with a capital letter"

	// Simulating logrus-style calls
	Info("connection failed")               // want "log message should start with a capital letter"
	Infof("failed to process %d blocks", 5) // want "format string should start with a capital letter"
	Error("low disk space")                 // want "log message should start with a capital letter"
	Debug("processing attestation")         // want "log message should start with a capital letter"

	// More examples
	Warn("validator not found") // want "log message should start with a capital letter"
}

func GoodCapitalization() {
	// These should NOT trigger the analyzer
	logrus.Print("Hello world")
	logrus.Printf("Starting the beacon chain process")

	// Simulating logrus-style calls with proper capitalization
	Info("Connection established successfully")
	Infof("Processing %d blocks in epoch %d", 5, 100)
	Error("Connection failed with timeout")
	Errorf("Failed to process %d blocks", 5)
	Warn("Low disk space detected")
	Debug("Processing attestation for validator")

	// Fun blockchain-specific examples with proper capitalization
	Info("Validator activated successfully")
	Info("New block mined with hash 0x123abc")
	Info("Checkpoint finalized at epoch 50000")
	Info("Sync committee duties assigned")
	Info("Fork choice updated to new head")

	// Acceptable edge cases - these should NOT trigger
	Info("404 validator not found")                      // Numbers are OK
	Info("/eth/v1/beacon/blocks endpoint")               // Paths are OK
	Info("config=mainnet")                               // Config format is OK
	Info("https://beacon-node.example.com")              // URLs are OK
	Infof("%s network started", "mainnet")               // Format specifiers are OK
	Debug("--weak-subjectivity-checkpoint not provided") // CLI flags are OK
	Debug("-v flag enabled")                             // Single dash flags are OK
	Info("--datadir=/tmp/beacon")                        // Flags with values are OK

	// Empty or whitespace
	Info("")    // Empty is OK
	Info("   ") // Just whitespace is OK
}

// Mock logrus-style functions for testing
func Info(msg string)                   { logrus.Print(msg) }
func Infof(format string, args ...any)  { logrus.Printf(format, args...) }
func Error(msg string)                  { logrus.Print(msg) }
func Errorf(format string, args ...any) { logrus.Printf(format, args...) }
func Warn(msg string)                   { logrus.Print(msg) }
func Warnf(format string, args ...any)  { logrus.Printf(format, args...) }
func Debug(msg string)                  { logrus.Print(msg) }
func Debugf(format string, args ...any) { logrus.Printf(format, args...) }
