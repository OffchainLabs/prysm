package testing

import (
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
