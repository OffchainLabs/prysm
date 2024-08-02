package payloadattestation

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var (
	errNilPayloadAttMessage   = errors.New("received nil payload attestation message")
	errNilPayloadAttData      = errors.New("received nil payload attestation data")
	errNilPayloadAttSignature = errors.New("received nil payload attestation signature")
)

// ROMessage represents a read-only payload attestation message.
type ROMessage struct {
	m *ethpb.PayloadAttestationMessage
}

// validatePayloadAtt checks if the given payload attestation message is valid.
func validatePayloadAtt(m *ethpb.PayloadAttestationMessage) error {
	if m == nil {
		return errNilPayloadAttMessage
	}
	if m.Data == nil {
		return errNilPayloadAttData
	}
	if len(m.Signature) == 0 {
		return errNilPayloadAttSignature
	}
	return nil
}

// NewReadOnly creates a new ReadOnly instance after validating the message.
func NewReadOnly(m *ethpb.PayloadAttestationMessage) (ROMessage, error) {
	if err := validatePayloadAtt(m); err != nil {
		return ROMessage{}, err
	}
	return ROMessage{m}, nil
}

// ValidatorIndex returns the validator index from the payload attestation message.
func (r *ROMessage) ValidatorIndex() primitives.ValidatorIndex {
	return r.m.ValidatorIndex
}

// Signature returns the signature from the payload attestation message.
func (r *ROMessage) Signature() [96]byte {
	return bytesutil.ToBytes96(r.m.Signature)
}

// BeaconBlockRoot returns the beacon block root from the payload attestation message.
func (r *ROMessage) BeaconBlockRoot() [32]byte {
	return bytesutil.ToBytes32(r.m.Data.BeaconBlockRoot)
}

// Slot returns the slot from the payload attestation message.
func (r *ROMessage) Slot() primitives.Slot {
	return r.m.Data.Slot
}

// PayloadStatus returns the payload status from the payload attestation message.
func (r *ROMessage) PayloadStatus() primitives.PTCStatus {
	return r.m.Data.PayloadStatus
}

// SigningRoot returns the signing root from the payload attestation message.
func (r *ROMessage) SigningRoot(domain []byte) ([32]byte, error) {
	return signing.ComputeSigningRoot(r.m.Data, domain)
}

// VerifiedROMessage represents a verified read-only payload attestation message.
type VerifiedROMessage struct {
	ROMessage
}

// NewVerifiedROMessage creates a new VerifiedROMessage instance after validating the message.
func NewVerifiedROMessage(r ROMessage) VerifiedROMessage {
	return VerifiedROMessage{r}
}
