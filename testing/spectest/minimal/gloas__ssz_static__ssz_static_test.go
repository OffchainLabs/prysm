package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/ssz_static"
)

func TestMinimal_Gloas_SSZStatic(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	ssz_static.RunSSZStaticTests(t, "minimal")
}
