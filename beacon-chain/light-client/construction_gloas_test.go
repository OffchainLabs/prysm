package light_client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

func TestLightClientConstruction_GloasDefaults(t *testing.T) {
	setupConstructionConfig(t)
	slot := primitives.Slot(7 * fieldparams.SlotsPerEpoch)
	block := constructionBlock(t, version.Gloas, slot)
	update, err := CreateDefaultLightClientUpdate(block)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, update.Version())
	p := update.Proto().(*pb.LightClientUpdateGloas)
	require.Equal(t, slot, p.AttestedHeader.Beacon.Slot)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(p.NextSyncCommittee.Pubkeys)))
	require.Equal(t, 11, len(p.NextSyncCommitteeBranch))
	require.Equal(t, 9, len(p.FinalityBranch))
	require.Equal(t, true, proto.Equal(emptyLightClientHeaderGloas(0), p.FinalizedHeader))
	hasCommittee, err := HasRelevantSyncCommittee(update)
	require.NoError(t, err)
	require.Equal(t, false, hasCommittee)
	hasFinality, err := HasFinality(update)
	require.NoError(t, err)
	require.Equal(t, false, hasFinality)
	encoded, err := update.MarshalSSZ()
	require.NoError(t, err)
	decoded := &pb.LightClientUpdateGloas{}
	require.NoError(t, decoded.UnmarshalSSZ(encoded))
	require.Equal(t, true, proto.Equal(p, decoded))

	bootstrap, err := createDefaultLightClientBootstrap(slot)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, bootstrap.Version())
	branch, err := bootstrap.CurrentSyncCommitteeBranchGloas()
	require.NoError(t, err)
	require.Equal(t, interfaces.LightClientSyncCommitteeBranchGloas{}, branch)
	encoded, err = bootstrap.MarshalSSZ()
	require.NoError(t, err)
	decodedBootstrap := &pb.LightClientBootstrapGloas{}
	require.NoError(t, decodedBootstrap.UnmarshalSSZ(encoded))
	require.Equal(t, true, proto.Equal(bootstrap.Proto(), decodedBootstrap))

	// Empty attested and finalized headers must not share their backing slices.
	p.AttestedHeader.ExecutionBranch[0][0] = 1
	p.AttestedHeader.Beacon.ParentRoot[0] = 1
	require.Equal(t, byte(0), p.FinalizedHeader.ExecutionBranch[0][0])
	require.Equal(t, byte(0), p.FinalizedHeader.Beacon.ParentRoot[0])
}

func TestLightClientConstruction_GloasUpdates(t *testing.T) {
	setupConstructionConfig(t)
	for _, tt := range []struct {
		name    string
		fork    int
		blinded bool
	}{
		{name: "unavailable", fork: -1},
		{name: "genesis", fork: version.Phase0},
		{name: "altair", fork: version.Altair},
		{name: "bellatrix", fork: version.Bellatrix},
		{name: "capella", fork: version.Capella},
		{name: "deneb", fork: version.Deneb},
		{name: "electra", fork: version.Electra},
		{name: "fulu", fork: version.Fulu},
		{name: "gloas", fork: version.Gloas},
		{name: "blinded capella", fork: version.Capella, blinded: true},
		{name: "blinded fulu", fork: version.Fulu, blinded: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var finalized interfaces.ReadOnlySignedBeaconBlock
			if tt.fork >= version.Phase0 {
				block := constructionBlock(t, tt.fork, primitives.Slot(tt.fork*fieldparams.SlotsPerEpoch))
				finalized = block
				if tt.blinded {
					var err error
					finalized, err = block.ToBlinded()
					require.NoError(t, err)
				}
			}
			attestedSlot := primitives.Slot(7*fieldparams.SlotsPerEpoch + 1)
			attestedState, attestedBlock := constructionStateBlock(t, version.Gloas, attestedSlot, [32]byte{}, finalized)
			attestedRoot, err := attestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			signatureState, signatureBlock := constructionStateBlock(t, version.Gloas, attestedSlot+1, attestedRoot, nil)
			update, err := NewLightClientUpdateFromBeaconState(t.Context(), signatureState, signatureBlock, attestedState, attestedBlock, finalized)
			require.NoError(t, err)
			require.Equal(t, version.Gloas, update.Version())
			require.Equal(t, signatureBlock.Block().Slot(), update.SignatureSlot())
			p := update.Proto().(*pb.LightClientUpdateGloas)
			syncAggregate, err := signatureBlock.Block().Body().SyncAggregate()
			require.NoError(t, err)
			require.Equal(t, true, proto.Equal(syncAggregate, p.SyncAggregate))
			checkConstructionHeader(t, update.AttestedHeader(), attestedBlock)
			committeeRoot, err := p.NextSyncCommittee.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, true, trie.VerifyMerkleProof(p.AttestedHeader.Beacon.StateRoot, committeeRoot[:], 2946, p.NextSyncCommitteeBranch))
			hasCommittee, err := HasRelevantSyncCommittee(update)
			require.NoError(t, err)
			require.Equal(t, true, hasCommittee)
			hasFinality, err := HasFinality(update)
			require.NoError(t, err)
			require.Equal(t, finalized != nil, hasFinality)
			if finalized != nil {
				require.Equal(t, true, trie.VerifyMerkleProof(p.AttestedHeader.Beacon.StateRoot, attestedState.FinalizedCheckpoint().Root, 735, p.FinalityBranch))
			}
			if finalized == nil || finalized.Block().Slot() == 0 {
				require.Equal(t, true, proto.Equal(emptyLightClientHeaderGloas(0), p.FinalizedHeader))
			} else {
				checkConstructionHeader(t, update.FinalizedHeader(), finalized)
				root, err := p.FinalizedHeader.Beacon.HashTreeRoot()
				require.NoError(t, err)
				require.DeepEqual(t, root[:], attestedState.FinalizedCheckpoint().Root)
			}
			encoded, err := update.MarshalSSZ()
			require.NoError(t, err)
			decoded := &pb.LightClientUpdateGloas{}
			require.NoError(t, decoded.UnmarshalSSZ(encoded))
			require.Equal(t, true, proto.Equal(p, decoded))

			finality, err := NewLightClientFinalityUpdateFromBeaconState(t.Context(), signatureState, signatureBlock, attestedState, attestedBlock, finalized)
			require.NoError(t, err)
			require.Equal(t, version.Gloas, finality.Version())
			require.Equal(t, true, proto.Equal(p.AttestedHeader, finality.AttestedHeader().Proto()))
			require.Equal(t, true, proto.Equal(p.FinalizedHeader, finality.FinalizedHeader().Proto()))
			require.DeepEqual(t, p.FinalityBranch, finality.Proto().(*pb.LightClientFinalityUpdateGloas).FinalityBranch)
			_, err = finality.MarshalSSZ()
			require.NoError(t, err)
			optimistic, err := NewLightClientOptimisticUpdateFromBeaconState(t.Context(), signatureState, signatureBlock, attestedState, attestedBlock)
			require.NoError(t, err)
			require.Equal(t, version.Gloas, optimistic.Version())
			require.Equal(t, true, proto.Equal(p.AttestedHeader, optimistic.AttestedHeader().Proto()))
			_, err = optimistic.MarshalSSZ()
			require.NoError(t, err)
		})
	}
}

func TestLightClientConstruction_GloasBoundaries(t *testing.T) {
	setupConstructionConfig(t)
	t.Run("signature crosses fork", func(t *testing.T) {
		forkSlot := primitives.Slot(7 * fieldparams.SlotsPerEpoch)
		attestedState, attestedBlock := constructionStateBlock(t, version.Fulu, forkSlot-1, [32]byte{}, nil)
		root, err := attestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		signatureState, signatureBlock := constructionStateBlock(t, version.Gloas, forkSlot, root, nil)
		update, err := NewLightClientUpdateFromBeaconState(t.Context(), signatureState, signatureBlock, attestedState, attestedBlock, nil)
		require.NoError(t, err)
		p, ok := update.Proto().(*pb.LightClientUpdateElectra)
		require.Equal(t, true, ok)
		require.Equal(t, version.Fulu, update.Version())
		committeeRoot, err := p.NextSyncCommittee.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(p.AttestedHeader.Beacon.StateRoot, committeeRoot[:], 87, p.NextSyncCommitteeBranch))
	})
	t.Run("signature crosses committee period", func(t *testing.T) {
		periodSlots := primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SlotsPerEpoch
		attestedState, attestedBlock := constructionStateBlock(t, version.Gloas, periodSlots*2-1, [32]byte{}, nil)
		root, err := attestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		signatureState, signatureBlock := constructionStateBlock(t, version.Gloas, periodSlots*2, root, nil)
		update, err := NewLightClientUpdateFromBeaconState(t.Context(), signatureState, signatureBlock, attestedState, attestedBlock, nil)
		require.NoError(t, err)
		hasCommittee, err := HasRelevantSyncCommittee(update)
		require.NoError(t, err)
		require.Equal(t, false, hasCommittee)
		branch, err := update.NextSyncCommitteeBranchGloas()
		require.NoError(t, err)
		require.Equal(t, interfaces.LightClientSyncCommitteeBranchGloas{}, branch)
		for _, key := range update.NextSyncCommittee().Pubkeys {
			require.DeepEqual(t, make([]byte, fieldparams.BLSPubkeyLength), key)
		}
	})
}

func TestLightClientConstruction_BootstrapForks(t *testing.T) {
	setupConstructionConfig(t)
	for _, fork := range []int{version.Altair, version.Bellatrix, version.Capella, version.Deneb, version.Electra, version.Fulu, version.Gloas} {
		t.Run(version.String(fork), func(t *testing.T) {
			st, block := constructionStateBlock(t, fork, primitives.Slot(fork*fieldparams.SlotsPerEpoch), [32]byte{}, nil)
			// A Gloas wall-clock slot must not upgrade a historical bootstrap.
			bootstrap, err := NewLightClientBootstrapFromBeaconState(t.Context(), 8*fieldparams.SlotsPerEpoch, st, block)
			require.NoError(t, err)
			require.Equal(t, fork, bootstrap.Version())
			committee, err := st.CurrentSyncCommittee()
			require.NoError(t, err)
			require.Equal(t, true, proto.Equal(committee, bootstrap.CurrentSyncCommittee()))
			committeeRoot, err := committee.HashTreeRoot()
			require.NoError(t, err)
			var branch [][]byte
			var gindex uint64
			switch p := bootstrap.Proto().(type) {
			case *pb.LightClientBootstrapAltair:
				branch, gindex = p.CurrentSyncCommitteeBranch, 54
			case *pb.LightClientBootstrapCapella:
				branch, gindex = p.CurrentSyncCommitteeBranch, 54
			case *pb.LightClientBootstrapDeneb:
				branch, gindex = p.CurrentSyncCommitteeBranch, 54
			case *pb.LightClientBootstrapElectra:
				branch, gindex = p.CurrentSyncCommitteeBranch, 86
			case *pb.LightClientBootstrapGloas:
				branch, gindex = p.CurrentSyncCommitteeBranch, 2945
				checkConstructionHeader(t, bootstrap.Header(), block)
			default:
				t.Fatalf("unexpected bootstrap %T", p)
			}
			require.Equal(t, true, trie.VerifyMerkleProof(bootstrap.Header().Beacon().StateRoot, committeeRoot[:], gindex, branch))
			headerRoot, err := bootstrap.Header().Beacon().HashTreeRoot()
			require.NoError(t, err)
			blockRoot, err := block.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, blockRoot, headerRoot)
			_, err = bootstrap.MarshalSSZ()
			require.NoError(t, err)
		})
	}
}

func TestLightClientConstruction_GloasRanking(t *testing.T) {
	setupConstructionConfig(t)
	block := constructionBlock(t, version.Gloas, 7*fieldparams.SlotsPerEpoch)
	for _, field := range []string{"committee", "finality"} {
		t.Run(field, func(t *testing.T) {
			oldUpdate, err := CreateDefaultLightClientUpdate(block)
			require.NoError(t, err)
			newUpdate, err := CreateDefaultLightClientUpdate(block)
			require.NoError(t, err)
			oldUpdate.SetSignatureSlot(block.Block().Slot() + 1)
			newUpdate.SetSignatureSlot(block.Block().Slot() + 1)
			p := newUpdate.Proto().(*pb.LightClientUpdateGloas)
			if field == "committee" {
				p.NextSyncCommitteeBranch[10][31] = 1
				require.NoError(t, newUpdate.SetNextSyncCommitteeBranch(p.NextSyncCommitteeBranch))
			} else {
				p.FinalityBranch[8][31] = 1
				require.NoError(t, newUpdate.SetFinalityBranch(p.FinalityBranch))
			}
			better, err := IsBetterUpdate(newUpdate, oldUpdate)
			require.NoError(t, err)
			require.Equal(t, true, better)
			better, err = IsBetterUpdate(oldUpdate, newUpdate)
			require.NoError(t, err)
			require.Equal(t, false, better)
		})
	}
}

func TestLightClientConstruction_BootstrapInvalidInputs(t *testing.T) {
	setupConstructionConfig(t)
	for _, tt := range []struct {
		name   string
		mutate func(*testing.T, state.BeaconState, interfaces.SignedBeaconBlock)
		want   string
	}{
		{
			name: "before Altair",
			mutate: func(t *testing.T, st state.BeaconState, _ interfaces.SignedBeaconBlock) {
				require.NoError(t, st.SetSlot(0))
			},
			want: "light client bootstrap is not supported before Altair",
		},
		{
			name: "state is not the block post state",
			mutate: func(t *testing.T, st state.BeaconState, _ interfaces.SignedBeaconBlock) {
				require.NoError(t, st.SetSlot(st.Slot()+1))
			},
			want: "not equal to latest block header slot",
		},
		{
			name: "block does not match state",
			mutate: func(_ *testing.T, _ state.BeaconState, block interfaces.SignedBeaconBlock) {
				block.SetParentRoot(gloasHeaderTestRoot(1))
			},
			want: "not equal to block root",
		},
		{
			name: "malformed state header",
			mutate: func(t *testing.T, st state.BeaconState, _ interfaces.SignedBeaconBlock) {
				header := st.LatestBlockHeader()
				header.BodyRoot = []byte{1}
				require.NoError(t, st.SetLatestBlockHeader(header))
			},
			want: "could not get latest block header root",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st, block := constructionStateBlock(t, version.Gloas, 7*fieldparams.SlotsPerEpoch, [32]byte{}, nil)
			tt.mutate(t, st, block)
			bootstrap, err := NewLightClientBootstrapFromBeaconState(t.Context(), block.Block().Slot(), st, block)
			require.ErrorContains(t, tt.want, err)
			require.Equal(t, nil, bootstrap)
		})
	}
}

func TestLightClientConstruction_BranchPresence(t *testing.T) {
	setupConstructionConfig(t)
	for _, tt := range []struct {
		fork           int
		committeeDepth int
		finalityDepth  int
	}{
		{version.Altair, 5, 6},
		{version.Electra, 6, 7},
		{version.Gloas, 11, 9},
	} {
		t.Run(version.String(tt.fork), func(t *testing.T) {
			block := constructionBlock(t, tt.fork, primitives.Slot(tt.fork*fieldparams.SlotsPerEpoch))
			update, err := CreateDefaultLightClientUpdate(block)
			require.NoError(t, err)
			hasCommittee, err := HasRelevantSyncCommittee(update)
			require.NoError(t, err)
			require.Equal(t, false, hasCommittee)
			hasFinality, err := HasFinality(update)
			require.NoError(t, err)
			require.Equal(t, false, hasFinality)
			branch := func(depth int) [][]byte {
				proof := make([][]byte, depth)
				for i := range proof {
					proof[i] = make([]byte, 32)
				}
				proof[depth-1][31] = 1
				return proof
			}
			require.NoError(t, update.SetNextSyncCommitteeBranch(branch(tt.committeeDepth)))
			require.NoError(t, update.SetFinalityBranch(branch(tt.finalityDepth)))
			hasCommittee, err = HasRelevantSyncCommittee(update)
			require.NoError(t, err)
			require.Equal(t, true, hasCommittee)
			hasFinality, err = HasFinality(update)
			require.NoError(t, err)
			require.Equal(t, true, hasFinality)
		})
	}
}

func setupConstructionConfig(t *testing.T) {
	t.Helper()
	cfg := params.MainnetConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 1, 2, 3
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch = 4, 5, 6, 7
	params.SetActiveTestCleanup(t, cfg)
}

func constructionBlock(t *testing.T, fork int, slot primitives.Slot) interfaces.SignedBeaconBlock {
	t.Helper()
	constructors := []func() any{
		func() any { return util.NewBeaconBlock() },
		func() any { return util.NewBeaconBlockAltair() },
		func() any { return util.NewBeaconBlockBellatrix() },
		func() any { return util.NewBeaconBlockCapella() },
		func() any { return util.NewBeaconBlockDeneb() },
		func() any { return util.NewBeaconBlockElectra() },
		func() any { return util.NewBeaconBlockFulu() },
		func() any { return util.NewBeaconBlockGloas() },
	}
	block, err := blocks.NewSignedBeaconBlock(constructors[fork]())
	require.NoError(t, err)
	block.SetSlot(slot)
	populateGloasHeaderTestExecution(t, block)
	if fork >= version.Altair {
		aggregate := &pb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		}
		aggregate.SyncCommitteeBits.SetBitAt(0, true)
		require.NoError(t, block.SetSyncAggregate(aggregate))
	}
	return block
}

func constructionStateBlock(t *testing.T, fork int, slot primitives.Slot, parent [32]byte, finalized interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, interfaces.SignedBeaconBlock) {
	t.Helper()
	var st state.BeaconState
	var err error
	switch fork {
	case version.Altair:
		st, err = util.NewBeaconStateAltair()
	case version.Bellatrix:
		st, err = util.NewBeaconStateBellatrix()
	case version.Capella:
		st, err = util.NewBeaconStateCapella()
	case version.Deneb:
		st, err = util.NewBeaconStateDeneb()
	case version.Electra:
		st, err = util.NewBeaconStateElectra()
	case version.Fulu:
		st, err = util.NewBeaconStateFulu()
	case version.Gloas:
		st, err = util.NewBeaconStateGloas(func(p *pb.BeaconStateGloas) error {
			p.BuilderPendingPayments = p.BuilderPendingPayments[:2*fieldparams.SlotsPerEpoch]
			p.ExecutionPayloadAvailability = make([]byte, params.BeaconConfig().SlotsPerHistoricalRoot/8)
			return nil
		})
	default:
		t.Fatalf("unsupported state fork %d", fork)
	}
	require.NoError(t, err)
	if fork >= version.Fulu {
		require.NoError(t, st.SetProposerLookahead(make([]primitives.ValidatorIndex, 2*fieldparams.SlotsPerEpoch)))
	}
	committee := func(value byte) *pb.SyncCommittee {
		keys := make([][]byte, fieldparams.SyncCommitteeLength)
		for i := range keys {
			keys[i] = make([]byte, fieldparams.BLSPubkeyLength)
			keys[i][0] = value
		}
		return &pb.SyncCommittee{Pubkeys: keys, AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength)}
	}
	require.NoError(t, st.SetCurrentSyncCommittee(committee(1)))
	require.NoError(t, st.SetNextSyncCommittee(committee(2)))
	require.NoError(t, st.SetSlot(slot))
	if finalized != nil && finalized.Block().Slot() != 0 {
		root, err := finalized.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, st.SetFinalizedCheckpoint(&pb.Checkpoint{Epoch: slots.ToEpoch(finalized.Block().Slot()), Root: root[:]}))
	}
	block := constructionBlock(t, fork, slot)
	block.SetParentRoot(parent[:])
	header, err := block.Header()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(header.Header))
	root, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)
	block.SetStateRoot(root[:])
	return st, block
}

func checkConstructionHeader(t *testing.T, header interfaces.LightClientHeader, block interfaces.ReadOnlySignedBeaconBlock) {
	t.Helper()
	root, err := block.Block().HashTreeRoot()
	require.NoError(t, err)
	headerRoot, err := header.Beacon().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, root, headerRoot)
	p := header.Proto().(*pb.LightClientHeaderGloas)
	if block.Version() < version.Capella {
		require.DeepEqual(t, make([]byte, 32), p.ExecutionBlockHash)
		require.DeepEqual(t, emptyGloasExecutionProof(), p.ExecutionBranch)
		return
	}
	var gindex uint64 = 812
	depth := 9
	if block.Version() == version.Capella {
		gindex, depth = 412, 8
	} else if block.Version() == version.Gloas {
		gindex, depth = 2856, 11
	}
	for _, node := range p.ExecutionBranch[:11-depth] {
		require.DeepEqual(t, make([]byte, 32), node)
	}
	require.Equal(t, true, trie.VerifyMerkleProof(p.Beacon.BodyRoot, p.ExecutionBlockHash, gindex, p.ExecutionBranch[11-depth:]))
}
