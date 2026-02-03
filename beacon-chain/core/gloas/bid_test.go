package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	fastssz "github.com/prysmaticlabs/fastssz"
	"google.golang.org/protobuf/proto"
)

type stubBlockBody struct {
	signedBid *ethpb.SignedExecutionPayloadBid
}

func (s stubBlockBody) Version() int                                 { return version.Gloas }
func (s stubBlockBody) RandaoReveal() [96]byte                       { return [96]byte{} }
func (s stubBlockBody) Eth1Data() *ethpb.Eth1Data                    { return nil }
func (s stubBlockBody) Graffiti() [32]byte                           { return [32]byte{} }
func (s stubBlockBody) ProposerSlashings() []*ethpb.ProposerSlashing { return nil }
func (s stubBlockBody) AttesterSlashings() []ethpb.AttSlashing       { return nil }
func (s stubBlockBody) Attestations() []ethpb.Att                    { return nil }
func (s stubBlockBody) Deposits() []*ethpb.Deposit                   { return nil }
func (s stubBlockBody) VoluntaryExits() []*ethpb.SignedVoluntaryExit { return nil }
func (s stubBlockBody) SyncAggregate() (*ethpb.SyncAggregate, error) { return nil, nil }
func (s stubBlockBody) IsNil() bool                                  { return s.signedBid == nil }
func (s stubBlockBody) HashTreeRoot() ([32]byte, error)              { return [32]byte{}, nil }
func (s stubBlockBody) Proto() (proto.Message, error)                { return nil, nil }
func (s stubBlockBody) Execution() (interfaces.ExecutionData, error) { return nil, nil }
func (s stubBlockBody) BLSToExecutionChanges() ([]*ethpb.SignedBLSToExecutionChange, error) {
	return nil, nil
}
func (s stubBlockBody) BlobKzgCommitments() ([][]byte, error) { return nil, nil }
func (s stubBlockBody) ExecutionRequests() (*enginev1.ExecutionRequests, error) {
	return nil, nil
}
func (s stubBlockBody) PayloadAttestations() ([]*ethpb.PayloadAttestation, error) {
	return nil, nil
}
func (s stubBlockBody) SignedExecutionPayloadBid() (*ethpb.SignedExecutionPayloadBid, error) {
	return s.signedBid, nil
}
func (s stubBlockBody) MarshalSSZ() ([]byte, error)         { return nil, nil }
func (s stubBlockBody) MarshalSSZTo([]byte) ([]byte, error) { return nil, nil }
func (s stubBlockBody) UnmarshalSSZ([]byte) error           { return nil }
func (s stubBlockBody) SizeSSZ() int                        { return 0 }

type stubBlock struct {
	slot       primitives.Slot
	proposer   primitives.ValidatorIndex
	parentRoot [32]byte
	body       stubBlockBody
	v          int
}

var (
	_ interfaces.ReadOnlyBeaconBlockBody = (*stubBlockBody)(nil)
	_ interfaces.ReadOnlyBeaconBlock     = (*stubBlock)(nil)
)

func (s stubBlock) Slot() primitives.Slot                    { return s.slot }
func (s stubBlock) ProposerIndex() primitives.ValidatorIndex { return s.proposer }
func (s stubBlock) ParentRoot() [32]byte                     { return s.parentRoot }
func (s stubBlock) StateRoot() [32]byte                      { return [32]byte{} }
func (s stubBlock) Body() interfaces.ReadOnlyBeaconBlockBody { return s.body }
func (s stubBlock) IsNil() bool                              { return false }
func (s stubBlock) IsBlinded() bool                          { return false }
func (s stubBlock) HashTreeRoot() ([32]byte, error)          { return [32]byte{}, nil }
func (s stubBlock) Proto() (proto.Message, error)            { return nil, nil }
func (s stubBlock) MarshalSSZ() ([]byte, error)              { return nil, nil }
func (s stubBlock) MarshalSSZTo([]byte) ([]byte, error)      { return nil, nil }
func (s stubBlock) UnmarshalSSZ([]byte) error                { return nil }
func (s stubBlock) SizeSSZ() int                             { return 0 }
func (s stubBlock) Version() int                             { return s.v }
func (s stubBlock) AsSignRequestObject() (validatorpb.SignRequestObject, error) {
	return nil, nil
}
func (s stubBlock) HashTreeRootWith(*fastssz.Hasher) error { return nil }

func buildGloasState(t *testing.T, slot primitives.Slot, proposerIdx primitives.ValidatorIndex, builderIdx primitives.BuilderIndex, balance uint64, randao [32]byte, latestHash [32]byte, builderPubkey [48]byte) *state_native.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	stateRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = bytes.Repeat([]byte{0xAA}, 32)
		stateRoots[i] = bytes.Repeat([]byte{0xBB}, 32)
	}
	randaoMixes := make([][]byte, cfg.EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = randao[:]
	}

	withdrawalCreds := make([]byte, 32)
	withdrawalCreds[0] = cfg.BuilderWithdrawalPrefixByte

	validatorCount := int(proposerIdx) + 1
	validators := make([]*ethpb.Validator, validatorCount)
	balances := make([]uint64, validatorCount)
	for i := range validatorCount {
		validators[i] = &ethpb.Validator{
			PublicKey:                  builderPubkey[:],
			WithdrawalCredentials:      withdrawalCreds,
			EffectiveBalance:           balance,
			Slashed:                    false,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  cfg.FarFutureEpoch,
			WithdrawableEpoch:          cfg.FarFutureEpoch,
		}
		balances[i] = balance
	}

	payments := make([]*ethpb.BuilderPendingPayment, cfg.SlotsPerEpoch*2)
	for i := range payments {
		payments[i] = &ethpb.BuilderPendingPayment{Withdrawal: &ethpb.BuilderPendingWithdrawal{}}
	}

	var builders []*ethpb.Builder
	if builderIdx != params.BeaconConfig().BuilderIndexSelfBuild {
		builderCount := int(builderIdx) + 1
		builders = make([]*ethpb.Builder, builderCount)
		builders[builderCount-1] = &ethpb.Builder{
			Pubkey:            builderPubkey[:],
			Version:           []byte{0},
			ExecutionAddress:  bytes.Repeat([]byte{0x01}, 20),
			Balance:           primitives.Gwei(balance),
			DepositEpoch:      0,
			WithdrawableEpoch: cfg.FarFutureEpoch,
		}
	}

	stProto := &ethpb.BeaconStateGloas{
		Slot:                  slot,
		GenesisValidatorsRoot: bytes.Repeat([]byte{0x11}, 32),
		Fork: &ethpb.Fork{
			CurrentVersion:  bytes.Repeat([]byte{0x22}, 4),
			PreviousVersion: bytes.Repeat([]byte{0x22}, 4),
			Epoch:           0,
		},
		BlockRoots:                blockRoots,
		StateRoots:                stateRoots,
		RandaoMixes:               randaoMixes,
		Validators:                validators,
		Balances:                  balances,
		LatestBlockHash:           latestHash[:],
		BuilderPendingPayments:    payments,
		BuilderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{},
		Builders:                  builders,
		FinalizedCheckpoint: &ethpb.Checkpoint{
			Epoch: 1,
		},
	}

	st, err := state_native.InitializeFromProtoGloas(stProto)
	require.NoError(t, err)
	return st.(*state_native.BeaconState)
}

func signBid(t *testing.T, sk common.SecretKey, bid *ethpb.ExecutionPayloadBid, fork *ethpb.Fork, genesisRoot [32]byte) [96]byte {
	t.Helper()
	epoch := slots.ToEpoch(primitives.Slot(bid.Slot))
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainBeaconBuilder, genesisRoot[:])
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(bid, domain)
	require.NoError(t, err)
	sig := sk.Sign(root[:]).Marshal()
	var out [96]byte
	copy(out[:], sig)
	return out
}

func blobCommitmentsForSlot(slot primitives.Slot, count int) [][]byte {
	max := int(params.BeaconConfig().MaxBlobsPerBlockAtEpoch(slots.ToEpoch(slot)))
	if count > max {
		count = max
	}
	commitments := make([][]byte, count)
	for i := range commitments {
		commitments[i] = bytes.Repeat([]byte{0xEE}, 48)
	}
	return commitments
}

func tooManyBlobCommitmentsForSlot(slot primitives.Slot) [][]byte {
	max := int(params.BeaconConfig().MaxBlobsPerBlockAtEpoch(slots.ToEpoch(slot)))
	count := max + 1
	commitments := make([][]byte, count)
	for i := range commitments {
		commitments[i] = bytes.Repeat([]byte{0xEE}, 48)
	}
	return commitments
}

func TestProcessExecutionPayloadBid_SelfBuildSuccess(t *testing.T) {
	slot := primitives.Slot(12)
	proposerIdx := primitives.ValidatorIndex(0)
	builderIdx := params.BeaconConfig().BuilderIndexSelfBuild
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))
	pubKey := [48]byte{}
	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinActivationBalance+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xCC}, 32),
		BlockHash:          bytes.Repeat([]byte{0xDD}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              0,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xFF}, 20),
	}
	signed := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}

	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	require.NoError(t, ProcessExecutionPayloadBid(state, block))

	stateProto, ok := state.ToProto().(*ethpb.BeaconStateGloas)
	require.Equal(t, true, ok)
	slotIndex := params.BeaconConfig().SlotsPerEpoch + (slot % params.BeaconConfig().SlotsPerEpoch)
	require.Equal(t, primitives.Gwei(0), stateProto.BuilderPendingPayments[slotIndex].Withdrawal.Amount)
}

func TestProcessExecutionPayloadBid_SelfBuildNonZeroAmountFails(t *testing.T) {
	slot := primitives.Slot(2)
	proposerIdx := primitives.ValidatorIndex(0)
	builderIdx := params.BeaconConfig().BuilderIndexSelfBuild
	randao := [32]byte{}
	latestHash := [32]byte{1}
	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinActivationBalance+1000, randao, latestHash, [48]byte{})

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xAA}, 32),
		BlockHash:          bytes.Repeat([]byte{0xBB}, 32),
		PrevRandao:         randao[:],
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              10,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xDD}, 20),
	}
	signed := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err := ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "self-build amount must be zero", err)
}

func TestProcessExecutionPayloadBid_PendingPaymentAndCacheBid(t *testing.T) {
	slot := primitives.Slot(8)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	pub := sk.PublicKey().Marshal()
	var pubKey [48]byte
	copy(pubKey[:], pub)

	balance := params.BeaconConfig().MinActivationBalance + 1_000_000
	state := buildGloasState(t, slot, proposerIdx, builderIdx, balance, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xCC}, 32),
		BlockHash:          bytes.Repeat([]byte{0xDD}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              500_000,
		ExecutionPayment:   1,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xFF}, 20),
	}

	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: sig[:],
	}

	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx, // not self-build
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	require.NoError(t, ProcessExecutionPayloadBid(state, block))

	stateProto, ok := state.ToProto().(*ethpb.BeaconStateGloas)
	require.Equal(t, true, ok)
	slotIndex := params.BeaconConfig().SlotsPerEpoch + (slot % params.BeaconConfig().SlotsPerEpoch)
	require.Equal(t, primitives.Gwei(500_000), stateProto.BuilderPendingPayments[slotIndex].Withdrawal.Amount)

	require.NotNil(t, stateProto.LatestExecutionPayloadBid)
	require.Equal(t, primitives.BuilderIndex(1), stateProto.LatestExecutionPayloadBid.BuilderIndex)
	require.Equal(t, primitives.Gwei(500_000), stateProto.LatestExecutionPayloadBid.Value)
}

func TestProcessExecutionPayloadBid_BuilderNotActive(t *testing.T) {
	slot := primitives.Slot(4)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0x01}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0x02}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)
	// Make builder inactive by setting withdrawable_epoch.
	stateProto := state.ToProto().(*ethpb.BeaconStateGloas)
	stateProto.Builders[int(builderIdx)].WithdrawableEpoch = 0
	stateIface, err := state_native.InitializeFromProtoGloas(stateProto)
	require.NoError(t, err)
	state = stateIface.(*state_native.BeaconState)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0x03}, 32),
		BlockHash:          bytes.Repeat([]byte{0x04}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              10,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0x06}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "is not active", err)
}

func TestProcessExecutionPayloadBid_CannotCoverBid(t *testing.T) {
	slot := primitives.Slot(5)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0x0A}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0x0B}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+10, randao, latestHash, pubKey)
	stateProto := state.ToProto().(*ethpb.BeaconStateGloas)
	// Add pending balances to push below required balance.
	stateProto.BuilderPendingWithdrawals = []*ethpb.BuilderPendingWithdrawal{
		{Amount: 15, BuilderIndex: builderIdx},
	}
	stateProto.BuilderPendingPayments = []*ethpb.BuilderPendingPayment{
		{Withdrawal: &ethpb.BuilderPendingWithdrawal{Amount: 20, BuilderIndex: builderIdx}},
	}
	stateIface, err := state_native.InitializeFromProtoGloas(stateProto)
	require.NoError(t, err)
	state = stateIface.(*state_native.BeaconState)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xCC}, 32),
		BlockHash:          bytes.Repeat([]byte{0xDD}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              25,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xFF}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "cannot cover bid amount", err)
}

func TestProcessExecutionPayloadBid_InvalidSignature(t *testing.T) {
	slot := primitives.Slot(6)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xCC}, 32),
		BlockHash:          bytes.Repeat([]byte{0xDD}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              10,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xFF}, 20),
	}
	// Use an invalid signature.
	invalidSig := [96]byte{1}
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: invalidSig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "bid signature validation failed", err)
}

func TestProcessExecutionPayloadBid_TooManyBlobCommitments(t *testing.T) {
	slot := primitives.Slot(9)
	proposerIdx := primitives.ValidatorIndex(0)
	builderIdx := params.BeaconConfig().BuilderIndexSelfBuild
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))
	pubKey := [48]byte{}
	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinActivationBalance+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xCC}, 32),
		BlockHash:          bytes.Repeat([]byte{0xDD}, 32),
		PrevRandao:         randao[:],
		BuilderIndex:       builderIdx,
		Slot:               slot,
		BlobKzgCommitments: tooManyBlobCommitmentsForSlot(slot),
		FeeRecipient:       bytes.Repeat([]byte{0xFF}, 20),
	}
	signed := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}

	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err := ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "blob KZG commitments over max", err)
}

func TestProcessExecutionPayloadBid_SlotMismatch(t *testing.T) {
	slot := primitives.Slot(10)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0xAA}, 32),
		BlockHash:          bytes.Repeat([]byte{0xBB}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot + 1, // mismatch
		Value:              1,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0xDD}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "bid slot", err)
}

func TestProcessExecutionPayloadBid_ParentHashMismatch(t *testing.T) {
	slot := primitives.Slot(11)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    bytes.Repeat([]byte{0x11}, 32), // mismatch
		ParentBlockRoot:    bytes.Repeat([]byte{0x22}, 32),
		BlockHash:          bytes.Repeat([]byte{0x33}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              1,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0x55}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "parent block hash mismatch", err)
}

func TestProcessExecutionPayloadBid_ParentRootMismatch(t *testing.T) {
	slot := primitives.Slot(12)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)

	parentRoot := bytes.Repeat([]byte{0x22}, 32)
	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    parentRoot,
		BlockHash:          bytes.Repeat([]byte{0x33}, 32),
		PrevRandao:         randao[:],
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              1,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0x55}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bytes.Repeat([]byte{0x99}, 32)), // mismatch
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "parent block root mismatch", err)
}

func TestProcessExecutionPayloadBid_PrevRandaoMismatch(t *testing.T) {
	slot := primitives.Slot(13)
	builderIdx := primitives.BuilderIndex(1)
	proposerIdx := primitives.ValidatorIndex(2)
	randao := [32]byte(bytes.Repeat([]byte{0xAA}, 32))
	latestHash := [32]byte(bytes.Repeat([]byte{0xBB}, 32))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	var pubKey [48]byte
	copy(pubKey[:], sk.PublicKey().Marshal())

	state := buildGloasState(t, slot, proposerIdx, builderIdx, params.BeaconConfig().MinDepositAmount+1000, randao, latestHash, pubKey)

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    latestHash[:],
		ParentBlockRoot:    bytes.Repeat([]byte{0x22}, 32),
		BlockHash:          bytes.Repeat([]byte{0x33}, 32),
		PrevRandao:         bytes.Repeat([]byte{0x01}, 32), // mismatch
		GasLimit:           1,
		BuilderIndex:       builderIdx,
		Slot:               slot,
		Value:              1,
		ExecutionPayment:   0,
		BlobKzgCommitments: blobCommitmentsForSlot(slot, 1),
		FeeRecipient:       bytes.Repeat([]byte{0x55}, 20),
	}
	genesis := bytesutil.ToBytes32(state.GenesisValidatorsRoot())
	sig := signBid(t, sk, bid, state.Fork(), genesis)
	signed := &ethpb.SignedExecutionPayloadBid{Message: bid, Signature: sig[:]}
	block := stubBlock{
		slot:       slot,
		proposer:   proposerIdx,
		parentRoot: bytesutil.ToBytes32(bid.ParentBlockRoot),
		body:       stubBlockBody{signedBid: signed},
		v:          version.Gloas,
	}

	err = ProcessExecutionPayloadBid(state, block)
	require.ErrorContains(t, "prev randao mismatch", err)
}
