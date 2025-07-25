package blockchain

import (
	"io"
	"sync"
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
	
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
	    s.cfg.ForkChoiceStore.Lock()
	    defer s.cfg.ForkChoiceStore.Unlock()
	    require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))
	    wg.Done()
	}()

	s.cfg.ForkChoiceStore.Lock()
	require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))
	s.cfg.ForkChoiceStore.Unlock()
	wg.Done()

	wg.Wait()
}
