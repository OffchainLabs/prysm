//go:build !linux

package journald

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// Enable returns an error on non-Linux systems
func Enable(_ logrus.Level) error {
	return fmt.Errorf("journald is not supported in this platform")
}
