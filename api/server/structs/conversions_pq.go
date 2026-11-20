package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ----------------------------------------------------------------------------
// VRF proof
// ----------------------------------------------------------------------------

// VRFProofFromConsensus encodes a VRF proof, stored as the concatenation of its nodes, as the list of its hex encoded
// nodes.
func VRFProofFromConsensus(proof []byte) []string {
	nodes := make([]string, 0, len(proof)/fieldparams.RootLength)
	for start := 0; start+fieldparams.RootLength <= len(proof); start += fieldparams.RootLength {
		nodes = append(nodes, hexutil.Encode(proof[start:start+fieldparams.RootLength]))
	}

	return nodes
}

// VRFProofToConsensus decodes a VRF proof, encoded as the list of its hex encoded nodes, as the concatenation of its
// nodes.
func VRFProofToConsensus(nodes []string) ([]byte, error) {
	if len(nodes) != fieldparams.VRFProofLength {
		return nil, fmt.Errorf("VRF proof of %d nodes, expected %d nodes", len(nodes), fieldparams.VRFProofLength)
	}

	proof := make([]byte, 0, fieldparams.VRFProofLength*fieldparams.RootLength)
	for i, node := range nodes {
		decoded, err := bytesutil.DecodeHexWithLength(node, fieldparams.RootLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("[%d]", i))
		}

		proof = append(proof, decoded...)
	}

	return proof, nil
}

// ----------------------------------------------------------------------------
// Validator
// ----------------------------------------------------------------------------

func ValidatorPQFromConsensus(v *eth.ValidatorPQ) *ValidatorPQ {
	return &ValidatorPQ{
		Pubkey:                     hexutil.Encode(v.PublicKey),
		VrfRoot:                    hexutil.Encode(v.VrfRoot),
		WithdrawalCredentials:      hexutil.Encode(v.WithdrawalCredentials),
		EffectiveBalance:           fmt.Sprintf("%d", v.EffectiveBalance),
		Slashed:                    v.Slashed,
		ActivationEligibilityEpoch: fmt.Sprintf("%d", v.ActivationEligibilityEpoch),
		ActivationEpoch:            fmt.Sprintf("%d", v.ActivationEpoch),
		ExitEpoch:                  fmt.Sprintf("%d", v.ExitEpoch),
		WithdrawableEpoch:          fmt.Sprintf("%d", v.WithdrawableEpoch),
	}
}

func (v *ValidatorPQ) ToConsensus() (*eth.ValidatorPQ, error) {
	if v == nil {
		return nil, errNilValue
	}

	pubkey, err := bytesutil.DecodeHexWithLength(v.Pubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Pubkey")
	}

	vrfRoot, err := bytesutil.DecodeHexWithLength(v.VrfRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "VrfRoot")
	}

	withdrawalCredentials, err := bytesutil.DecodeHexWithLength(v.WithdrawalCredentials, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "WithdrawalCredentials")
	}

	effectiveBalance, err := strconv.ParseUint(v.EffectiveBalance, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "EffectiveBalance")
	}

	activationEligibilityEpoch, err := strconv.ParseUint(v.ActivationEligibilityEpoch, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ActivationEligibilityEpoch")
	}

	activationEpoch, err := strconv.ParseUint(v.ActivationEpoch, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ActivationEpoch")
	}

	exitEpoch, err := strconv.ParseUint(v.ExitEpoch, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExitEpoch")
	}

	withdrawableEpoch, err := strconv.ParseUint(v.WithdrawableEpoch, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "WithdrawableEpoch")
	}

	return &eth.ValidatorPQ{
		PublicKey:                  pubkey,
		VrfRoot:                    vrfRoot,
		WithdrawalCredentials:      withdrawalCredentials,
		EffectiveBalance:           effectiveBalance,
		Slashed:                    v.Slashed,
		ActivationEligibilityEpoch: primitives.Epoch(activationEligibilityEpoch),
		ActivationEpoch:            primitives.Epoch(activationEpoch),
		ExitEpoch:                  primitives.Epoch(exitEpoch),
		WithdrawableEpoch:          primitives.Epoch(withdrawableEpoch),
	}, nil
}

// ----------------------------------------------------------------------------
// Attestations
// ----------------------------------------------------------------------------

func SingleAttPQFromConsensus(a *eth.SingleAttestationPQ) *SingleAttestation {
	return &SingleAttestation{
		CommitteeIndex: fmt.Sprintf("%d", a.CommitteeId),
		AttesterIndex:  fmt.Sprintf("%d", a.AttesterIndex),
		Data:           AttDataFromConsensus(a.Data),
		Signature:      hexutil.Encode(a.Signature),
	}
}

// ToConsensusPQ converts a single attestation of the PQ fork, whose signature is a leanXMSS signature.
func (a *SingleAttestation) ToConsensusPQ() (*eth.SingleAttestationPQ, error) {
	if a == nil {
		return nil, errNilValue
	}

	committeeIndex, err := strconv.ParseUint(a.CommitteeIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "CommitteeIndex")
	}

	attesterIndex, err := strconv.ParseUint(a.AttesterIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "AttesterIndex")
	}

	if a.Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Data")
	}

	data, err := a.Data.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}

	signature, err := decodeXMSSSignature(a.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SingleAttestationPQ{
		CommitteeId:   primitives.CommitteeIndex(committeeIndex),
		AttesterIndex: primitives.ValidatorIndex(attesterIndex),
		Data:          data,
		Signature:     signature,
	}, nil
}

func AttPQFromConsensus(a *eth.AttestationPQ) *AttestationPQ {
	return &AttestationPQ{
		AggregationBits: hexutil.Encode(a.AggregationBits),
		Data:            AttDataFromConsensus(a.Data),
		CommitteeBits:   hexutil.Encode(a.CommitteeBits),
	}
}

func (a *AttestationPQ) ToConsensus() (*eth.AttestationPQ, error) {
	if a == nil {
		return nil, errNilValue
	}

	aggregationBits, err := hexutil.Decode(a.AggregationBits)
	if err != nil {
		return nil, server.NewDecodeError(err, "AggregationBits")
	}

	if a.Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Data")
	}

	data, err := a.Data.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}

	committeeBits, err := decodeCommitteeBitsPQ(a.CommitteeBits)
	if err != nil {
		return nil, server.NewDecodeError(err, "CommitteeBits")
	}

	return &eth.AttestationPQ{
		AggregationBits: aggregationBits,
		Data:            data,
		CommitteeBits:   committeeBits,
	}, nil
}

func AttsPQFromConsensus(src []*eth.AttestationPQ) []*AttestationPQ {
	attestations := make([]*AttestationPQ, len(src))
	for i, attestation := range src {
		attestations[i] = AttPQFromConsensus(attestation)
	}

	return attestations
}

func AttsPQToConsensus(src []*AttestationPQ) ([]*eth.AttestationPQ, error) {
	if err := slice.VerifyMaxLength(src, params.BeaconConfig().MaxAttestationsPQ); err != nil {
		return nil, err
	}

	attestations := make([]*eth.AttestationPQ, len(src))
	for i, attestation := range src {
		var err error
		if attestations[i], err = attestation.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("[%d]", i))
		}
	}

	return attestations, nil
}

func IndexedAttPQFromConsensus(a *eth.IndexedAttestationPQ) *IndexedAttestationPQ {
	indices := make([]string, len(a.AttestingIndices))
	for i, index := range a.AttestingIndices {
		indices[i] = fmt.Sprintf("%d", index)
	}

	return &IndexedAttestationPQ{
		AttestingIndices: indices,
		Data:             AttDataFromConsensus(a.Data),
		Proof:            hexutil.Encode(a.Proof),
	}
}

func (a *IndexedAttestationPQ) ToConsensus() (*eth.IndexedAttestationPQ, error) {
	if a == nil {
		return nil, errNilValue
	}

	indices := make([]uint64, len(a.AttestingIndices))
	for i, index := range a.AttestingIndices {
		var err error
		if indices[i], err = strconv.ParseUint(index, 10, 64); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("AttestingIndices[%d]", i))
		}
	}

	if a.Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Data")
	}

	data, err := a.Data.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}

	proof, err := decodeAttestationProof(a.Proof)
	if err != nil {
		return nil, server.NewDecodeError(err, "Proof")
	}

	return &eth.IndexedAttestationPQ{
		AttestingIndices: indices,
		Data:             data,
		Proof:            proof,
	}, nil
}

func AttesterSlashingPQFromConsensus(s *eth.AttesterSlashingPQ) *AttesterSlashingPQ {
	return &AttesterSlashingPQ{
		Attestation1: IndexedAttPQFromConsensus(s.Attestation_1),
		Attestation2: IndexedAttPQFromConsensus(s.Attestation_2),
	}
}

func (s *AttesterSlashingPQ) ToConsensus() (*eth.AttesterSlashingPQ, error) {
	if s == nil {
		return nil, errNilValue
	}

	attestation1, err := s.Attestation1.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Attestation1")
	}

	attestation2, err := s.Attestation2.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Attestation2")
	}

	return &eth.AttesterSlashingPQ{
		Attestation_1: attestation1,
		Attestation_2: attestation2,
	}, nil
}

func AggregatePQFromConsensus(aggregate *eth.AttestationPQ, proof []byte) *AggregatePQ {
	return &AggregatePQ{
		Aggregate: AttPQFromConsensus(aggregate),
		Proof:     hexutil.Encode(proof),
	}
}

// ToConsensus returns the aggregate and the proof of the signatures of its attesters.
func (a *AggregatePQ) ToConsensus() (*eth.AttestationPQ, []byte, error) {
	if a == nil {
		return nil, nil, errNilValue
	}

	aggregate, err := a.Aggregate.ToConsensus()
	if err != nil {
		return nil, nil, server.NewDecodeError(err, "Aggregate")
	}

	proof, err := decodeAttestationProof(a.Proof)
	if err != nil {
		return nil, nil, server.NewDecodeError(err, "Proof")
	}

	return aggregate, proof, nil
}

func SignedAggregateAttAndProofPQFromConsensus(s *eth.SignedAggregateAttestationAndProofPQ) *SignedAggregateAttestationAndProofPQ {
	return &SignedAggregateAttestationAndProofPQ{
		Message: &AggregateAttestationAndProofPQ{
			AggregatorIndex: fmt.Sprintf("%d", s.Message.AggregatorIndex),
			Aggregate:       AttPQFromConsensus(s.Message.Aggregate),
			Proof:           hexutil.Encode(s.Message.Proof),
		},
		Signature: hexutil.Encode(s.Signature),
	}
}

func (a *AggregateAttestationAndProofPQ) ToConsensus() (*eth.AggregateAttestationAndProofPQ, error) {
	if a == nil {
		return nil, errNilValue
	}

	aggregatorIndex, err := strconv.ParseUint(a.AggregatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "AggregatorIndex")
	}

	aggregate, err := a.Aggregate.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Aggregate")
	}

	proof, err := decodeAttestationProof(a.Proof)
	if err != nil {
		return nil, server.NewDecodeError(err, "Proof")
	}

	return &eth.AggregateAttestationAndProofPQ{
		AggregatorIndex: primitives.ValidatorIndex(aggregatorIndex),
		Aggregate:       aggregate,
		Proof:           proof,
	}, nil
}

func (s *SignedAggregateAttestationAndProofPQ) ToConsensus() (*eth.SignedAggregateAttestationAndProofPQ, error) {
	if s == nil {
		return nil, errNilValue
	}

	message, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(s.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedAggregateAttestationAndProofPQ{
		Message:   message,
		Signature: signature,
	}, nil
}

// ----------------------------------------------------------------------------
// Payload attestations, proposer preferences, slashings and bids
// ----------------------------------------------------------------------------

func PayloadAttestationMessagePQFromConsensus(m *eth.PayloadAttestationMessagePQ) *PayloadAttestationMessage {
	return &PayloadAttestationMessage{
		ValidatorIndex: fmt.Sprintf("%d", m.ValidatorIndex),
		Data:           PayloadAttestationDataFromConsensus(m.Data),
		Signature:      hexutil.Encode(m.Signature),
	}
}

// ToConsensusPQ converts a payload attestation message of the PQ fork, whose signature is a leanXMSS signature.
func (p *PayloadAttestationMessage) ToConsensusPQ() (*eth.PayloadAttestationMessagePQ, error) {
	if p == nil {
		return nil, errNilValue
	}

	validatorIndex, err := strconv.ParseUint(p.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}

	data, err := p.Data.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}

	signature, err := decodeXMSSSignature(p.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.PayloadAttestationMessagePQ{
		ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
		Data:           data,
		Signature:      signature,
	}, nil
}

func SignedProposerPreferencesPQFromConsensus(s *eth.SignedProposerPreferencesPQ) *SignedProposerPreferences {
	return &SignedProposerPreferences{
		Message:   ProposerPreferencesFromConsensus(s.Message),
		Signature: hexutil.Encode(s.Signature),
	}
}

// ToConsensusPQ converts signed proposer preferences of the PQ fork, whose signature is a leanXMSS signature.
func (s *SignedProposerPreferences) ToConsensusPQ() (*eth.SignedProposerPreferencesPQ, error) {
	if s == nil {
		return nil, errNilValue
	}

	if s.Message == nil {
		return nil, server.NewDecodeError(errNilValue, "Message")
	}

	message, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(s.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedProposerPreferencesPQ{
		Message:   message,
		Signature: signature,
	}, nil
}

func SignedBeaconBlockHeaderPQFromConsensus(h *eth.SignedBeaconBlockHeaderPQ) *SignedBeaconBlockHeader {
	return &SignedBeaconBlockHeader{
		Message:   BeaconBlockHeaderFromConsensus(h.Header),
		Signature: hexutil.Encode(h.Signature),
	}
}

// ToConsensusPQ converts a signed block header of the PQ fork, whose signature is a leanXMSS signature.
func (h *SignedBeaconBlockHeader) ToConsensusPQ() (*eth.SignedBeaconBlockHeaderPQ, error) {
	if h == nil {
		return nil, errNilValue
	}

	header, err := h.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(h.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedBeaconBlockHeaderPQ{
		Header:    header,
		Signature: signature,
	}, nil
}

func ProposerSlashingPQFromConsensus(s *eth.ProposerSlashingPQ) *ProposerSlashing {
	return &ProposerSlashing{
		SignedHeader1: SignedBeaconBlockHeaderPQFromConsensus(s.Header_1),
		SignedHeader2: SignedBeaconBlockHeaderPQFromConsensus(s.Header_2),
	}
}

// ToConsensusPQ converts a proposer slashing of the PQ fork, whose headers are signed with leanXMSS.
func (s *ProposerSlashing) ToConsensusPQ() (*eth.ProposerSlashingPQ, error) {
	if s == nil {
		return nil, errNilValue
	}

	header1, err := s.SignedHeader1.ToConsensusPQ()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedHeader1")
	}

	header2, err := s.SignedHeader2.ToConsensusPQ()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedHeader2")
	}

	return &eth.ProposerSlashingPQ{
		Header_1: header1,
		Header_2: header2,
	}, nil
}

func SignedExecutionPayloadBidPQFromConsensus(b *eth.SignedExecutionPayloadBidPQ) *SignedExecutionPayloadBid {
	return &SignedExecutionPayloadBid{
		Message:   ExecutionPayloadBidFromConsensus(b.Message),
		Signature: hexutil.Encode(b.Signature),
	}
}

// ToConsensusPQ converts a signed bid of the PQ fork, whose signature is a leanXMSS signature.
func (b *SignedExecutionPayloadBid) ToConsensusPQ() (*eth.SignedExecutionPayloadBidPQ, error) {
	if b == nil {
		return nil, errNilValue
	}

	message, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(b.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedExecutionPayloadBidPQ{
		Message:   message,
		Signature: signature,
	}, nil
}

// ----------------------------------------------------------------------------
// Execution requests and payload envelope
// ----------------------------------------------------------------------------

func ExecutionRequestsPQFromConsensus(r *enginev1.ExecutionRequestsPQ) *ExecutionRequestsPQ {
	validatorPreregistrations := make([]*ValidatorPreregistrationRequest, len(r.ValidatorPreregistrations))
	for i, request := range r.ValidatorPreregistrations {
		validatorPreregistrations[i] = &ValidatorPreregistrationRequest{
			Pubkey:                hexutil.Encode(request.Pubkey),
			WithdrawalCredentials: hexutil.Encode(request.WithdrawalCredentials),
			VrfRoot:               hexutil.Encode(request.VrfRoot),
		}
	}

	builderPreregistrations := make([]*BuilderPreregistrationRequest, len(r.BuilderPreregistrations))
	for i, request := range r.BuilderPreregistrations {
		builderPreregistrations[i] = &BuilderPreregistrationRequest{
			Pubkey:                hexutil.Encode(request.Pubkey),
			WithdrawalCredentials: hexutil.Encode(request.WithdrawalCredentials),
		}
	}

	voluntaryExits := make([]*VoluntaryExitRequest, len(r.VoluntaryExits))
	for i, request := range r.VoluntaryExits {
		voluntaryExits[i] = &VoluntaryExitRequest{Pubkey: hexutil.Encode(request.Pubkey)}
	}

	return &ExecutionRequestsPQ{
		Deposits:                  DepositRequestsFromConsensus(r.Deposits),
		Withdrawals:               WithdrawalRequestsFromConsensus(r.Withdrawals),
		Consolidations:            ConsolidationRequestsFromConsensus(r.Consolidations),
		BuilderDeposits:           BuilderDepositRequestsFromConsensus(r.BuilderDeposits),
		BuilderExits:              BuilderExitRequestsFromConsensus(r.BuilderExits),
		ValidatorPreregistrations: validatorPreregistrations,
		BuilderPreregistrations:   builderPreregistrations,
		VoluntaryExits:            voluntaryExits,
	}
}

func (r *ValidatorPreregistrationRequest) ToConsensus() (*enginev1.ValidatorPreregistrationRequest, error) {
	if r == nil {
		return nil, errNilValue
	}

	pubkey, err := bytesutil.DecodeHexWithLength(r.Pubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Pubkey")
	}

	withdrawalCredentials, err := bytesutil.DecodeHexWithLength(r.WithdrawalCredentials, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "WithdrawalCredentials")
	}

	vrfRoot, err := bytesutil.DecodeHexWithLength(r.VrfRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "VrfRoot")
	}

	return &enginev1.ValidatorPreregistrationRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: withdrawalCredentials,
		VrfRoot:               vrfRoot,
	}, nil
}

func (r *BuilderPreregistrationRequest) ToConsensus() (*enginev1.BuilderPreregistrationRequest, error) {
	if r == nil {
		return nil, errNilValue
	}

	pubkey, err := bytesutil.DecodeHexWithLength(r.Pubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Pubkey")
	}

	withdrawalCredentials, err := bytesutil.DecodeHexWithLength(r.WithdrawalCredentials, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "WithdrawalCredentials")
	}

	return &enginev1.BuilderPreregistrationRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: withdrawalCredentials,
	}, nil
}

func (r *VoluntaryExitRequest) ToConsensus() (*enginev1.VoluntaryExitRequest, error) {
	if r == nil {
		return nil, errNilValue
	}

	pubkey, err := bytesutil.DecodeHexWithLength(r.Pubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Pubkey")
	}

	return &enginev1.VoluntaryExitRequest{Pubkey: pubkey}, nil
}

func (r *ExecutionRequestsPQ) ToConsensus() (*enginev1.ExecutionRequestsPQ, error) {
	if r == nil {
		return nil, errNilValue
	}

	// The PQ requests keep the requests of Gloas: convert them as Gloas does.
	gloas, err := (&ExecutionRequestsGloas{
		Deposits:        r.Deposits,
		Withdrawals:     r.Withdrawals,
		Consolidations:  r.Consolidations,
		BuilderDeposits: r.BuilderDeposits,
		BuilderExits:    r.BuilderExits,
	}).ToConsensus()
	if err != nil {
		return nil, err
	}

	config := params.BeaconConfig()

	if err := slice.VerifyMaxLength(r.ValidatorPreregistrations, config.MaxValidatorPreregistrationRequestsPerPayload); err != nil {
		return nil, server.NewDecodeError(err, "ValidatorPreregistrations")
	}

	validatorPreregistrations := make([]*enginev1.ValidatorPreregistrationRequest, len(r.ValidatorPreregistrations))
	for i, request := range r.ValidatorPreregistrations {
		if validatorPreregistrations[i], err = request.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ValidatorPreregistrations[%d]", i))
		}
	}

	if err := slice.VerifyMaxLength(r.BuilderPreregistrations, config.MaxBuilderPreregistrationRequestsPerPayload); err != nil {
		return nil, server.NewDecodeError(err, "BuilderPreregistrations")
	}

	builderPreregistrations := make([]*enginev1.BuilderPreregistrationRequest, len(r.BuilderPreregistrations))
	for i, request := range r.BuilderPreregistrations {
		if builderPreregistrations[i], err = request.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("BuilderPreregistrations[%d]", i))
		}
	}

	if err := slice.VerifyMaxLength(r.VoluntaryExits, config.MaxVoluntaryExitRequestsPerPayload); err != nil {
		return nil, server.NewDecodeError(err, "VoluntaryExits")
	}

	voluntaryExits := make([]*enginev1.VoluntaryExitRequest, len(r.VoluntaryExits))
	for i, request := range r.VoluntaryExits {
		if voluntaryExits[i], err = request.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("VoluntaryExits[%d]", i))
		}
	}

	return &enginev1.ExecutionRequestsPQ{
		Deposits:                  gloas.Deposits,
		Withdrawals:               gloas.Withdrawals,
		Consolidations:            gloas.Consolidations,
		BuilderDeposits:           gloas.BuilderDeposits,
		BuilderExits:              gloas.BuilderExits,
		ValidatorPreregistrations: validatorPreregistrations,
		BuilderPreregistrations:   builderPreregistrations,
		VoluntaryExits:            voluntaryExits,
	}, nil
}

func ExecutionPayloadEnvelopePQFromConsensus(e *eth.ExecutionPayloadEnvelopePQ) (*ExecutionPayloadEnvelopePQ, error) {
	payload, err := ExecutionPayloadGloasFromConsensus(e.Payload)
	if err != nil {
		return nil, err
	}

	var requests *ExecutionRequestsPQ
	if e.ExecutionRequests != nil {
		requests = ExecutionRequestsPQFromConsensus(e.ExecutionRequests)
	}

	return &ExecutionPayloadEnvelopePQ{
		Payload:               payload,
		ExecutionRequests:     requests,
		BuilderIndex:          fmt.Sprintf("%d", e.BuilderIndex),
		BeaconBlockRoot:       hexutil.Encode(e.BeaconBlockRoot),
		ParentBeaconBlockRoot: hexutil.Encode(e.ParentBeaconBlockRoot),
	}, nil
}

func (e *ExecutionPayloadEnvelopePQ) ToConsensus() (*eth.ExecutionPayloadEnvelopePQ, error) {
	if e == nil {
		return nil, errNilValue
	}

	if e.Payload == nil {
		return nil, server.NewDecodeError(errNilValue, "Payload")
	}

	payload, err := e.Payload.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Payload")
	}

	var requests *enginev1.ExecutionRequestsPQ
	if e.ExecutionRequests != nil {
		if requests, err = e.ExecutionRequests.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, "ExecutionRequests")
		}
	}

	builderIndex, err := strconv.ParseUint(e.BuilderIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderIndex")
	}

	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(e.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BeaconBlockRoot")
	}

	parentBeaconBlockRoot, err := bytesutil.DecodeHexWithLength(e.ParentBeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBeaconBlockRoot")
	}

	return &eth.ExecutionPayloadEnvelopePQ{
		Payload:               payload,
		ExecutionRequests:     requests,
		BuilderIndex:          primitives.BuilderIndex(builderIndex),
		BeaconBlockRoot:       beaconBlockRoot,
		ParentBeaconBlockRoot: parentBeaconBlockRoot,
	}, nil
}

func SignedExecutionPayloadEnvelopePQFromConsensus(e *eth.SignedExecutionPayloadEnvelopePQ) (*SignedExecutionPayloadEnvelopePQ, error) {
	message, err := ExecutionPayloadEnvelopePQFromConsensus(e.Message)
	if err != nil {
		return nil, err
	}

	return &SignedExecutionPayloadEnvelopePQ{
		Message:   message,
		Signature: hexutil.Encode(e.Signature),
	}, nil
}

func (e *SignedExecutionPayloadEnvelopePQ) ToConsensus() (*eth.SignedExecutionPayloadEnvelopePQ, error) {
	if e == nil {
		return nil, errNilValue
	}

	message, err := e.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(e.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedExecutionPayloadEnvelopePQ{
		Message:   message,
		Signature: signature,
	}, nil
}

func SignedExecutionPayloadEnvelopeContentsPQFromConsensus(
	signed *eth.SignedExecutionPayloadEnvelopePQ,
	kzgProofs, blobs [][]byte,
) (*SignedExecutionPayloadEnvelopeContentsPQ, error) {
	envelope, err := SignedExecutionPayloadEnvelopePQFromConsensus(signed)
	if err != nil {
		return nil, err
	}

	return &SignedExecutionPayloadEnvelopeContentsPQ{
		SignedExecutionPayloadEnvelope: envelope,
		KzgProofs:                      encodeHexList(kzgProofs),
		Blobs:                          encodeHexList(blobs),
	}, nil
}

// ToConsensus returns the signed envelope, the KZG proofs and the blobs.
func (c *SignedExecutionPayloadEnvelopeContentsPQ) ToConsensus() (*eth.SignedExecutionPayloadEnvelopePQ, [][]byte, [][]byte, error) {
	if c == nil {
		return nil, nil, nil, errNilValue
	}

	envelope, err := c.SignedExecutionPayloadEnvelope.ToConsensus()
	if err != nil {
		return nil, nil, nil, server.NewDecodeError(err, "SignedExecutionPayloadEnvelope")
	}

	kzgProofs, err := decodeHexList(c.KzgProofs, "KzgProofs")
	if err != nil {
		return nil, nil, nil, err
	}

	blobs, err := decodeHexList(c.Blobs, "Blobs")
	if err != nil {
		return nil, nil, nil, err
	}

	return envelope, kzgProofs, blobs, nil
}

// ----------------------------------------------------------------------------
// Block
// ----------------------------------------------------------------------------

func BeaconBlockPQFromConsensus(b *eth.BeaconBlockPQ) *BeaconBlockPQ {
	body := b.Body

	proposerSlashings := make([]*ProposerSlashing, len(body.ProposerSlashings))
	for i, slashing := range body.ProposerSlashings {
		proposerSlashings[i] = ProposerSlashingPQFromConsensus(slashing)
	}

	attesterSlashings := make([]*AttesterSlashingPQ, len(body.AttesterSlashings))
	for i, slashing := range body.AttesterSlashings {
		attesterSlashings[i] = AttesterSlashingPQFromConsensus(slashing)
	}

	var parentExecutionRequests *ExecutionRequestsPQ
	if body.ParentExecutionRequests != nil {
		parentExecutionRequests = ExecutionRequestsPQFromConsensus(body.ParentExecutionRequests)
	}

	var signedBid *SignedExecutionPayloadBid
	if body.SignedExecutionPayloadBid != nil {
		signedBid = SignedExecutionPayloadBidPQFromConsensus(body.SignedExecutionPayloadBid)
	}

	return &BeaconBlockPQ{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyPQ{
			RandaoReveal:              VRFProofFromConsensus(body.RandaoReveal),
			Eth1Data:                  Eth1DataFromConsensus(body.Eth1Data),
			Graffiti:                  hexutil.Encode(body.Graffiti),
			ProposerSlashings:         proposerSlashings,
			AttesterSlashings:         attesterSlashings,
			Attestations:              AttsPQFromConsensus(body.Attestations),
			Deposits:                  DepositsFromConsensus(body.Deposits),
			SignedExecutionPayloadBid: signedBid,
			ParentExecutionRequests:   parentExecutionRequests,
			Proof:                     hexutil.Encode(body.Proof),
		},
	}
}

func SignedBeaconBlockPQFromConsensus(b *eth.SignedBeaconBlockPQ) *SignedBeaconBlockPQ {
	return &SignedBeaconBlockPQ{
		Message:   BeaconBlockPQFromConsensus(b.Block),
		Signature: hexutil.Encode(b.Signature),
	}
}

func (b *BeaconBlockPQ) ToConsensus() (*eth.BeaconBlockPQ, error) {
	if b == nil {
		return nil, errNilValue
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}

	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}

	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}

	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}

	body, err := b.Body.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Body")
	}

	return &eth.BeaconBlockPQ{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body:          body,
	}, nil
}

func (b *BeaconBlockBodyPQ) ToConsensus() (*eth.BeaconBlockBodyPQ, error) {
	if b == nil {
		return nil, errNilValue
	}

	if b.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Eth1Data")
	}

	if b.SignedExecutionPayloadBid == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedExecutionPayloadBid")
	}

	randaoReveal, err := VRFProofToConsensus(b.RandaoReveal)
	if err != nil {
		return nil, server.NewDecodeError(err, "RandaoReveal")
	}

	depositRoot, err := bytesutil.DecodeHexWithLength(b.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.DepositRoot")
	}

	depositCount, err := strconv.ParseUint(b.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.DepositCount")
	}

	blockHash, err := bytesutil.DecodeHexWithLength(b.Eth1Data.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.BlockHash")
	}

	graffiti, err := bytesutil.DecodeHexWithLength(b.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Graffiti")
	}

	if err := slice.VerifyMaxLength(b.ProposerSlashings, params.BeaconConfig().MaxProposerSlashings); err != nil {
		return nil, server.NewDecodeError(err, "ProposerSlashings")
	}

	proposerSlashings := make([]*eth.ProposerSlashingPQ, len(b.ProposerSlashings))
	for i, slashing := range b.ProposerSlashings {
		if proposerSlashings[i], err = slashing.ToConsensusPQ(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ProposerSlashings[%d]", i))
		}
	}

	if err := slice.VerifyMaxLength(b.AttesterSlashings, params.BeaconConfig().MaxAttesterSlashingsElectra); err != nil {
		return nil, server.NewDecodeError(err, "AttesterSlashings")
	}

	attesterSlashings := make([]*eth.AttesterSlashingPQ, len(b.AttesterSlashings))
	for i, slashing := range b.AttesterSlashings {
		if attesterSlashings[i], err = slashing.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("AttesterSlashings[%d]", i))
		}
	}

	attestations, err := AttsPQToConsensus(b.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Attestations")
	}

	deposits, err := DepositsToConsensus(b.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Deposits")
	}

	signedBid, err := b.SignedExecutionPayloadBid.ToConsensusPQ()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedExecutionPayloadBid")
	}

	var parentExecutionRequests *enginev1.ExecutionRequestsPQ
	if b.ParentExecutionRequests != nil {
		if parentExecutionRequests, err = b.ParentExecutionRequests.ToConsensus(); err != nil {
			return nil, server.NewDecodeError(err, "ParentExecutionRequests")
		}
	}

	proof, err := decodeAttestationProof(b.Proof)
	if err != nil {
		return nil, server.NewDecodeError(err, "Proof")
	}

	return &eth.BeaconBlockBodyPQ{
		RandaoReveal: randaoReveal,
		Eth1Data: &eth.Eth1Data{
			DepositRoot:  depositRoot,
			DepositCount: depositCount,
			BlockHash:    blockHash,
		},
		Graffiti:                  graffiti,
		ProposerSlashings:         proposerSlashings,
		AttesterSlashings:         attesterSlashings,
		Attestations:              attestations,
		Deposits:                  deposits,
		SignedExecutionPayloadBid: signedBid,
		ParentExecutionRequests:   parentExecutionRequests,
		Proof:                     proof,
	}, nil
}

func (b *SignedBeaconBlockPQ) ToConsensus() (*eth.SignedBeaconBlockPQ, error) {
	if b == nil {
		return nil, errNilValue
	}

	block, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	signature, err := decodeXMSSSignature(b.Signature)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedBeaconBlockPQ{
		Block:     block,
		Signature: signature,
	}, nil
}

func BlockContentsPQFromConsensus(
	block *eth.BeaconBlockPQ,
	envelope *eth.ExecutionPayloadEnvelopePQ,
	kzgProofs, blobs [][]byte,
) (*BlockContentsPQ, error) {
	encodedEnvelope, err := ExecutionPayloadEnvelopePQFromConsensus(envelope)
	if err != nil {
		return nil, err
	}

	return &BlockContentsPQ{
		Block:                    BeaconBlockPQFromConsensus(block),
		ExecutionPayloadEnvelope: encodedEnvelope,
		KzgProofs:                encodeHexList(kzgProofs),
		Blobs:                    encodeHexList(blobs),
	}, nil
}

// ToConsensus returns the block, the envelope of its payload, the KZG proofs and the blobs.
func (c *BlockContentsPQ) ToConsensus() (*eth.BeaconBlockPQ, *eth.ExecutionPayloadEnvelopePQ, [][]byte, [][]byte, error) {
	if c == nil {
		return nil, nil, nil, nil, errNilValue
	}

	block, err := c.Block.ToConsensus()
	if err != nil {
		return nil, nil, nil, nil, server.NewDecodeError(err, "Block")
	}

	envelope, err := c.ExecutionPayloadEnvelope.ToConsensus()
	if err != nil {
		return nil, nil, nil, nil, server.NewDecodeError(err, "ExecutionPayloadEnvelope")
	}

	kzgProofs, err := decodeHexList(c.KzgProofs, "KzgProofs")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	blobs, err := decodeHexList(c.Blobs, "Blobs")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return block, envelope, kzgProofs, blobs, nil
}

// ----------------------------------------------------------------------------
// Private helpers
// ----------------------------------------------------------------------------

// decodeXMSSSignature decodes a hex encoded leanXMSS signature.
func decodeXMSSSignature(s string) ([]byte, error) {
	return bytesutil.DecodeHexWithLength(s, fieldparams.XMSSSignatureLength)
}

// decodeAttestationProof decodes a hex encoded attestation proof, a `ByteList[MAX_STARK_PROOF_SIZE]`.
func decodeAttestationProof(s string) ([]byte, error) {
	proof, err := hexutil.Decode(s)
	if err != nil {
		return nil, err
	}

	if maxSize := params.BeaconConfig().MaxStarkProofSize; uint64(len(proof)) > maxSize {
		return nil, fmt.Errorf("proof of %d bytes exceeds the maximum of %d bytes", len(proof), maxSize)
	}

	return proof, nil
}

// decodeCommitteeBitsPQ decodes hex encoded PQ committee bits, a `Bitvector[MAX_COMMITTEES_PER_SLOT_PQ]`.
func decodeCommitteeBitsPQ(s string) ([]byte, error) {
	bits, err := hexutil.Decode(s)
	if err != nil {
		return nil, err
	}

	if expected := (params.BeaconConfig().MaxCommitteesPerSlotPQ + 7) / 8; uint64(len(bits)) != expected {
		return nil, fmt.Errorf("committee bits of %d bytes, expected %d bytes", len(bits), expected)
	}

	return bits, nil
}

func encodeHexList(values [][]byte) []string {
	encoded := make([]string, len(values))
	for i, value := range values {
		encoded[i] = hexutil.Encode(value)
	}

	return encoded
}

func decodeHexList(values []string, name string) ([][]byte, error) {
	decoded := make([][]byte, len(values))
	for i, value := range values {
		var err error
		if decoded[i], err = hexutil.Decode(value); err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("%s[%d]", name, i))
		}
	}

	return decoded, nil
}
