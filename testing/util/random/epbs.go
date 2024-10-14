package random

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// SignedBeaconBlock creates a random SignedBeaconBlockEPBS for testing purposes.
func SignedBeaconBlock(t *testing.T) *ethpb.SignedBeaconBlockEpbs {
	return &ethpb.SignedBeaconBlockEpbs{
		Block:     BeaconBlock(t),
		Signature: randomBytes(96, t),
	}
}

// BeaconBlock creates a random BeaconBlockEPBS for testing purposes.
func BeaconBlock(t *testing.T) *ethpb.BeaconBlockEpbs {
	return &ethpb.BeaconBlockEpbs{
		Slot:          primitives.Slot(randomUint64(t)),
		ProposerIndex: primitives.ValidatorIndex(randomUint64(t)),
		ParentRoot:    randomBytes(32, t),
		StateRoot:     randomBytes(32, t),
		Body:          BeaconBlockBody(t),
	}
}

// BeaconBlockBody creates a random BeaconBlockBodyEPBS for testing purposes.
func BeaconBlockBody(t *testing.T) *ethpb.BeaconBlockBodyEpbs {
	return &ethpb.BeaconBlockBodyEpbs{
		RandaoReveal: randomBytes(96, t),
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot:  randomBytes(32, t),
			DepositCount: randomUint64(t),
			BlockHash:    randomBytes(32, t),
		},
		Graffiti: randomBytes(32, t),
		ProposerSlashings: []*ethpb.ProposerSlashing{
			{Header_1: SignedBeaconBlockHeader(t),
				Header_2: SignedBeaconBlockHeader(t)},
		},
		AttesterSlashings: []*ethpb.AttesterSlashing{
			{
				Attestation_1: IndexedAttestation(t),
				Attestation_2: IndexedAttestation(t),
			},
		},
		Attestations:   []*ethpb.Attestation{Attestation(t), Attestation(t), Attestation(t)},
		Deposits:       []*ethpb.Deposit{Deposit(t), Deposit(t), Deposit(t)},
		VoluntaryExits: []*ethpb.SignedVoluntaryExit{SignedVoluntaryExit(t), SignedVoluntaryExit(t)},
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      bitfield.NewBitvector512(),
			SyncCommitteeSignature: randomBytes(96, t),
		},
		BlsToExecutionChanges:        []*ethpb.SignedBLSToExecutionChange{SignedBLSToExecutionChange(t), SignedBLSToExecutionChange(t)},
		SignedExecutionPayloadHeader: SignedExecutionPayloadHeader(t),
		PayloadAttestations: []*ethpb.PayloadAttestation{
			PayloadAttestation(t), PayloadAttestation(t), PayloadAttestation(t), PayloadAttestation(t),
		},
	}
}

// SignedBeaconBlockHeader creates a random SignedBeaconBlockHeader for testing purposes.
func SignedBeaconBlockHeader(t *testing.T) *ethpb.SignedBeaconBlockHeader {
	return &ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:          primitives.Slot(randomUint64(t)),
			ProposerIndex: primitives.ValidatorIndex(randomUint64(t)),
			ParentRoot:    randomBytes(32, t),
			StateRoot:     randomBytes(32, t),
			BodyRoot:      randomBytes(32, t),
		},
		Signature: randomBytes(96, t),
	}
}

// IndexedAttestation creates a random IndexedAttestation for testing purposes.
func IndexedAttestation(t *testing.T) *ethpb.IndexedAttestation {
	return &ethpb.IndexedAttestation{
		AttestingIndices: []uint64{randomUint64(t), randomUint64(t), randomUint64(t)},
		Data:             AttestationData(t),
		Signature:        randomBytes(96, t),
	}
}

// Attestation creates a random Attestation for testing purposes.
func Attestation(t *testing.T) *ethpb.Attestation {
	return &ethpb.Attestation{
		AggregationBits: bitfield.NewBitlist(123),
		Data:            AttestationData(t),
		Signature:       randomBytes(96, t),
	}
}

// AttestationData creates a random AttestationData for testing purposes.
func AttestationData(t *testing.T) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            primitives.Slot(randomUint64(t)),
		CommitteeIndex:  primitives.CommitteeIndex(randomUint64(t)),
		BeaconBlockRoot: randomBytes(32, t),
		Source: &ethpb.Checkpoint{
			Epoch: primitives.Epoch(randomUint64(t)),
			Root:  randomBytes(32, t),
		},
		Target: &ethpb.Checkpoint{
			Epoch: primitives.Epoch(randomUint64(t)),
			Root:  randomBytes(32, t),
		},
	}
}

// Deposit creates a random Deposit for testing purposes.
func Deposit(t *testing.T) *ethpb.Deposit {
	proof := make([][]byte, 33)
	for i := 0; i < 33; i++ {
		proof[i] = randomBytes(32, t)
	}
	return &ethpb.Deposit{
		Proof: proof,
		Data:  DepositData(t),
	}
}

// DepositData creates a random DepositData for testing purposes.
func DepositData(t *testing.T) *ethpb.Deposit_Data {
	return &ethpb.Deposit_Data{
		PublicKey:             randomBytes(48, t),
		WithdrawalCredentials: randomBytes(32, t),
		Amount:                randomUint64(t),
		Signature:             randomBytes(96, t),
	}
}

// SignedBLSToExecutionChange creates a random SignedBLSToExecutionChange for testing purposes.
func SignedBLSToExecutionChange(t *testing.T) *ethpb.SignedBLSToExecutionChange {
	return &ethpb.SignedBLSToExecutionChange{
		Message:   BLSToExecutionChange(t),
		Signature: randomBytes(96, t),
	}
}

// BLSToExecutionChange creates a random BLSToExecutionChange for testing purposes.
func BLSToExecutionChange(t *testing.T) *ethpb.BLSToExecutionChange {
	return &ethpb.BLSToExecutionChange{
		ValidatorIndex:     primitives.ValidatorIndex(randomUint64(t)),
		FromBlsPubkey:      randomBytes(48, t),
		ToExecutionAddress: randomBytes(20, t),
	}
}

// SignedVoluntaryExit creates a random SignedVoluntaryExit for testing purposes.
func SignedVoluntaryExit(t *testing.T) *ethpb.SignedVoluntaryExit {
	return &ethpb.SignedVoluntaryExit{
		Exit:      VoluntaryExit(t),
		Signature: randomBytes(96, t),
	}
}

// VoluntaryExit creates a random VoluntaryExit for testing purposes.
func VoluntaryExit(t *testing.T) *ethpb.VoluntaryExit {
	return &ethpb.VoluntaryExit{
		Epoch:          primitives.Epoch(randomUint64(t)),
		ValidatorIndex: primitives.ValidatorIndex(randomUint64(t)),
	}
}

// BeaconState creates a random BeaconStateEPBS for testing purposes.
func BeaconState(t *testing.T) *ethpb.BeaconStateEPBS {
	slashing := make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)
	pubkeys := make([][]byte, 512)
	for i := range pubkeys {
		pubkeys[i] = randomBytes(48, t)
	}
	return &ethpb.BeaconStateEPBS{
		GenesisTime:           randomUint64(t),
		GenesisValidatorsRoot: randomBytes(32, t),
		Slot:                  primitives.Slot(randomUint64(t)),
		Fork: &ethpb.Fork{
			PreviousVersion: randomBytes(4, t),
			CurrentVersion:  randomBytes(4, t),
			Epoch:           primitives.Epoch(randomUint64(t)),
		},
		LatestBlockHeader: &ethpb.BeaconBlockHeader{
			Slot:          primitives.Slot(randomUint64(t)),
			ProposerIndex: primitives.ValidatorIndex(randomUint64(t)),
			ParentRoot:    randomBytes(32, t),
			StateRoot:     randomBytes(32, t),
			BodyRoot:      randomBytes(32, t),
		},
		BlockRoots:      [][]byte{randomBytes(32, t), randomBytes(32, t), randomBytes(32, t)},
		StateRoots:      [][]byte{randomBytes(32, t), randomBytes(32, t), randomBytes(32, t)},
		HistoricalRoots: [][]byte{randomBytes(32, t), randomBytes(32, t), randomBytes(32, t)},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot:  randomBytes(32, t),
			DepositCount: randomUint64(t),
			BlockHash:    randomBytes(32, t),
		},
		Eth1DataVotes:    []*ethpb.Eth1Data{{DepositRoot: randomBytes(32, t), DepositCount: randomUint64(t), BlockHash: randomBytes(32, t)}},
		Eth1DepositIndex: randomUint64(t),
		Validators: []*ethpb.Validator{
			{
				PublicKey:                  randomBytes(48, t),
				WithdrawalCredentials:      randomBytes(32, t),
				EffectiveBalance:           randomUint64(t),
				ActivationEligibilityEpoch: primitives.Epoch(randomUint64(t)),
				ActivationEpoch:            primitives.Epoch(randomUint64(t)),
				ExitEpoch:                  primitives.Epoch(randomUint64(t)),
				WithdrawableEpoch:          primitives.Epoch(randomUint64(t)),
			},
		},
		Balances:                    []uint64{randomUint64(t)},
		RandaoMixes:                 [][]byte{randomBytes(32, t), randomBytes(32, t), randomBytes(32, t)},
		Slashings:                   slashing,
		PreviousEpochParticipation:  randomBytes(32, t),
		CurrentEpochParticipation:   randomBytes(32, t),
		JustificationBits:           randomBytes(1, t),
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Epoch: primitives.Epoch(randomUint64(t)), Root: randomBytes(32, t)},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Epoch: primitives.Epoch(randomUint64(t)), Root: randomBytes(32, t)},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Epoch: primitives.Epoch(randomUint64(t)), Root: randomBytes(32, t)},
		InactivityScores:            []uint64{randomUint64(t)},
		CurrentSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: randomBytes(48, t),
		},
		NextSyncCommittee: &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: randomBytes(48, t),
		},
		NextWithdrawalIndex:          randomUint64(t),
		NextWithdrawalValidatorIndex: primitives.ValidatorIndex(randomUint64(t)),
		HistoricalSummaries: []*ethpb.HistoricalSummary{{
			BlockSummaryRoot: randomBytes(32, t),
			StateSummaryRoot: randomBytes(32, t),
		}},
		DepositRequestsStartIndex:     randomUint64(t),
		DepositBalanceToConsume:       primitives.Gwei(randomUint64(t)),
		ExitBalanceToConsume:          primitives.Gwei(randomUint64(t)),
		EarliestExitEpoch:             primitives.Epoch(randomUint64(t)),
		ConsolidationBalanceToConsume: primitives.Gwei(randomUint64(t)),
		EarliestConsolidationEpoch:    primitives.Epoch(randomUint64(t)),
		PendingBalanceDeposits: []*ethpb.PendingBalanceDeposit{
			{
				Index:  primitives.ValidatorIndex(randomUint64(t)),
				Amount: randomUint64(t),
			},
		},
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{
				Index:             primitives.ValidatorIndex(randomUint64(t)),
				Amount:            randomUint64(t),
				WithdrawableEpoch: primitives.Epoch(randomUint64(t)),
			},
		},
		PendingConsolidations: []*ethpb.PendingConsolidation{
			{
				SourceIndex: primitives.ValidatorIndex(randomUint64(t)),
				TargetIndex: primitives.ValidatorIndex(randomUint64(t)),
			},
		},
		LatestBlockHash: randomBytes(32, t),
		LatestFullSlot:  primitives.Slot(randomUint64(t)),
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        randomBytes(32, t),
			ParentBlockRoot:        randomBytes(32, t),
			BlockHash:              randomBytes(32, t),
			BuilderIndex:           primitives.ValidatorIndex(randomUint64(t)),
			Slot:                   primitives.Slot(randomUint64(t)),
			Value:                  randomUint64(t),
			BlobKzgCommitmentsRoot: randomBytes(32, t),
			GasLimit:               randomUint64(t),
		},
		LastWithdrawalsRoot: randomBytes(32, t),
	}
}

// SignedExecutionPayloadHeader creates a random SignedExecutionPayloadHeader for testing purposes.
func SignedExecutionPayloadHeader(t *testing.T) *enginev1.SignedExecutionPayloadHeader {
	return &enginev1.SignedExecutionPayloadHeader{
		Message:   ExecutionPayloadHeader(t),
		Signature: randomBytes(96, t),
	}
}

// ExecutionPayloadHeader creates a random ExecutionPayloadHeaderEPBS for testing.
func ExecutionPayloadHeader(t *testing.T) *enginev1.ExecutionPayloadHeaderEPBS {
	return &enginev1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        randomBytes(32, t),
		ParentBlockRoot:        randomBytes(32, t),
		BlockHash:              randomBytes(32, t),
		BuilderIndex:           primitives.ValidatorIndex(randomUint64(t)),
		Slot:                   primitives.Slot(randomUint64(t)),
		Value:                  randomUint64(t),
		BlobKzgCommitmentsRoot: randomBytes(32, t),
		GasLimit:               randomUint64(t),
	}
}

// PayloadAttestation creates a random PayloadAttestation for testing purposes.
func PayloadAttestation(t *testing.T) *ethpb.PayloadAttestation {
	bv := bitfield.NewBitvector512()
	b := randomBytes(64, t)
	copy(bv[:], b)
	return &ethpb.PayloadAttestation{
		AggregationBits: bv,
		Data:            PayloadAttestationData(t),
		Signature:       randomBytes(96, t),
	}
}

// PayloadAttestationData generates a random PayloadAttestationData for testing purposes.
func PayloadAttestationData(t *testing.T) *ethpb.PayloadAttestationData {
	// Generate a random BeaconBlockRoot
	randomBytes := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		t.Fatalf("Failed to generate random BeaconBlockRoot: %v", err)
	}

	// Generate a random Slot value
	randomSlot, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		t.Fatalf("Failed to generate random Slot: %v", err)
	}

	payloadStatuses := []primitives.PTCStatus{
		primitives.PAYLOAD_ABSENT,
		primitives.PAYLOAD_PRESENT,
		primitives.PAYLOAD_WITHHELD,
	}
	// Select a random PayloadStatus
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(payloadStatuses))))
	if err != nil {
		t.Fatalf("Failed to select random PayloadStatus: %v", err)
	}
	randomPayloadStatus := payloadStatuses[index.Int64()]

	return &ethpb.PayloadAttestationData{
		BeaconBlockRoot: randomBytes,
		Slot:            primitives.Slot(randomSlot.Uint64()),
		PayloadStatus:   randomPayloadStatus,
	}
}

// SignedExecutionPayloadEnvelope creates a random SignedExecutionPayloadEnvelope for testing purposes.
func SignedExecutionPayloadEnvelope(t *testing.T) *enginev1.SignedExecutionPayloadEnvelope {
	return &enginev1.SignedExecutionPayloadEnvelope{
		Message:   ExecutionPayloadEnvelope(t),
		Signature: randomBytes(96, t),
	}
}

// ExecutionPayloadEnvelope creates a random ExecutionPayloadEnvelope for testing purposes.
func ExecutionPayloadEnvelope(t *testing.T) *enginev1.ExecutionPayloadEnvelope {
	withheld := randomUint64(t)%2 == 0
	return &enginev1.ExecutionPayloadEnvelope{
		Payload:            ExecutionPayload(t),
		BuilderIndex:       primitives.ValidatorIndex(randomUint64(t)),
		BeaconBlockRoot:    randomBytes(32, t),
		BlobKzgCommitments: [][]byte{randomBytes(48, t), randomBytes(48, t), randomBytes(48, t)},
		PayloadWithheld:    withheld,
		StateRoot:          randomBytes(32, t),
	}
}

// ExecutionPayload creates a random ExecutionPayloadEPBS for testing purposes.
func ExecutionPayload(t *testing.T) *enginev1.ExecutionPayloadElectra {
	return &enginev1.ExecutionPayloadElectra{
		ParentHash:    randomBytes(32, t),
		FeeRecipient:  randomBytes(20, t),
		StateRoot:     randomBytes(32, t),
		ReceiptsRoot:  randomBytes(32, t),
		LogsBloom:     randomBytes(256, t),
		PrevRandao:    randomBytes(32, t),
		BlockNumber:   randomUint64(t),
		GasLimit:      randomUint64(t),
		GasUsed:       randomUint64(t),
		Timestamp:     randomUint64(t),
		ExtraData:     randomBytes(32, t),
		BaseFeePerGas: randomBytes(32, t),
		BlockHash:     randomBytes(32, t),
		Transactions:  [][]byte{randomBytes(32, t), randomBytes(32, t), randomBytes(32, t)},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:          randomUint64(t),
				ValidatorIndex: primitives.ValidatorIndex(randomUint64(t)),
				Address:        randomBytes(20, t),
				Amount:         randomUint64(t),
			},
		},
		BlobGasUsed:           randomUint64(t),
		ExcessBlobGas:         randomUint64(t),
		DepositRequests:       []*enginev1.DepositRequest{DepositRequest(t), DepositRequest(t), DepositRequest(t), DepositRequest(t)},
		WithdrawalRequests:    WithdrawalRequests(t),
		ConsolidationRequests: []*enginev1.ConsolidationRequest{ConsolidationRequest(t)},
	}
}

func DepositRequest(t *testing.T) *enginev1.DepositRequest {
	return &enginev1.DepositRequest{
		Pubkey:                randomBytes(48, t),
		WithdrawalCredentials: randomBytes(32, t),
		Amount:                randomUint64(t),
		Signature:             randomBytes(96, t),
		Index:                 randomUint64(t),
	}
}

func WithdrawalRequests(t *testing.T) []*enginev1.WithdrawalRequest {
	requests := make([]*enginev1.WithdrawalRequest, fieldparams.MaxWithdrawalRequestsPerPayload)
	for i := range requests {
		requests[i] = WithdrawalRequest(t)
	}
	return requests
}

func WithdrawalRequest(t *testing.T) *enginev1.WithdrawalRequest {
	return &enginev1.WithdrawalRequest{
		SourceAddress:   randomBytes(20, t),
		ValidatorPubkey: randomBytes(48, t),
		Amount:          randomUint64(t),
	}
}

func ConsolidationRequest(t *testing.T) *enginev1.ConsolidationRequest {
	return &enginev1.ConsolidationRequest{
		SourceAddress: randomBytes(20, t),
		SourcePubkey:  randomBytes(20, t),
		TargetPubkey:  randomBytes(48, t),
	}
}

// SignedBlindPayloadEnvelope creates a random SignedBlindPayloadEnvelope for testing purposes.
func SignedBlindPayloadEnvelope(t *testing.T) *ethpb.SignedBlindPayloadEnvelope {
	return &ethpb.SignedBlindPayloadEnvelope{
		Message:   BlindPayloadEnvelope(t),
		Signature: randomBytes(96, t),
	}
}

// BlindPayloadEnvelope creates a random BlindPayloadEnvelope for testing purposes.
func BlindPayloadEnvelope(t *testing.T) *ethpb.BlindPayloadEnvelope {
	withheld := randomUint64(t)%2 == 0
	return &ethpb.BlindPayloadEnvelope{
		PayloadRoot:        randomBytes(32, t),
		BuilderIndex:       primitives.ValidatorIndex(randomUint64(t)),
		BeaconBlockRoot:    randomBytes(32, t),
		BlobKzgCommitments: [][]byte{randomBytes(48, t), randomBytes(48, t), randomBytes(48, t)},
		PayloadWithheld:    withheld,
		StateRoot:          randomBytes(32, t),
	}
}

func randomBytes(n int, t *testing.T) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}
	return b
}

func randomUint64(t *testing.T) uint64 {
	var num uint64
	b := randomBytes(8, t)
	num = binary.BigEndian.Uint64(b)
	return num
}
