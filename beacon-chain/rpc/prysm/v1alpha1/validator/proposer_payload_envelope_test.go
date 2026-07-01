package validator

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testGloasBlock(t *testing.T) (*consensusblocks.GetPayloadResponse, interfaces.SignedBeaconBlock) {
	t.Helper()

	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:      make([]byte, 32),
		FeeRecipient:    make([]byte, 20),
		StateRoot:       make([]byte, 32),
		ReceiptsRoot:    make([]byte, 32),
		LogsBloom:       make([]byte, 256),
		PrevRandao:      make([]byte, 32),
		BaseFeePerGas:   make([]byte, 32),
		BlockHash:       make([]byte, 32),
		ExtraData:       make([]byte, 0),
		BlockAccessList: []byte{0xc0},
	}
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	sBlk, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)

	return local, sBlk
}

func TestStoreExecutionPayloadEnvelope(t *testing.T) {
	local, sBlk := testGloasBlock(t)

	vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	envelope, err := vs.storeExecutionPayloadEnvelope(sBlk, local)
	require.NoError(t, err)
	require.Equal(t, sBlk.Block().Slot(), envelope.Payload.SlotNumber)

	contents, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
	require.Equal(t, true, ok)
	require.Equal(t, sBlk.Block().Slot(), contents.Envelope.Payload.SlotNumber)
}

func TestStoreExecutionPayloadEnvelope_RejectsEmptyBlockAccessList(t *testing.T) {
	local, sBlk := testGloasBlock(t)
	local.ExecutionData.Proto().(*enginev1.ExecutionPayloadGloas).BlockAccessList = nil

	vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	err := vs.storeExecutionPayloadEnvelope(sBlk, local)
	require.ErrorContains(t, "block access list cannot be empty", err)
}

func TestExtractExecutionPayloadGloas(t *testing.T) {
	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:      make([]byte, 32),
		FeeRecipient:    make([]byte, 20),
		StateRoot:       make([]byte, 32),
		ReceiptsRoot:    make([]byte, 32),
		LogsBloom:       make([]byte, 256),
		PrevRandao:      make([]byte, 32),
		BaseFeePerGas:   make([]byte, 32),
		BlockHash:       make([]byte, 32),
		ExtraData:       make([]byte, 0),
		BlockAccessList: []byte{0xc0},
	}
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData: ed,
		Bid:           big.NewInt(0),
	}

	result := extractExecutionPayloadGloas(local)
	require.NotNil(t, result)
	require.DeepEqual(t, payload, result)
}

func TestExtractExecutionPayloadGloas_Nil(t *testing.T) {
	require.Equal(t, true, extractExecutionPayloadGloas(nil) == nil)
	require.Equal(t, true, extractExecutionPayloadGloas(&consensusblocks.GetPayloadResponse{}) == nil)
}

func TestGetExecutionPayloadEnvelopeRPC_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "request cannot be nil", err)
}

func TestGetExecutionPayloadEnvelopeRPC_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: 0, // epoch 0, before GloasForkEpoch 10
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
}

func TestPublishExecutionPayloadEnvelope_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "must set contents or blinded", err)

	_, err = vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{}},
		},
	})
	require.ErrorContains(t, "signed envelope or payload cannot be nil", err)
}

func TestPublishExecutionPayloadEnvelope_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{
				SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{
					Message: &ethpb.ExecutionPayloadEnvelope{
						Payload: &enginev1.ExecutionPayloadGloas{SlotNumber: 0}, // epoch 0, before GloasForkEpoch 10
					},
				},
			},
		},
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
}

func TestPublishExecutionPayloadEnvelope_StatelessContents_RejectsBadProofs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	require.NoError(t, kzg.Start())

	blobCount := 2
	rawBlobs := make([]kzg.Blob, blobCount)
	for i := range rawBlobs {
		rawBlobs[i] = kzg.Blob{uint8(i + 1)}
	}
	_, proofsPerBlob := util.GenerateCellsAndProofs(t, rawBlobs)

	flatBlobs := make([][]byte, blobCount)
	for i, b := range rawBlobs {
		flatBlobs[i] = b[:]
	}
	flatProofs := make([][]byte, 0, blobCount*fieldparams.NumberOfColumns)
	for _, proofs := range proofsPerBlob {
		for _, p := range proofs {
			flatProofs = append(flatProofs, p[:])
		}
	}
	// Corrupt the first proof — verifyCellProofs must reject before any P2P/cache/receiver is touched.
	flatProofs[0] = bytes.Repeat([]byte{0xff}, 48)

	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload:               &enginev1.ExecutionPayloadGloas{SlotNumber: 1},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{
				SignedExecutionPayloadEnvelope: signed,
				Blobs:                          flatBlobs,
				KzgProofs:                      flatProofs,
			},
		},
	})
	require.ErrorContains(t, "kzg verification failed", err)
}

func TestGetExecutionPayloadEnvelopeRPC_Success(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			SlotNumber:    1,
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          primitives.BuilderIndex(0),
		BeaconBlockRoot:       make([]byte, 32),
		ParentBeaconBlockRoot: make([]byte, 32),
	}

	vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: envelope})

	resp, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	// The RPC returns the blinded wire form; its HTR must equal the cached full envelope's HTR.
	require.NotNil(t, resp.Blinded)
	wantHTR, err := envelope.HashTreeRoot()
	require.NoError(t, err)
	gotHTR, err := resp.Blinded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, wantHTR, gotHTR)
}

func TestPublishExecutionPayloadEnvelope_Success(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	broadcaster := &mockp2p.MockBroadcaster{}
	receiver := &mockExecutionPayloadEnvelopeReceiver{}
	vs := &Server{
		P2P:                              broadcaster,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}

	req := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:      make([]byte, 32),
				FeeRecipient:    make([]byte, 20),
				StateRoot:       make([]byte, 32),
				ReceiptsRoot:    make([]byte, 32),
				LogsBloom:       make([]byte, 256),
				PrevRandao:      make([]byte, 32),
				BaseFeePerGas:   make([]byte, 32),
				BlockHash:       make([]byte, 32),
				ExtraData:       make([]byte, 0),
				BlockAccessList: []byte{0xc0},
				SlotNumber:      1,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BuilderIndex:          0,
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: req},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	require.Equal(t, 1, receiver.calls)
}

func TestPublishExecutionPayloadEnvelope_RejectsEmptyBlockAccessList(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{SlotNumber: 1},
		},
	})
	require.ErrorContains(t, "block access list cannot be empty", err)
}

func TestPublishExecutionPayloadEnvelope_ImportFailureIsAborted(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	broadcaster := &mockp2p.MockBroadcaster{}
	receiver := &mockExecutionPayloadEnvelopeReceiver{err: errors.New("import failed")}
	vs := &Server{
		P2P:                              broadcaster,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}

	req := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:      make([]byte, 32),
				FeeRecipient:    make([]byte, 20),
				StateRoot:       make([]byte, 32),
				ReceiptsRoot:    make([]byte, 32),
				LogsBloom:       make([]byte, 256),
				PrevRandao:      make([]byte, 32),
				BaseFeePerGas:   make([]byte, 32),
				BlockHash:       make([]byte, 32),
				ExtraData:       make([]byte, 0),
				BlockAccessList: []byte{0xc0},
				SlotNumber:      1,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: req},
		},
	})
	require.NotNil(t, err)
	// Broadcast must have happened before the import failure (spec 202).
	require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	require.Equal(t, codes.Aborted, status.Code(err))
}

type mockExecutionPayloadEnvelopeReceiver struct {
	calls int
	err   error
}

func (m *mockExecutionPayloadEnvelopeReceiver) ReceiveExecutionPayloadEnvelope(_ context.Context, _ interfaces.ROSignedExecutionPayloadEnvelope) error {
	m.calls++
	return m.err
}
