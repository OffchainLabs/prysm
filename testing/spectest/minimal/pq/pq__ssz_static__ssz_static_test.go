package pq

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/pq/ssz_static"
)

func TestMinimal_PQ_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "minimal")
}
