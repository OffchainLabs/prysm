package test_helpers

import (
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func GenerateProtoGloasBeaconBlock() *ethpb.BeaconBlockGloas {
	return &ethpb.BeaconBlockGloas{
		Slot:          1,
		ProposerIndex: 2,
		ParentRoot:    FillByteSlice(32, 3),
		StateRoot:     FillByteSlice(32, 4),
		Body: &ethpb.BeaconBlockBodyGloas{
			RandaoReveal: FillByteSlice(96, 5),
			Eth1Data: &ethpb.Eth1Data{
				DepositRoot:  FillByteSlice(32, 6),
				DepositCount: 7,
				BlockHash:    FillByteSlice(32, 8),
			},
			Graffiti: FillByteSlice(32, 9),
			ProposerSlashings: []*ethpb.ProposerSlashing{
				{
					Header_1: &ethpb.SignedBeaconBlockHeader{
						Header: &ethpb.BeaconBlockHeader{
							Slot:          10,
							ProposerIndex: 11,
							ParentRoot:    FillByteSlice(32, 12),
							StateRoot:     FillByteSlice(32, 13),
							BodyRoot:      FillByteSlice(32, 14),
						},
						Signature: FillByteSlice(96, 15),
					},
					Header_2: &ethpb.SignedBeaconBlockHeader{
						Header: &ethpb.BeaconBlockHeader{
							Slot:          16,
							ProposerIndex: 17,
							ParentRoot:    FillByteSlice(32, 18),
							StateRoot:     FillByteSlice(32, 19),
							BodyRoot:      FillByteSlice(32, 20),
						},
						Signature: FillByteSlice(96, 21),
					},
				},
			},
			AttesterSlashings: []*ethpb.AttesterSlashingElectra{},
			Attestations:      []*ethpb.AttestationElectra{},
			Deposits:          []*ethpb.Deposit{},
			VoluntaryExits:    []*ethpb.SignedVoluntaryExit{},
			SyncAggregate: &ethpb.SyncAggregate{
				SyncCommitteeBits:      FillByteSlice(64, 100),
				SyncCommitteeSignature: FillByteSlice(96, 101),
			},
			BlsToExecutionChanges: []*ethpb.SignedBLSToExecutionChange{},
			SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
				Message: &ethpb.ExecutionPayloadBid{
					ParentBlockHash:    FillByteSlice(32, 110),
					ParentBlockRoot:    FillByteSlice(32, 111),
					BlockHash:          FillByteSlice(32, 112),
					PrevRandao:         FillByteSlice(32, 113),
					FeeRecipient:       FillByteSlice(20, 114),
					GasLimit:           120,
					BuilderIndex:       121,
					Slot:               1,
					Value:              123,
					ExecutionPayment:   124,
					BlobKzgCommitments: [][]byte{},
				},
				Signature: FillByteSlice(96, 130),
			},
			PayloadAttestations: []*ethpb.PayloadAttestation{},
		},
	}
}

func GenerateJsonGloasBeaconBlock() *structs.BeaconBlockGloas {
	return &structs.BeaconBlockGloas{
		Slot:          "1",
		ProposerIndex: "2",
		ParentRoot:    hexutil.Encode(FillByteSlice(32, 3)),
		StateRoot:     hexutil.Encode(FillByteSlice(32, 4)),
		Body: &structs.BeaconBlockBodyGloas{
			RandaoReveal: hexutil.Encode(FillByteSlice(96, 5)),
			Eth1Data: &structs.Eth1Data{
				DepositRoot:  hexutil.Encode(FillByteSlice(32, 6)),
				DepositCount: "7",
				BlockHash:    hexutil.Encode(FillByteSlice(32, 8)),
			},
			Graffiti: hexutil.Encode(FillByteSlice(32, 9)),
			ProposerSlashings: []*structs.ProposerSlashing{
				{
					SignedHeader1: &structs.SignedBeaconBlockHeader{
						Message: &structs.BeaconBlockHeader{
							Slot:          "10",
							ProposerIndex: "11",
							ParentRoot:    hexutil.Encode(FillByteSlice(32, 12)),
							StateRoot:     hexutil.Encode(FillByteSlice(32, 13)),
							BodyRoot:      hexutil.Encode(FillByteSlice(32, 14)),
						},
						Signature: hexutil.Encode(FillByteSlice(96, 15)),
					},
					SignedHeader2: &structs.SignedBeaconBlockHeader{
						Message: &structs.BeaconBlockHeader{
							Slot:          "16",
							ProposerIndex: "17",
							ParentRoot:    hexutil.Encode(FillByteSlice(32, 18)),
							StateRoot:     hexutil.Encode(FillByteSlice(32, 19)),
							BodyRoot:      hexutil.Encode(FillByteSlice(32, 20)),
						},
						Signature: hexutil.Encode(FillByteSlice(96, 21)),
					},
				},
			},
			AttesterSlashings: []*structs.AttesterSlashingElectra{},
			Attestations:      []*structs.AttestationElectra{},
			Deposits:          []*structs.Deposit{},
			VoluntaryExits:    []*structs.SignedVoluntaryExit{},
			SyncAggregate: &structs.SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(FillByteSlice(64, 100)),
				SyncCommitteeSignature: hexutil.Encode(FillByteSlice(96, 101)),
			},
			BLSToExecutionChanges: []*structs.SignedBLSToExecutionChange{},
			SignedExecutionPayloadBid: &structs.SignedExecutionPayloadBid{
				Message: &structs.ExecutionPayloadBid{
					ParentBlockHash:    hexutil.Encode(FillByteSlice(32, 110)),
					ParentBlockRoot:    hexutil.Encode(FillByteSlice(32, 111)),
					BlockHash:          hexutil.Encode(FillByteSlice(32, 112)),
					PrevRandao:         hexutil.Encode(FillByteSlice(32, 113)),
					FeeRecipient:       hexutil.Encode(FillByteSlice(20, 114)),
					GasLimit:           "120",
					BuilderIndex:       "121",
					Slot:               "1",
					Value:              "123",
					ExecutionPayment:   "124",
					BlobKzgCommitments: []string{},
				},
				Signature: hexutil.Encode(FillByteSlice(96, 130)),
			},
			PayloadAttestations: []*structs.PayloadAttestation{},
		},
	}
}

func GenerateProtoExecutionPayloadEnvelope() *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    FillByteSlice(32, 200),
			FeeRecipient:  FillByteSlice(20, 201),
			StateRoot:     FillByteSlice(32, 202),
			ReceiptsRoot:  FillByteSlice(32, 203),
			LogsBloom:     FillByteSlice(256, 204),
			PrevRandao:    FillByteSlice(32, 205),
			BaseFeePerGas: FillByteSlice(32, 206),
			BlockHash:     FillByteSlice(32, 207),
			ExtraData:     make([]byte, 0),
			SlotNumber:    1,
		},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		BuilderIndex:      121,
		BeaconBlockRoot:   FillByteSlice(32, 210),
	}
}
