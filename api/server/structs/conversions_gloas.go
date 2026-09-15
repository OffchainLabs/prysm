package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func ROExecutionPayloadBidFromConsensus(b interfaces.ROExecutionPayloadBid) *ExecutionPayloadBid {
	if b == nil {
		return nil
	}

	pbh := b.ParentBlockHash()
	pbr := b.ParentBlockRoot()
	bh := b.BlockHash()
	pr := b.PrevRandao()
	fr := b.FeeRecipient()
	commitments := b.BlobKzgCommitments()
	blobKzgCommitments := make([]string, 0, len(commitments))
	for _, commitment := range commitments {
		blobKzgCommitments = append(blobKzgCommitments, hexutil.Encode(commitment))
	}
	erRoot := b.ExecutionRequestsRoot()
	return &ExecutionPayloadBid{
		ParentBlockHash:       hexutil.Encode(pbh[:]),
		ParentBlockRoot:       hexutil.Encode(pbr[:]),
		BlockHash:             hexutil.Encode(bh[:]),
		PrevRandao:            hexutil.Encode(pr[:]),
		FeeRecipient:          hexutil.Encode(fr[:]),
		GasLimit:              fmt.Sprintf("%d", b.GasLimit()),
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex()),
		Slot:                  fmt.Sprintf("%d", b.Slot()),
		Value:                 fmt.Sprintf("%d", b.Value()),
		ExecutionPayment:      fmt.Sprintf("%d", b.ExecutionPayment()),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: hexutil.Encode(erRoot[:]),
	}
}

func BuildersFromConsensus(builders []*ethpb.Builder) []*Builder {
	newBuilders := make([]*Builder, len(builders))
	for i, b := range builders {
		newBuilders[i] = BuilderFromConsensus(b)
	}
	return newBuilders
}

func BuilderFromConsensus(b *ethpb.Builder) *Builder {
	return &Builder{
		Pubkey:            hexutil.Encode(b.Pubkey),
		Version:           hexutil.Encode(b.Version),
		ExecutionAddress:  hexutil.Encode(b.ExecutionAddress),
		Balance:           fmt.Sprintf("%d", b.Balance),
		DepositEpoch:      fmt.Sprintf("%d", b.DepositEpoch),
		WithdrawableEpoch: fmt.Sprintf("%d", b.WithdrawableEpoch),
	}
}

func BuilderPendingPaymentsFromConsensus(payments []*ethpb.BuilderPendingPayment) []*BuilderPendingPayment {
	newPayments := make([]*BuilderPendingPayment, len(payments))
	for i, p := range payments {
		newPayments[i] = BuilderPendingPaymentFromConsensus(p)
	}
	return newPayments
}

func BuilderPendingPaymentFromConsensus(p *ethpb.BuilderPendingPayment) *BuilderPendingPayment {
	return &BuilderPendingPayment{
		Weight:        fmt.Sprintf("%d", p.Weight),
		Withdrawal:    BuilderPendingWithdrawalFromConsensus(p.Withdrawal),
		ProposerIndex: fmt.Sprintf("%d", p.ProposerIndex),
	}
}

func BuilderPendingWithdrawalsFromConsensus(withdrawals []*ethpb.BuilderPendingWithdrawal) []*BuilderPendingWithdrawal {
	newWithdrawals := make([]*BuilderPendingWithdrawal, len(withdrawals))
	for i, w := range withdrawals {
		newWithdrawals[i] = BuilderPendingWithdrawalFromConsensus(w)
	}
	return newWithdrawals
}

func BuilderPendingWithdrawalFromConsensus(w *ethpb.BuilderPendingWithdrawal) *BuilderPendingWithdrawal {
	return &BuilderPendingWithdrawal{
		FeeRecipient: hexutil.Encode(w.FeeRecipient),
		Amount:       fmt.Sprintf("%d", w.Amount),
		BuilderIndex: fmt.Sprintf("%d", w.BuilderIndex),
	}
}

func PTCWindowFromConsensus(window []*ethpb.PTCs) []*PTCs {
	out := make([]*PTCs, len(window))
	for i, slot := range window {
		out[i] = PTCsFromConsensus(slot)
	}
	return out
}

func PTCsFromConsensus(p *ethpb.PTCs) *PTCs {
	if p == nil {
		return &PTCs{}
	}
	indices := make([]string, len(p.ValidatorIndices))
	for i, idx := range p.ValidatorIndices {
		indices[i] = fmt.Sprintf("%d", idx)
	}
	return &PTCs{ValidatorIndices: indices}
}

func (s *SignedProposerPreferences) ToConsensus() (*ethpb.SignedProposerPreferences, error) {
	if s == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedProposerPreferences")
	}
	if s.Message == nil {
		return nil, server.NewDecodeError(errNilValue, "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(s.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &ethpb.SignedProposerPreferences{
		Message:   msg,
		Signature: sig,
	}, nil
}

func SignedProposerPreferencesFromConsensus(s *ethpb.SignedProposerPreferences) *SignedProposerPreferences {
	if s == nil {
		return nil
	}
	return &SignedProposerPreferences{
		Message:   ProposerPreferencesFromConsensus(s.Message),
		Signature: hexutil.Encode(s.Signature),
	}
}

func BuilderRequestAuthFromConsensus(m *ethpb.BuilderRequestAuth) *BuilderRequestAuth {
	if m == nil {
		return nil
	}
	return &BuilderRequestAuth{
		Data: hexutil.Encode(m.Data),
		Slot: fmt.Sprintf("%d", m.Slot),
	}
}

func SignedBuilderRequestAuthFromConsensus(s *ethpb.SignedBuilderRequestAuth) *SignedBuilderRequestAuth {
	if s == nil {
		return nil
	}
	return &SignedBuilderRequestAuth{
		Message:   BuilderRequestAuthFromConsensus(s.Message),
		Signature: hexutil.Encode(s.Signature),
	}
}

func BuilderPreferencesFromConsensus(p *ethpb.BuilderPreferences) *BuilderPreferences {
	if p == nil {
		return nil
	}
	return &BuilderPreferences{
		MaxExecutionPayment: fmt.Sprintf("%d", p.MaxExecutionPayment),
	}
}

func BuilderPreferencesRequestFromConsensus(r *ethpb.BuilderPreferencesRequest) *BuilderPreferencesRequest {
	if r == nil {
		return nil
	}
	return &BuilderPreferencesRequest{
		Preferences: BuilderPreferencesFromConsensus(r.Preferences),
		Auth:        SignedBuilderRequestAuthFromConsensus(r.Auth),
	}
}

// Bounds from the beacon-APIs Gloas builder schemas.
const (
	maxBuilderEntries               = 64
	maxBuilderUrlLength             = 2048
	maxBuilderPubkeys               = 64
	maxBuilderRequestAuthDataLength = 4096
	// MAX_BUILDER_ENTRIES * (MIN_SEED_LOOKAHEAD + 1) * SLOTS_PER_EPOCH = 64 * 2 * 32.
	MaxBuilderPreferencesList = 4096
)

func (a *BuilderRequestAuth) ToConsensus() (*ethpb.BuilderRequestAuth, error) {
	data, err := bytesutil.DecodeHexWithMaxLength(a.Data, maxBuilderRequestAuthDataLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}
	if len(data) == 0 {
		return nil, server.NewDecodeError(fmt.Errorf("must not be empty"), "Data")
	}
	slot, err := strconv.ParseUint(a.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	return &ethpb.BuilderRequestAuth{Data: data, Slot: primitives.Slot(slot)}, nil
}

func (s *SignedBuilderRequestAuth) ToConsensus() (*ethpb.SignedBuilderRequestAuth, error) {
	if s.Message == nil {
		return nil, server.NewDecodeError(fmt.Errorf("must not be nil"), "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(s.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &ethpb.SignedBuilderRequestAuth{Message: msg, Signature: sig}, nil
}

func (e *BuilderPreferencesEntry) ToConsensus() (*ethpb.BuilderPreferencesEntry, error) {
	pubkey, err := bytesutil.DecodeHexWithLength(e.ProposerPubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerPubkey")
	}
	url, err := decodeBuilderUrl(e.Url)
	if err != nil {
		return nil, err
	}
	if e.Auth == nil {
		return nil, server.NewDecodeError(fmt.Errorf("must not be nil"), "Auth")
	}
	auth, err := e.Auth.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Auth")
	}
	maxPayment, err := strconv.ParseUint(e.MaxExecutionPayment, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "MaxExecutionPayment")
	}
	return &ethpb.BuilderPreferencesEntry{
		ProposerPubkey:      pubkey,
		Url:                 url,
		Auth:                auth,
		MaxExecutionPayment: primitives.Gwei(maxPayment),
	}, nil
}

func BuilderPreferencesEntryFromConsensus(e *ethpb.BuilderPreferencesEntry) *BuilderPreferencesEntry {
	if e == nil {
		return nil
	}
	return &BuilderPreferencesEntry{
		ProposerPubkey:      hexutil.Encode(e.ProposerPubkey),
		Url:                 string(e.Url),
		Auth:                SignedBuilderRequestAuthFromConsensus(e.Auth),
		MaxExecutionPayment: fmt.Sprintf("%d", e.MaxExecutionPayment),
	}
}

func (e *BuilderEntry) ToConsensus() (*ethpb.BuilderEntry, error) {
	url, err := decodeBuilderUrl(e.Url)
	if err != nil {
		return nil, err
	}
	if e.Auth == nil {
		return nil, server.NewDecodeError(fmt.Errorf("must not be nil"), "Auth")
	}
	auth, err := e.Auth.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Auth")
	}
	if len(e.BuilderPubkeys) > maxBuilderPubkeys {
		return nil, server.NewDecodeError(fmt.Errorf("more than %d items", maxBuilderPubkeys), "BuilderPubkeys")
	}
	pubkeys := make([][]byte, len(e.BuilderPubkeys))
	for i, pk := range e.BuilderPubkeys {
		pubkeys[i], err = bytesutil.DecodeHexWithLength(pk, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("BuilderPubkeys[%d]", i))
		}
	}
	maxPayment, err := strconv.ParseUint(e.MaxExecutionPayment, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "MaxExecutionPayment")
	}
	minBid, err := strconv.ParseUint(e.MinBid, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "MinBid")
	}
	boostFactor, err := strconv.ParseUint(e.BuilderBoostFactor, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderBoostFactor")
	}
	return &ethpb.BuilderEntry{
		Url:                 url,
		Auth:                auth,
		BuilderPubkeys:      pubkeys,
		MaxExecutionPayment: primitives.Gwei(maxPayment),
		MinBid:              primitives.Gwei(minBid),
		BuilderBoostFactor:  boostFactor,
	}, nil
}

func BuilderEntryFromConsensus(e *ethpb.BuilderEntry) *BuilderEntry {
	if e == nil {
		return nil
	}
	pubkeys := make([]string, len(e.BuilderPubkeys))
	for i, pk := range e.BuilderPubkeys {
		pubkeys[i] = hexutil.Encode(pk)
	}
	return &BuilderEntry{
		Url:                 string(e.Url),
		Auth:                SignedBuilderRequestAuthFromConsensus(e.Auth),
		BuilderPubkeys:      pubkeys,
		MaxExecutionPayment: fmt.Sprintf("%d", e.MaxExecutionPayment),
		MinBid:              fmt.Sprintf("%d", e.MinBid),
		BuilderBoostFactor:  fmt.Sprintf("%d", e.BuilderBoostFactor),
	}
}

func (c *BuilderConfig) ToConsensus() (*ethpb.BuilderConfig, error) {
	minBid, err := strconv.ParseUint(c.MinBid, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "MinBid")
	}
	boostFactor, err := strconv.ParseUint(c.BuilderBoostFactor, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderBoostFactor")
	}
	if len(c.Builders) > maxBuilderEntries {
		return nil, server.NewDecodeError(fmt.Errorf("more than %d items", maxBuilderEntries), "Builders")
	}
	builders := make([]*ethpb.BuilderEntry, len(c.Builders))
	for i, b := range c.Builders {
		if b == nil {
			return nil, server.NewDecodeError(fmt.Errorf("must not be nil"), fmt.Sprintf("Builders[%d]", i))
		}
		builders[i], err = b.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Builders[%d]", i))
		}
	}
	return &ethpb.BuilderConfig{
		MinBid:             primitives.Gwei(minBid),
		BuilderBoostFactor: boostFactor,
		Builders:           builders,
	}, nil
}

func BuilderConfigFromConsensus(c *ethpb.BuilderConfig) *BuilderConfig {
	if c == nil {
		return nil
	}
	builders := make([]*BuilderEntry, len(c.Builders))
	for i, b := range c.Builders {
		builders[i] = BuilderEntryFromConsensus(b)
	}
	return &BuilderConfig{
		MinBid:             fmt.Sprintf("%d", c.MinBid),
		BuilderBoostFactor: fmt.Sprintf("%d", c.BuilderBoostFactor),
		Builders:           builders,
	}
}

func decodeBuilderUrl(u string) ([]byte, error) {
	if u == "" {
		return nil, server.NewDecodeError(fmt.Errorf("must not be empty"), "Url")
	}
	if len(u) > maxBuilderUrlLength {
		return nil, server.NewDecodeError(fmt.Errorf("longer than %d bytes", maxBuilderUrlLength), "Url")
	}
	return []byte(u), nil
}

func ProposerPreferencesFromConsensus(p *ethpb.ProposerPreferences) *ProposerPreferences {
	if p == nil {
		return nil
	}
	return &ProposerPreferences{
		DependentRoot:  hexutil.Encode(p.DependentRoot),
		ProposalSlot:   fmt.Sprintf("%d", p.ProposalSlot),
		ValidatorIndex: fmt.Sprintf("%d", p.ValidatorIndex),
		FeeRecipient:   hexutil.Encode(p.FeeRecipient),
		TargetGasLimit: fmt.Sprintf("%d", p.TargetGasLimit),
	}
}

func (p *ProposerPreferences) ToConsensus() (*ethpb.ProposerPreferences, error) {
	dependentRoot, err := bytesutil.DecodeHexWithLength(p.DependentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "DependentRoot")
	}
	slot, err := strconv.ParseUint(p.ProposalSlot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposalSlot")
	}
	valIdx, err := strconv.ParseUint(p.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}
	feeRecipient, err := bytesutil.DecodeHexWithLength(p.FeeRecipient, common.AddressLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "FeeRecipient")
	}
	gasLimit, err := strconv.ParseUint(p.TargetGasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "TargetGasLimit")
	}
	return &ethpb.ProposerPreferences{
		DependentRoot:  dependentRoot,
		ProposalSlot:   primitives.Slot(slot),
		ValidatorIndex: primitives.ValidatorIndex(valIdx),
		FeeRecipient:   feeRecipient,
		TargetGasLimit: gasLimit,
	}, nil
}
