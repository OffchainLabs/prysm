//go:build tools

package tools

// Trick go mod into requiring NilAway and therefore Gazelle won't prune it.
import (
	_ "go.uber.org/nilaway"
)
