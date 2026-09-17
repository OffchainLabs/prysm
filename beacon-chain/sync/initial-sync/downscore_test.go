package initialsync

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	p2pt "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type testDownscorePeer int

const (
	testDownscoreNeither testDownscorePeer = iota
	testDownscoreBlock
	testDownscoreBlob
)

func peerIDForTestDownscore(w testDownscorePeer, name string) peer.ID {
	switch w {
	case testDownscoreBlock:
		return peer.ID("block" + name)
	case testDownscoreBlob:
		return peer.ID("blob" + name)
	default:
		return ""
	}
}

func TestUpdatePeerScorerStats(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		processed uint64
		downPeer  testDownscorePeer
	}{
		{
			name:      "invalid block",
			err:       blockchain.ErrInvalidPayload,
			downPeer:  testDownscoreBlock,
			processed: 10,
		},
		{
			name:      "invalid blob",
			err:       verification.ErrBlobIndexInvalid,
			downPeer:  testDownscoreBlob,
			processed: 3,
		},
		{
			name:      "not validity error",
			err:       errors.New("test"),
			processed: 32,
		},
		{
			name:      "no error",
			processed: 32,
		},
	}
	s := &Service{
		cfg: &Config{
			P2P: p2pt.NewTestP2P(t),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := &blocksQueueFetchedData{
				blocksFrom: peerIDForTestDownscore(testDownscoreBlock, c.name),
				blobsFrom:  peerIDForTestDownscore(testDownscoreBlob, c.name),
			}
			s.updatePeerScorerStats(data, c.processed, c.err)
			if c.err != nil && c.downPeer != testDownscoreNeither {
				switch c.downPeer {
				case testDownscoreBlock:
					// block should be downscored
					require.Equal(t, 1, s.cfg.P2P.PeerScoring().BadResponseCount(data.blocksFrom))
					// blob should not be downscored
					require.Equal(t, 0, s.cfg.P2P.PeerScoring().BadResponseCount(data.blobsFrom))
				case testDownscoreBlob:
					// block should not be downscored
					require.Equal(t, 0, s.cfg.P2P.PeerScoring().BadResponseCount(data.blocksFrom))
					// blob should be downscored
					require.Equal(t, 1, s.cfg.P2P.PeerScoring().BadResponseCount(data.blobsFrom))
				}
				assert.Equal(t, uint64(0), s.cfg.P2P.BlockProviderSelector().ProcessedBlocks(data.blocksFrom))
				return
			}
			// block should not be downscored - also we expect a not found error since peer scoring did not interact with blocks
			require.Equal(t, 0, s.cfg.P2P.PeerScoring().BadResponseCount(data.blocksFrom))
			// no downscore recorded for the blob peer
			require.Equal(t, 0, s.cfg.P2P.PeerScoring().BadResponseCount(data.blobsFrom))

			assert.Equal(t, c.processed, s.cfg.P2P.BlockProviderSelector().ProcessedBlocks(data.blocksFrom))
		})
	}
}

func TestOnDataReceivedDownscore(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		downPeer testDownscorePeer
	}{
		{
			name:     "invalid block",
			err:      sync.ErrInvalidFetchedData,
			downPeer: testDownscoreBlock,
		},
		{
			name:     "invalid blob",
			err:      errors.Wrap(verification.ErrBlobInvalid, "test"),
			downPeer: testDownscoreBlob,
		},
		{
			name: "not validity error",
			err:  errors.New("test"),
		},
		{
			name: "no error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := &fetchRequestResponse{
				blocksFrom: peerIDForTestDownscore(testDownscoreBlock, c.name),
				blobsFrom:  peerIDForTestDownscore(testDownscoreBlob, c.name),
				err:        c.err,
			}
			if c.downPeer == testDownscoreBlob {
				require.Equal(t, true, verification.IsBlobValidationFailure(c.err))
			}
			ctx := t.Context()
			p2p := p2pt.NewTestP2P(t)
			mc := &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
			fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{
				chain: mc,
				p2p:   p2p,
				clock: startup.NewClock(mc.Genesis, mc.ValidatorsRoot),
			})
			q := newBlocksQueue(ctx, &blocksQueueConfig{
				p2p:                 p2p,
				blocksFetcher:       fetcher,
				highestExpectedSlot: primitives.Slot(32),
				chain:               mc})
			sm := q.smm.addStateMachine(0)
			sm.state = stateScheduled
			handle := q.onDataReceivedEvent(t.Context())
			endState, err := handle(sm, data)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				require.NoError(t, err)
			}
			// state machine should stay in "scheduled" if there's an error
			// and transition to "data parsed" if there's no error
			if c.err != nil {
				require.Equal(t, stateScheduled, endState)
			} else {
				require.Equal(t, stateDataParsed, endState)
			}
			if c.err != nil && c.downPeer != testDownscoreNeither {
				switch c.downPeer {
				case testDownscoreBlock:
					// block should be downscored
					blocksCount := p2p.PeerScoring().BadResponseCount(data.blocksFrom)
					require.Equal(t, 1, blocksCount)
					// blob should not be downscored
					require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(data.blobsFrom))
				case testDownscoreBlob:
					// block should not be downscored
					require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(data.blocksFrom))
					// blob should be downscored
					blobCount := p2p.PeerScoring().BadResponseCount(data.blobsFrom)
					require.Equal(t, 1, blobCount)
				}
				assert.Equal(t, uint64(0), p2p.BlockProviderSelector().ProcessedBlocks(data.blocksFrom))
				return
			}
			// block should not be downscored - also we expect a not found error since peer scoring did not interact with blocks
			// no downscore recorded for the peer
			require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(data.blocksFrom))
			// no downscore recorded for the peer
			require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(data.blobsFrom))
		})
	}
}
