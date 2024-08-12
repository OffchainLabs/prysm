package verification

import (
	"os"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
)

func TestMain(t *testing.M) {
	if err := kzg.Start(); err != nil {
		os.Exit(1)
	}
	t.Run()
}
