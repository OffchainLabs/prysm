package db

import "github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
