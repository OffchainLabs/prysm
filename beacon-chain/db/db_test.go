package db

import "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
