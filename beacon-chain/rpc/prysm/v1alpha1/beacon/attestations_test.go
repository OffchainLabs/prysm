package beacon

import (
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	dbTest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/cmd"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestServer_mapAttestationToTargetRoot(t *testing.T) {
	count := primitives.Slot(100)
	atts := make([]ethpb.Att, count)
	targetRoot1 := bytesutil.ToBytes32([]byte("root1"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	for i := range count {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		atts[i] = &ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Target: &ethpb.Checkpoint{
					Root: targetRoot[:],
				},
			},
			AggregationBits: bitfield.Bitlist{0b11},
		}

	}
	mappedAtts := mapAttestationsByTargetRoot(atts)
	wantedMapLen := 2
	wantedMapNumberOfElements := 50
	assert.Equal(t, wantedMapLen, len(mappedAtts), "Unexpected mapped attestations length")
	assert.Equal(t, wantedMapNumberOfElements, len(mappedAtts[targetRoot1]), "Unexpected number of attestations per block root")
	assert.Equal(t, wantedMapNumberOfElements, len(mappedAtts[targetRoot2]), "Unexpected number of attestations per block root")
}

func TestServer_ListIndexedAttestations_GenesisEpoch(t *testing.T) {
	db := dbTest.SetupDB(t)
	helpers.ClearCache()
	ctx := t.Context()
	targetRoot1 := bytesutil.ToBytes32([]byte("root"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	count := params.BeaconConfig().SlotsPerEpoch
	atts := make([]*ethpb.Attestation, 0, count)
	atts2 := make([]*ethpb.Attestation, 0, count)

	for i := range count {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		blockExample := util.NewBeaconBlock()
		blockExample.Block.Body.Attestations = []*ethpb.Attestation{
			{
				Signature: make([]byte, fieldparams.BLSSignatureLength),
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Target: &ethpb.Checkpoint{
						Root: targetRoot[:],
					},
					Source: &ethpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Slot:           i,
					CommitteeIndex: 0,
				},
				AggregationBits: bitfield.NewBitlist(128 / uint64(params.BeaconConfig().SlotsPerEpoch)),
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		if i%2 == 0 {
			atts = append(atts, blockExample.Block.Body.Attestations...)
		} else {
			atts2 = append(atts2, blockExample.Block.Body.Attestations...)
		}

	}

	// We setup 512 validators so that committee size matches the length of attestations' aggregation bits.
	numValidators := uint64(512)
	state, _ := util.DeterministicGenesisState(t, numValidators)

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*ethpb.IndexedAttestation, len(atts)+len(atts2))
	for i := 0; i < len(atts); i++ {
		att := atts[i]
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		a, ok := idxAtt.(*ethpb.IndexedAttestation)
		require.Equal(t, true, ok, "unexpected type of indexed attestation")
		indexedAtts[i] = a
	}
	for i := 0; i < len(atts2); i++ {
		att := atts2[i]
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts2[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		a, ok := idxAtt.(*ethpb.IndexedAttestation)
		require.Equal(t, true, ok, "unexpected type of indexed attestation")
		indexedAtts[i+len(atts)] = a
	}

	bs := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &chainMock.ChainService{State: state},
		HeadFetcher:        &chainMock.ChainService{State: state},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
	}
	err := db.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: targetRoot1[:],
		Slot: 1,
	})
	require.NoError(t, err)

	err = db.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: targetRoot2[:],
		Slot: 2,
	})
	require.NoError(t, err)

	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot1[:])))
	require.NoError(t, state.SetSlot(state.Slot()+1))
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot2[:])))
	res, err := bs.ListIndexedAttestations(ctx, &ethpb.ListIndexedAttestationsRequest{
		QueryFilter: &ethpb.ListIndexedAttestationsRequest_GenesisEpoch{
			GenesisEpoch: true,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, len(indexedAtts), len(res.IndexedAttestations), "Incorrect indexted attestations length")
	sort.Slice(indexedAtts, func(i, j int) bool {
		return indexedAtts[i].GetData().Slot < indexedAtts[j].GetData().Slot
	})
	sort.Slice(res.IndexedAttestations, func(i, j int) bool {
		return res.IndexedAttestations[i].Data.Slot < res.IndexedAttestations[j].Data.Slot
	})

	assert.DeepEqual(t, indexedAtts, res.IndexedAttestations, "Incorrect list indexed attestations response")
}

func TestServer_ListIndexedAttestations_OldEpoch(t *testing.T) {
	db := dbTest.SetupDB(t)
	helpers.ClearCache()
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root"))
	count := params.BeaconConfig().SlotsPerEpoch
	atts := make([]*ethpb.Attestation, 0, count)
	epoch := primitives.Epoch(50)
	startSlot, err := slots.EpochStart(epoch)
	require.NoError(t, err)

	for i := startSlot; i < count; i++ {
		blockExample := &ethpb.SignedBeaconBlock{
			Block: &ethpb.BeaconBlock{
				Body: &ethpb.BeaconBlockBody{
					Attestations: []*ethpb.Attestation{
						{
							Data: &ethpb.AttestationData{
								BeaconBlockRoot: blockRoot[:],
								Slot:            i,
								CommitteeIndex:  0,
								Target: &ethpb.Checkpoint{
									Epoch: epoch,
									Root:  make([]byte, fieldparams.RootLength),
								},
							},
							AggregationBits: bitfield.Bitlist{0b11},
						},
					},
				},
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	// We setup 128 validators.
	numValidators := uint64(128)
	state, _ := util.DeterministicGenesisState(t, numValidators)

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = make([]byte, fieldparams.RootLength)
	}
	require.NoError(t, state.SetRandaoMixes(randaoMixes))
	require.NoError(t, state.SetSlot(startSlot))

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*ethpb.IndexedAttestation, len(atts))
	for i := 0; i < len(atts); i++ {
		att := atts[i]
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		a, ok := idxAtt.(*ethpb.IndexedAttestation)
		require.Equal(t, true, ok, "unexpected type of indexed attestation")
		indexedAtts[i] = a
	}

	bs := &Server{
		BeaconDB: db,
		GenesisTimeFetcher: &chainMock.ChainService{
			Genesis: time.Now(),
		},
		StateGen: stategen.New(db, doublylinkedtree.New()),
	}
	err = db.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: blockRoot[:],
		Slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epoch)),
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32([]byte("root"))))
	res, err := bs.ListIndexedAttestations(ctx, &ethpb.ListIndexedAttestationsRequest{
		QueryFilter: &ethpb.ListIndexedAttestationsRequest_Epoch{
			Epoch: epoch,
		},
	})
	require.NoError(t, err)
	require.DeepEqual(t, indexedAtts, res.IndexedAttestations, "Incorrect list indexed attestations response")
}

func TestServer_ListIndexedAttestationsElectra(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.ElectraForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	db := dbTest.SetupDB(t)
	helpers.ClearCache()
	ctx := t.Context()
	targetRoot1 := bytesutil.ToBytes32([]byte("root"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	count := params.BeaconConfig().SlotsPerEpoch
	atts := make([]*ethpb.AttestationElectra, 0, count)
	atts2 := make([]*ethpb.AttestationElectra, 0, count)

	for i := range count {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		blockExample := util.NewBeaconBlockElectra()
		ab := bitfield.NewBitlist(128 / uint64(params.BeaconConfig().SlotsPerEpoch))
		ab.SetBitAt(0, true)
		blockExample.Block.Body.Attestations = []*ethpb.AttestationElectra{
			{
				Signature: make([]byte, fieldparams.BLSSignatureLength),
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Target: &ethpb.Checkpoint{
						Root: targetRoot[:],
					},
					Source: &ethpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Slot: i,
				},
				AggregationBits: ab,
				CommitteeBits:   cb,
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		if i%2 == 0 {
			atts = append(atts, blockExample.Block.Body.Attestations...)
		} else {
			atts2 = append(atts2, blockExample.Block.Body.Attestations...)
		}

	}

	// We setup 512 validators so that committee size matches the length of attestations' aggregation bits.
	numValidators := uint64(512)
	state, _ := util.DeterministicGenesisStateElectra(t, numValidators)

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*ethpb.IndexedAttestationElectra, len(atts)+len(atts2))
	for i := 0; i < len(atts); i++ {
		att := atts[i]
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), state, att.Data.Slot, 0)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		a, ok := idxAtt.(*ethpb.IndexedAttestationElectra)
		require.Equal(t, true, ok, "unexpected type of indexed attestation")
		indexedAtts[i] = a
	}
	for i := 0; i < len(atts2); i++ {
		att := atts2[i]
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), state, att.Data.Slot, 0)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts2[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		a, ok := idxAtt.(*ethpb.IndexedAttestationElectra)
		require.Equal(t, true, ok, "unexpected type of indexed attestation")
		indexedAtts[i+len(atts)] = a
	}

	bs := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &chainMock.ChainService{State: state},
		HeadFetcher:        &chainMock.ChainService{State: state},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
	}
	err := db.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: targetRoot1[:],
		Slot: 1,
	})
	require.NoError(t, err)

	err = db.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: targetRoot2[:],
		Slot: 2,
	})
	require.NoError(t, err)

	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot1[:])))
	require.NoError(t, state.SetSlot(state.Slot()+1))
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot2[:])))
	res, err := bs.ListIndexedAttestationsElectra(ctx, &ethpb.ListIndexedAttestationsRequest{
		QueryFilter: &ethpb.ListIndexedAttestationsRequest_Epoch{
			Epoch: 0,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, len(indexedAtts), len(res.IndexedAttestations), "Incorrect indexted attestations length")
	sort.Slice(indexedAtts, func(i, j int) bool {
		return indexedAtts[i].GetData().Slot < indexedAtts[j].GetData().Slot
	})
	sort.Slice(res.IndexedAttestations, func(i, j int) bool {
		return res.IndexedAttestations[i].Data.Slot < res.IndexedAttestations[j].Data.Slot
	})

	assert.DeepEqual(t, indexedAtts, res.IndexedAttestations, "Incorrect list indexed attestations response")
}

func TestServer_AttestationPool_Pagination_ExceedsMaxPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &ethpb.AttestationPoolRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_OutOfRange(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := []ethpb.Att{
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: bytesutil.PadTo([]byte{1}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{1}, fieldparams.BLSSignatureLength),
		},
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            2,
				BeaconBlockRoot: bytesutil.PadTo([]byte{2}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{2}, fieldparams.BLSSignatureLength),
		},
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            3,
				BeaconBlockRoot: bytesutil.PadTo([]byte{3}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{3}, fieldparams.BLSSignatureLength),
		},
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{
		PageToken: strconv.Itoa(1),
		PageSize:  100,
	}
	wanted := fmt.Sprintf("page start %d >= list %d", req.PageSize, len(atts))
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_DefaultPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := make([]ethpb.Att, params.BeaconConfig().DefaultPageSize+1)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{}
	res, err := bs.AttestationPool(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DefaultPageSize, len(res.Attestations), "Unexpected number of attestations")
	assert.Equal(t, params.BeaconConfig().DefaultPageSize+1, int(res.TotalSize), "Unexpected total size")
}

func TestServer_AttestationPool_Pagination_CustomPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	numAtts := 100
	atts := make([]ethpb.Att, numAtts)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))
	tests := []struct {
		req *ethpb.AttestationPoolRequest
		res *ethpb.AttestationPoolResponse
	}{
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "2",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(3),
				PageSize:  30,
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(0),
				PageSize:  int32(numAtts),
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
	}
	for _, tt := range tests {
		res, err := bs.AttestationPool(ctx, tt.req)
		require.NoError(t, err)
		assert.Equal(t, tt.res.TotalSize, res.TotalSize, "Unexpected total size")
		assert.Equal(t, tt.res.NextPageToken, res.NextPageToken, "Unexpected next page token")
	}
}

func TestServer_AttestationPoolElectra(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := make([]ethpb.Att, params.BeaconConfig().DefaultPageSize+1)
	for i := range atts {
		att := util.NewAttestationElectra()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{}
	res, err := bs.AttestationPoolElectra(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DefaultPageSize, len(res.Attestations), "Unexpected number of attestations")
	assert.Equal(t, params.BeaconConfig().DefaultPageSize+1, int(res.TotalSize), "Unexpected total size")
}
