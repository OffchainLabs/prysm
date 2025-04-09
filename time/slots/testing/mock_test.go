package testing

import (
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
