package testutil

import "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"

type PathTest struct {
	Path     string
	Expected any
}

type TestSpec struct {
	Name      string
	Type      query.SSZObject
	Instance  query.SSZObject
	PathTests []PathTest
}
