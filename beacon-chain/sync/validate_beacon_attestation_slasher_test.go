package sync

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

// An attestation that references a bad block must be rejected with
// ValidationReject regardless of whether the slasher is enabled. Peer scoring
// depends on that disposition, so gating the bad-block check on the slasher
// lets bad-block attestations through as ValidationIgnore.
func TestService_validateCommitteeIndexBeaconAttestation_BadBlockRejectedWithSlasherEnabled(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	chain := &mockChain.ChainService{
		// 1 slot ago.
		Genesis:          time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second),
		ValidatorsRoot:   [32]byte{'A'},
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
		slasherEnabled:                   true,
	}
	s.initCaches()
	go s.verifierRoutine()

	invalidRoot := [32]byte{'A', 'B', 'C', 'D'}
	s.setBadBlock(ctx, invalidRoot)

	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(1))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState

	helpers.ClearCache()

	// The attestation votes for a block already marked bad.
	att := &ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b101},
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: invalidRoot[:],
			CommitteeIndex:  0,
			Slot:            1,
			Target: &ethpb.Checkpoint{
				Epoch: 0,
				Root:  validBlockRoot[:],
			},
			Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		},
	}

	digest := s.currentForkDigest()
	topic := fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix()

	com, err := helpers.BeaconCommitteeFromState(t.Context(), savedState, att.Data.Slot, att.Data.CommitteeIndex)
	require.NoError(t, err)
	domain, err := signing.Domain(savedState.Fork(), att.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
	require.NoError(t, err)
	attRoot, err := signing.ComputeSigningRoot(att.Data, domain)
	require.NoError(t, err)
	for i := 0; ; i++ {
		if att.GetAggregationBits().BitAt(uint64(i)) {
			att.SetSignature(keys[com[i]].Sign(attRoot[:]).Marshal())
			break
		}
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, att)
	require.NoError(t, err)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, _ := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
	require.Equal(t, pubsub.ValidationReject, res)
}
