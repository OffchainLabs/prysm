package testing

import (
	"github.com/prysmaticlabs/prysm/v6/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
