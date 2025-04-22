package db

import "github.com/OffchainLabs/prysm/v6/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
