package mock

import (
	"encoding/json"
	"fmt"
	"strings"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager/remote-web3signer/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/go-bitfield"
)

/////////////////////////////////////////////////////////////////////////////////////////////////
//////////////// Mock Requests //////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////

func SyncComitteeBits() []byte {
	currSize := new(eth.SyncAggregate).SyncCommitteeBits.Len()
	switch currSize {
	case 512:
		return bitfield.NewBitvector512()
	case 32:
		return bitfield.NewBitvector32()
	default:
		return nil
	}
}

func AggregationBits() []byte {
	currSize := new(eth.SyncCommitteeContribution).AggregationBits.Len()
	switch currSize {
	case 128:
		return bitfield.NewBitvector128()
	case 8:
		return bitfield.NewBitvector8()
	default:
		return nil
	}
}

// GetMockSignRequest returns a mock SignRequest by type.
func GetMockSignRequest(t string) *validatorpb.SignRequest {
	switch t {
	case "AGGREGATION_SLOT":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_Slot{
				Slot: 0,
			},
			SigningSlot: 0,
		}
	case "AGGREGATE_AND_PROOF":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_AggregateAttestationAndProof{
				AggregateAttestationAndProof: &eth.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: &eth.Attestation{
						AggregationBits: bitfield.Bitlist{0b1101},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, 96),
					},
					SelectionProof: make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			SigningSlot: 0,
		}
	case "AGGREGATE_AND_PROOF_V2":
		committeeBits := bitfield.NewBitvector64()
		committeeBits.SetBitAt(0, true)
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_AggregateAttestationAndProofElectra{
				AggregateAttestationAndProofElectra: &eth.AggregateAttestationAndProofElectra{
					AggregatorIndex: 0,
					Aggregate: &eth.AttestationElectra{
						AggregationBits: bitfield.Bitlist{0b1101},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature:     make([]byte, 96),
						CommitteeBits: committeeBits,
					},
					SelectionProof: make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			SigningSlot: primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch),
		}
	case "ATTESTATION":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_AttestationData{
				AttestationData: &eth.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Source: &eth.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Target: &eth.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
				},
			},
			SigningSlot: 0,
		}
	case "BLOCK":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_Block{
				Block: &eth.BeaconBlock{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    make([]byte, fieldparams.RootLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					Body: &eth.BeaconBlockBody{
						RandaoReveal: make([]byte, 32),
						Eth1Data: &eth.Eth1Data{
							DepositRoot:  make([]byte, fieldparams.RootLength),
							DepositCount: 0,
							BlockHash:    make([]byte, 32),
						},
						Graffiti: make([]byte, 32),
						ProposerSlashings: []*eth.ProposerSlashing{
							{
								Header_1: &eth.SignedBeaconBlockHeader{
									Header: &eth.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
								Header_2: &eth.SignedBeaconBlockHeader{
									Header: &eth.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						AttesterSlashings: []*eth.AttesterSlashing{
							{
								Attestation_1: &eth.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &eth.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
								Attestation_2: &eth.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &eth.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						Attestations: []*eth.Attestation{
							{
								AggregationBits: bitfield.Bitlist{0b1101},
								Data: &eth.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &eth.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &eth.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, 96),
							},
						},
						Deposits: []*eth.Deposit{
							{
								Proof: [][]byte{[]byte("A")},
								Data: &eth.Deposit_Data{
									PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
									WithdrawalCredentials: make([]byte, 32),
									Amount:                0,
									Signature:             make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						VoluntaryExits: []*eth.SignedVoluntaryExit{
							{
								Exit: &eth.VoluntaryExit{
									Epoch:          0,
									ValidatorIndex: 0,
								},
								Signature: make([]byte, fieldparams.BLSSignatureLength),
							},
						},
					},
				},
			},
			SigningSlot: 0,
		}
	case "BLOCK_V2", "BLOCK_V2_ALTAIR":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockAltair{
				BlockAltair: &eth.BeaconBlockAltair{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    make([]byte, fieldparams.RootLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					Body: &eth.BeaconBlockBodyAltair{
						RandaoReveal: make([]byte, 32),
						Eth1Data: &eth.Eth1Data{
							DepositRoot:  make([]byte, fieldparams.RootLength),
							DepositCount: 0,
							BlockHash:    make([]byte, 32),
						},
						Graffiti: make([]byte, 32),
						ProposerSlashings: []*eth.ProposerSlashing{
							{
								Header_1: &eth.SignedBeaconBlockHeader{
									Header: &eth.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
								Header_2: &eth.SignedBeaconBlockHeader{
									Header: &eth.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						AttesterSlashings: []*eth.AttesterSlashing{
							{
								Attestation_1: &eth.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &eth.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
								Attestation_2: &eth.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &eth.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &eth.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						Attestations: []*eth.Attestation{
							{
								AggregationBits: bitfield.Bitlist{0b1101},
								Data: &eth.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &eth.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &eth.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, 96),
							},
						},
						Deposits: []*eth.Deposit{
							{
								Proof: [][]byte{[]byte("A")},
								Data: &eth.Deposit_Data{
									PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
									WithdrawalCredentials: make([]byte, 32),
									Amount:                0,
									Signature:             make([]byte, fieldparams.BLSSignatureLength),
								},
							},
						},
						VoluntaryExits: []*eth.SignedVoluntaryExit{
							{
								Exit: &eth.VoluntaryExit{
									Epoch:          0,
									ValidatorIndex: 0,
								},
								Signature: make([]byte, fieldparams.BLSSignatureLength),
							},
						},
						SyncAggregate: &eth.SyncAggregate{
							SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
							SyncCommitteeBits:      SyncComitteeBits(),
						},
					},
				},
			},
			SigningSlot: 0,
		}
	case "BLOCK_V2_BELLATRIX":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockBellatrix{
				BlockBellatrix: util.HydrateBeaconBlockBellatrix(&eth.BeaconBlockBellatrix{}),
			},
		}
	case "BLOCK_V2_BLINDED_BELLATRIX":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlindedBlockBellatrix{
				BlindedBlockBellatrix: util.HydrateBlindedBeaconBlockBellatrix(&eth.BlindedBeaconBlockBellatrix{}),
			},
		}
	case "BLOCK_V2_CAPELLA":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockCapella{
				BlockCapella: util.HydrateBeaconBlockCapella(&eth.BeaconBlockCapella{}),
			},
		}
	case "BLOCK_V2_BLINDED_CAPELLA":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlindedBlockCapella{
				BlindedBlockCapella: util.HydrateBlindedBeaconBlockCapella(&eth.BlindedBeaconBlockCapella{}),
			},
		}
	case "BLOCK_V2_DENEB":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockDeneb{
				BlockDeneb: util.HydrateBeaconBlockDeneb(&eth.BeaconBlockDeneb{}),
			},
		}
	case "BLOCK_V2_BLINDED_DENEB":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlindedBlockDeneb{
				BlindedBlockDeneb: util.HydrateBlindedBeaconBlockDeneb(&eth.BlindedBeaconBlockDeneb{}),
			},
		}
	case "BLOCK_V2_ELECTRA":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockElectra{
				BlockElectra: util.HydrateBeaconBlockElectra(&eth.BeaconBlockElectra{}),
			},
		}
	case "BLOCK_V2_BLINDED_ELECTRA":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlindedBlockElectra{
				BlindedBlockElectra: util.HydrateBlindedBeaconBlockElectra(&eth.BlindedBeaconBlockElectra{}),
			},
		}
	case "BLOCK_V2_FULU":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlockFulu{
				BlockFulu: util.HydrateBeaconBlockFulu(&eth.BeaconBlockElectra{}),
			},
		}
	case "BLOCK_V2_BLINDED_FULU":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_BlindedBlockFulu{
				BlindedBlockFulu: util.HydrateBlindedBeaconBlockFulu(&eth.BlindedBeaconBlockFulu{}),
			},
		}
	case "RANDAO_REVEAL":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_Epoch{
				Epoch: 0,
			},
			SigningSlot: 0,
		}
	case "SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_ContributionAndProof{
				ContributionAndProof: &eth.ContributionAndProof{
					AggregatorIndex: 0,
					Contribution: &eth.SyncCommitteeContribution{
						Slot:              0,
						BlockRoot:         make([]byte, fieldparams.RootLength),
						SubcommitteeIndex: 0,
						AggregationBits:   AggregationBits(),
						Signature:         make([]byte, fieldparams.BLSSignatureLength),
					},
					SelectionProof: make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			SigningSlot: 0,
		}
	case "SYNC_COMMITTEE_MESSAGE":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_SyncMessageBlockRoot{
				SyncMessageBlockRoot: make([]byte, fieldparams.RootLength),
			},
			SigningSlot: 0,
		}
	case "SYNC_COMMITTEE_SELECTION_PROOF":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_SyncAggregatorSelectionData{
				SyncAggregatorSelectionData: &eth.SyncAggregatorSelectionData{
					Slot:              0,
					SubcommitteeIndex: 0,
				},
			},
			SigningSlot: 0,
		}
	case "VOLUNTARY_EXIT":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_Exit{
				Exit: &eth.VoluntaryExit{
					Epoch:          0,
					ValidatorIndex: 0,
				},
			},
			SigningSlot: 0,
		}
	case "VALIDATOR_REGISTRATION":
		return &validatorpb.SignRequest{
			PublicKey:       make([]byte, fieldparams.BLSPubkeyLength),
			SigningRoot:     make([]byte, fieldparams.RootLength),
			SignatureDomain: make([]byte, 4),
			Object: &validatorpb.SignRequest_Registration{
				Registration: &eth.ValidatorRegistrationV1{
					FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
					GasLimit:     uint64(0),
					Timestamp:    uint64(0),
					Pubkey:       make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			SigningSlot: 0,
		}
	default:
		fmt.Printf("Web3signer sign request type: %v  not found", t)
		return nil
	}
}

// AggregationSlotSignRequest is a mock implementation of the AggregationSlotSignRequest.
func AggregationSlotSignRequest() *types.AggregationSlotSignRequest {
	return &types.AggregationSlotSignRequest{
		Type:            "AGGREGATION_SLOT",
		ForkInfo:        ForkInfo(),
		SigningRoot:     make([]byte, fieldparams.RootLength),
		AggregationSlot: &types.AggregationSlot{Slot: "0"},
	}
}

// AggregateAndProofV2SignRequest is a mock implementation of the AggregateAndProofV2SignRequest.
func AggregateAndProofV2SignRequest(ver int) *types.AggregateAndProofV2SignRequest {
	var aggregateAndProofJSON []byte
	var slot primitives.Slot
	if ver < version.Electra {
		aggregateAndProofData := &types.AggregateAndProof{
			AggregatorIndex: "0",
			Aggregate:       Attestation(),
			SelectionProof:  make([]byte, fieldparams.BLSSignatureLength),
		}
		aggregateAndProofJSON, _ = json.Marshal(aggregateAndProofData)
		slot = 0 // Pre-Electra slot
	} else {
		aggregateAndProofData := &types.AggregateAndProofElectra{
			AggregatorIndex: "0",
			Aggregate:       AttestationElectra(),
			SelectionProof:  make([]byte, fieldparams.BLSSignatureLength),
		}
		aggregateAndProofJSON, _ = json.Marshal(aggregateAndProofData)
		slot = primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	}
	// Generate ForkInfo dynamically based on the slot
	forkInfo, _ := types.MapForkInfo(slot, make([]byte, fieldparams.RootLength))
	return &types.AggregateAndProofV2SignRequest{
		Type:        "AGGREGATE_AND_PROOF_V2",
		ForkInfo:    forkInfo,
		SigningRoot: make([]byte, fieldparams.RootLength),
		AggregateAndProof: &types.AggregateAndProofV2{
			Version: strings.ToUpper(version.String(ver)),
			Data:    aggregateAndProofJSON,
		},
	}
}

// AttestationSignRequest is a mock implementation of the AttestationSignRequest.
func AttestationSignRequest() *types.AttestationSignRequest {
	return &types.AttestationSignRequest{
		Type:        "ATTESTATION",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		Attestation: Attestation().Data,
	}
}

// BlockSignRequest is a mock implementation of the BlockSignRequest.
func BlockSignRequest() *types.BlockSignRequest {
	return &types.BlockSignRequest{
		Type:        "BLOCK",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		Block: &types.BeaconBlock{
			Slot:          "0",
			ProposerIndex: "0",
			ParentRoot:    make([]byte, fieldparams.RootLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			Body:          BeaconBlockBody(),
		},
	}
}

// BlockV2AltairSignRequest is a mock implementation of the BlockAltairSignRequest.
func BlockV2AltairSignRequest() *types.BlockAltairSignRequest {
	return &types.BlockAltairSignRequest{
		Type:        "BLOCK_V2",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		BeaconBlock: &types.BeaconBlockAltairBlockV2{
			Version: "ALTAIR",
			Block:   BeaconBlockAltair(),
		},
	}
}

func BlockV2BlindedSignRequest(bodyRoot []byte, version string) *types.BlockV2BlindedSignRequest {
	return &types.BlockV2BlindedSignRequest{
		Type:        "BLOCK_V2",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		BeaconBlock: &types.BeaconBlockV2Blinded{
			Version: version,
			BlockHeader: &types.BeaconBlockHeader{
				Slot:          "0",
				ProposerIndex: "0",
				ParentRoot:    make([]byte, fieldparams.RootLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				BodyRoot:      bodyRoot,
			},
		},
	}
}

// RandaoRevealSignRequest is a mock implementation of the RandaoRevealSignRequest.
func RandaoRevealSignRequest() *types.RandaoRevealSignRequest {
	return &types.RandaoRevealSignRequest{
		Type:        "RANDAO_REVEAL",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		RandaoReveal: &types.RandaoReveal{
			Epoch: "0",
		},
	}
}

// SyncCommitteeContributionAndProofSignRequest is a mock implementation of the SyncCommitteeContributionAndProofSignRequest.
func SyncCommitteeContributionAndProofSignRequest() *types.SyncCommitteeContributionAndProofSignRequest {
	return &types.SyncCommitteeContributionAndProofSignRequest{
		Type:                 "SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF",
		ForkInfo:             ForkInfo(),
		SigningRoot:          make([]byte, fieldparams.RootLength),
		ContributionAndProof: ContributionAndProof(),
	}
}

// SyncCommitteeMessageSignRequest is a mock implementation of the SyncCommitteeMessageSignRequest.
func SyncCommitteeMessageSignRequest() *types.SyncCommitteeMessageSignRequest {
	return &types.SyncCommitteeMessageSignRequest{
		Type:        "SYNC_COMMITTEE_MESSAGE",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		SyncCommitteeMessage: &types.SyncCommitteeMessage{
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Slot:            "0",
		},
	}
}

// SyncCommitteeSelectionProofSignRequest is a mock implementation of the SyncCommitteeSelectionProofSignRequest.
func SyncCommitteeSelectionProofSignRequest() *types.SyncCommitteeSelectionProofSignRequest {
	return &types.SyncCommitteeSelectionProofSignRequest{
		Type:        "SYNC_COMMITTEE_SELECTION_PROOF",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		SyncAggregatorSelectionData: &types.SyncAggregatorSelectionData{
			Slot:              "0",
			SubcommitteeIndex: "0",
		},
	}
}

// VoluntaryExitSignRequest is a mock implementation of the VoluntaryExitSignRequest.
func VoluntaryExitSignRequest() *types.VoluntaryExitSignRequest {
	return &types.VoluntaryExitSignRequest{
		Type:        "VOLUNTARY_EXIT",
		ForkInfo:    ForkInfo(),
		SigningRoot: make([]byte, fieldparams.RootLength),
		VoluntaryExit: &types.VoluntaryExit{
			Epoch:          "0",
			ValidatorIndex: "0",
		},
	}
}

// ValidatorRegistrationSignRequest is a mock implementation of the ValidatorRegistrationSignRequest.
func ValidatorRegistrationSignRequest() *types.ValidatorRegistrationSignRequest {
	return &types.ValidatorRegistrationSignRequest{
		Type:        "VALIDATOR_REGISTRATION",
		SigningRoot: make([]byte, fieldparams.RootLength),
		ValidatorRegistration: &types.ValidatorRegistration{
			FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			GasLimit:     fmt.Sprint(0),
			Timestamp:    fmt.Sprint(0),
			Pubkey:       make([]byte, fieldparams.BLSSignatureLength),
		},
	}
}

/////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////

// ForkInfo is a mock implementation of the ForkInfo.
func ForkInfo() *types.ForkInfo {
	return &types.ForkInfo{
		Fork: &types.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  make([]byte, 4),
			Epoch:           "0",
		},
		GenesisValidatorsRoot: make([]byte, fieldparams.RootLength),
	}
}

// Attestation is a mock implementation of the Attestation.
func Attestation() *types.Attestation {
	return &types.Attestation{
		AggregationBits: []byte(bitfield.Bitlist{0b1101}),
		Data: &types.AttestationData{
			Slot:            "0",
			Index:           "0",
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
			Target: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
}

// AttestationElectra is a mock implementation of the AttestationElectra.
func AttestationElectra() *types.AttestationElectra {
	committeeBits := bitfield.NewBitvector64()
	committeeBits.SetBitAt(0, true)
	return &types.AttestationElectra{
		AggregationBits: []byte(bitfield.Bitlist{0b1101}),
		Data: &types.AttestationData{
			Slot:            "0",
			Index:           "0",
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
			Target: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
		},
		Signature:     make([]byte, fieldparams.BLSSignatureLength),
		CommitteeBits: []byte(committeeBits),
	}
}

func IndexedAttestation() *types.IndexedAttestation {
	return &types.IndexedAttestation{
		AttestingIndices: []string{"0", "1", "2"},
		Data: &types.AttestationData{
			Slot:            "0",
			Index:           "0",
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
			Target: &types.Checkpoint{
				Epoch: "0",
				Root:  hexutil.Encode(make([]byte, fieldparams.RootLength)),
			},
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
}

func BeaconBlockAltair() *types.BeaconBlockAltair {
	return &types.BeaconBlockAltair{
		Slot:          "0",
		ProposerIndex: "0",
		ParentRoot:    make([]byte, fieldparams.RootLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		Body: &types.BeaconBlockBodyAltair{
			RandaoReveal: make([]byte, 32),
			Eth1Data: &types.Eth1Data{
				DepositRoot:  make([]byte, fieldparams.RootLength),
				DepositCount: "0",
				BlockHash:    make([]byte, 32),
			},
			Graffiti: make([]byte, 32),
			ProposerSlashings: []*types.ProposerSlashing{
				{
					Signedheader1: &types.SignedBeaconBlockHeader{
						Message: &types.BeaconBlockHeader{
							Slot:          "0",
							ProposerIndex: "0",
							ParentRoot:    make([]byte, fieldparams.RootLength),
							StateRoot:     make([]byte, fieldparams.RootLength),
							BodyRoot:      make([]byte, fieldparams.RootLength),
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
					Signedheader2: &types.SignedBeaconBlockHeader{
						Message: &types.BeaconBlockHeader{
							Slot:          "0",
							ProposerIndex: "0",
							ParentRoot:    make([]byte, fieldparams.RootLength),
							StateRoot:     make([]byte, fieldparams.RootLength),
							BodyRoot:      make([]byte, fieldparams.RootLength),
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
				},
			},
			AttesterSlashings: []*types.AttesterSlashing{
				{
					Attestation1: IndexedAttestation(),
					Attestation2: IndexedAttestation(),
				},
			},
			Attestations: []*types.Attestation{
				Attestation(),
			},
			Deposits: []*types.Deposit{
				{
					Proof: []string{"0x41"},
					Data: &types.DepositData{
						PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
						WithdrawalCredentials: make([]byte, 32),
						Amount:                "0",
						Signature:             make([]byte, fieldparams.BLSSignatureLength),
					},
				},
			},
			VoluntaryExits: []*types.SignedVoluntaryExit{
				{
					Message: &types.VoluntaryExit{
						Epoch:          "0",
						ValidatorIndex: "0",
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			SyncAggregate: &types.SyncAggregate{
				SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
				SyncCommitteeBits:      SyncComitteeBits(),
			},
		},
	}
}

func BeaconBlockBody() *types.BeaconBlockBody {
	return &types.BeaconBlockBody{
		RandaoReveal: make([]byte, 32),
		Eth1Data: &types.Eth1Data{
			DepositRoot:  make([]byte, fieldparams.RootLength),
			DepositCount: "0",
			BlockHash:    make([]byte, 32),
		},
		Graffiti: make([]byte, 32),
		ProposerSlashings: []*types.ProposerSlashing{
			{
				Signedheader1: &types.SignedBeaconBlockHeader{
					Message: &types.BeaconBlockHeader{
						Slot:          "0",
						ProposerIndex: "0",
						ParentRoot:    make([]byte, fieldparams.RootLength),
						StateRoot:     make([]byte, fieldparams.RootLength),
						BodyRoot:      make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
				Signedheader2: &types.SignedBeaconBlockHeader{
					Message: &types.BeaconBlockHeader{
						Slot:          "0",
						ProposerIndex: "0",
						ParentRoot:    make([]byte, fieldparams.RootLength),
						StateRoot:     make([]byte, fieldparams.RootLength),
						BodyRoot:      make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
			},
		},
		AttesterSlashings: []*types.AttesterSlashing{
			{
				Attestation1: IndexedAttestation(),
				Attestation2: IndexedAttestation(),
			},
		},
		Attestations: []*types.Attestation{
			Attestation(),
		},
		Deposits: []*types.Deposit{
			{
				Proof: []string{"0x41"},
				Data: &types.DepositData{
					PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: make([]byte, 32),
					Amount:                "0",
					Signature:             make([]byte, fieldparams.BLSSignatureLength),
				},
			},
		},
		VoluntaryExits: []*types.SignedVoluntaryExit{
			{
				Message: &types.VoluntaryExit{
					Epoch:          "0",
					ValidatorIndex: "0",
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		},
	}
}

func ContributionAndProof() *types.ContributionAndProof {
	return &types.ContributionAndProof{
		AggregatorIndex: "0",
		Contribution: &types.SyncCommitteeContribution{
			Slot:              "0",
			BeaconBlockRoot:   make([]byte, fieldparams.RootLength),
			SubcommitteeIndex: "0",
			AggregationBits:   AggregationBits(),
			Signature:         make([]byte, fieldparams.BLSSignatureLength),
		},
		SelectionProof: make([]byte, fieldparams.BLSSignatureLength),
	}
}
