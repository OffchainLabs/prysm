package util

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// NewBeaconStateHeze creates a beacon state with minimum marshalable fields.
func NewBeaconStateHeze(options ...func(state *ethpb.BeaconStateHeze) error) (state.BeaconState, error) {
	pubkeys := make([][]byte, fieldparams.SyncCommitteeLength)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 2*params.BeaconConfig().SlotsPerEpoch)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			},
		}
	}

	ptcWindow := make([]*ethpb.PTCs, 3*params.BeaconConfig().SlotsPerEpoch)
	for i := range ptcWindow {
		ptcWindow[i] = &ethpb.PTCs{
			ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.PTCSize),
		}
	}

	seed := &ethpb.BeaconStateHeze{
		BlockRoots:                 filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), 32),
		StateRoots:                 filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), 32),
		Slashings:                  make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		RandaoMixes:                filledByteSlice2D(uint64(params.BeaconConfig().EpochsPerHistoricalVector), 32),
		Validators:                 make([]*ethpb.Validator, 0),
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		},
		Eth1DataVotes: make([]*ethpb.Eth1Data, 0),
		Fork: &ethpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  make([]byte, 4),
		},
		HistoricalRoots:             make([][]byte, 0),
		JustificationBits:           bitfield.Bitvector4{0x0},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		LatestBlockHeader:           HydrateBeaconHeader(&ethpb.BeaconBlockHeader{}),
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		PreviousEpochParticipation:  make([]byte, 0),
		CurrentEpochParticipation:   make([]byte, 0),
		CurrentSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
		},
		NextSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
		},
		ProposerLookahead:            make([]primitives.ValidatorIndex, (params.BeaconConfig().MinSeedLookahead+1)*primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)),
		LatestExecutionPayloadBid:    HydrateExecutionPayloadBidHeze(nil),
		Builders:                     make([]*ethpb.Builder, 0),
		ExecutionPayloadAvailability: make([]byte, params.BeaconConfig().SlotsPerHistoricalRoot/8),
		BuilderPendingPayments:       builderPendingPayments,
		BuilderPendingWithdrawals:    make([]*ethpb.BuilderPendingWithdrawal, 0),
		LatestBlockHash:              make([]byte, 32),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		PtcWindow:                    ptcWindow,
	}

	for _, opt := range options {
		if err := opt(seed); err != nil {
			return nil, err
		}
	}

	return state_native.InitializeFromProtoUnsafeHeze(seed)
}

// NewBeaconBlockHeze creates a beacon block with minimum marshalable fields.
func NewBeaconBlockHeze() *ethpb.SignedBeaconBlockHeze {
	return HydrateSignedBeaconBlockHeze(&ethpb.SignedBeaconBlockHeze{})
}

// HydrateSignedBeaconBlockHeze hydrates a signed beacon block with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockHeze(b *ethpb.SignedBeaconBlockHeze) *ethpb.SignedBeaconBlockHeze {
	if b == nil {
		b = &ethpb.SignedBeaconBlockHeze{}
	}
	if b.Signature == nil {
		b.Signature = make([]byte, fieldparams.BLSSignatureLength)
	}
	b.Block = HydrateBeaconBlockHeze(b.Block)
	return b
}

// HydrateBeaconBlockHeze hydrates a beacon block with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateBeaconBlockHeze(b *ethpb.BeaconBlockHeze) *ethpb.BeaconBlockHeze {
	if b == nil {
		b = &ethpb.BeaconBlockHeze{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyHeze(b.Body)
	return b
}

// HydrateBeaconBlockBodyHeze hydrates a beacon block body with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyHeze(b *ethpb.BeaconBlockBodyHeze) *ethpb.BeaconBlockBodyHeze {
	if b == nil {
		b = &ethpb.BeaconBlockBodyHeze{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, fieldparams.BLSSignatureLength)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		}
	}
	if b.SignedExecutionPayloadBid == nil {
		b.SignedExecutionPayloadBid = &ethpb.SignedExecutionPayloadBidHeze{}
	}
	if b.SignedExecutionPayloadBid.Signature == nil {
		b.SignedExecutionPayloadBid.Signature = make([]byte, fieldparams.BLSSignatureLength)
	}
	b.SignedExecutionPayloadBid.Message = HydrateExecutionPayloadBidHeze(b.SignedExecutionPayloadBid.Message)
	if b.PayloadAttestations == nil {
		b.PayloadAttestations = make([]*ethpb.PayloadAttestation, 0)
	}
	if b.ParentExecutionRequests == nil {
		b.ParentExecutionRequests = &enginev1.ExecutionRequestsGloas{}
	}
	return b
}

// HydrateExecutionPayloadBidHeze hydrates an execution payload bid with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateExecutionPayloadBidHeze(b *ethpb.ExecutionPayloadBidHeze) *ethpb.ExecutionPayloadBidHeze {
	if b == nil {
		b = &ethpb.ExecutionPayloadBidHeze{}
	}
	g := HydrateExecutionPayloadBid(&ethpb.ExecutionPayloadBid{
		ParentBlockHash:       b.ParentBlockHash,
		ParentBlockRoot:       b.ParentBlockRoot,
		BlockHash:             b.BlockHash,
		PrevRandao:            b.PrevRandao,
		FeeRecipient:          b.FeeRecipient,
		BlobKzgCommitments:    b.BlobKzgCommitments,
		ExecutionRequestsRoot: b.ExecutionRequestsRoot,
	})
	b.ParentBlockHash = g.ParentBlockHash
	b.ParentBlockRoot = g.ParentBlockRoot
	b.BlockHash = g.BlockHash
	b.PrevRandao = g.PrevRandao
	b.FeeRecipient = g.FeeRecipient
	b.BlobKzgCommitments = g.BlobKzgCommitments
	b.ExecutionRequestsRoot = g.ExecutionRequestsRoot
	if b.InclusionListBits == nil {
		b.InclusionListBits = bitfield.NewBitvector16()
	}
	return b
}
