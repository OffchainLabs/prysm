package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestSendAttestationReadyOnBlock(t *testing.T) {
	setup := func(t *testing.T, slotOffset primitives.Slot) (*Service, *postBlockProcessConfig, chan *feed.Event) {
		s, _ := minimalTestService(t)
		blk := util.NewBeaconBlock()
		blk.Block.Slot = s.CurrentSlot() - slotOffset
		signed, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		roblock, err := blocks.NewROBlockWithRoot(signed, [32]byte{'b'})
		require.NoError(t, err)
		events := make(chan *feed.Event, 10)
		sub := s.cfg.StateNotifier.StateFeed().Subscribe(events)
		t.Cleanup(sub.Unsubscribe)
		return s, &postBlockProcessConfig{ctx: t.Context(), roblock: roblock}, events
	}

	drain := func(events chan *feed.Event) *statefeed.AttestationReadyData {
		for {
			select {
			case e := <-events:
				if e.Type == statefeed.AttestationReady {
					d, ok := e.Data.(*statefeed.AttestationReadyData)
					if ok {
						return d
					}
				}
			default:
				return nil
			}
		}
	}

	t.Run("head block for current slot fires", func(t *testing.T) {
		s, cfg, events := setup(t, 0)
		cfg.headRoot = cfg.roblock.Root()
		s.cfg.ForkChoiceStore.Lock()
		s.sendAttestationReadyOnBlock(cfg)
		s.cfg.ForkChoiceStore.Unlock()
		got := drain(events)
		require.NotNil(t, got)
		require.Equal(t, cfg.roblock.Root(), got.BeaconBlockRoot)
		require.Equal(t, cfg.roblock.Block().Slot(), got.Slot)
	})

	t.Run("old slot block does not fire", func(t *testing.T) {
		s, cfg, events := setup(t, 1)
		cfg.headRoot = cfg.roblock.Root()
		s.cfg.ForkChoiceStore.Lock()
		s.sendAttestationReadyOnBlock(cfg)
		s.cfg.ForkChoiceStore.Unlock()
		require.IsNil(t, drain(events))
	})

	t.Run("non-head block does not fire when not finalizing", func(t *testing.T) {
		s, cfg, events := setup(t, 0)
		cfg.headRoot = [32]byte{'p'}
		s.cfg.ForkChoiceStore.Lock()
		s.sendAttestationReadyOnBlock(cfg)
		s.cfg.ForkChoiceStore.Unlock()
		require.IsNil(t, drain(events))
	})

	t.Run("non-head block fires with head root when finalizing", func(t *testing.T) {
		s, cfg, events := setup(t, 0)
		cfg.headRoot = [32]byte{'p'}
		s.cfg.ForkChoiceStore.Lock()
		require.NoError(t, s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{
			Epoch: slots.ToEpoch(s.CurrentSlot()) - 2,
		}))
		s.sendAttestationReadyOnBlock(cfg)
		s.cfg.ForkChoiceStore.Unlock()
		got := drain(events)
		require.NotNil(t, got)
		require.Equal(t, [32]byte{'p'}, got.BeaconBlockRoot)
	})
}
