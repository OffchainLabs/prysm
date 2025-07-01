package sync

import (
	"io"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:                 64,
		BlockBatchLimitBurstFactor:      10,
		BlobBatchLimit:                  32,
		BlobBatchLimitBurstFactor:       2,
		DataColumnBatchLimit:            4096,
		DataColumnBatchLimitBurstFactor: 2,
	})
	defer func() {
		flags.Init(resetFlags)
	}()
	os.Exit(m.Run())
}
