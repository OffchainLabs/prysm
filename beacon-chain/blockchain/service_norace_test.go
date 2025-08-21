package blockchain

import (
	"io"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
}

func TestChainService_SaveHead_DataRace(t *testing.T) {
	s := testServiceWithDB(t)
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	st, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, err)
	go func() {
		require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))
	}()
	require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))
}
