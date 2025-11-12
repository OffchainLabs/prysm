package attestations

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/kv"
)

var _ Pool = (*kv.AttCaches)(nil)
