package testing

import (
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
