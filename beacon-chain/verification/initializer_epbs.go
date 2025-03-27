package verification

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	payloadattestation "github.com/OffchainLabs/prysm/v6/consensus-types/epbs/payload-attestation"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
)

// NewPayloadAttestationMsgVerifier creates a PayloadAttestationMsgVerifier for a single payload attestation message,
// with the given set of requirements.
func (ini *Initializer) NewPayloadAttestationMsgVerifier(pa payloadattestation.ROMessage, reqs []Requirement) *PayloadAttMsgVerifier {
	return &PayloadAttMsgVerifier{
		sharedResources: ini.shared,
		results:         newResults(reqs...),
		pa:              pa,
	}
}

// NewHeaderVerifier creates a SignedExecutionPayloadHeaderVerifier for a single signed execution payload header,
// with the given set of requirements.
func (ini *Initializer) NewHeaderVerifier(eh interfaces.ROSignedExecutionPayloadHeader, st state.ReadOnlyBeaconState, reqs []Requirement) *HeaderVerifier {
	return &HeaderVerifier{
		sharedResources: ini.shared,
		results:         newResults(reqs...),
		h:               eh,
		st:              st,
	}
}

// NewPayloadEnvelopeVerifier creates a SignedExecutionPayloadEnvelopeVerifier for a single signed execution payload envelope,
//
//	with the given set of requirements.
func (ini *Initializer) NewPayloadEnvelopeVerifier(ee interfaces.ROSignedExecutionPayloadEnvelope, reqs []Requirement) *EnvelopeVerifier {
	return &EnvelopeVerifier{
		results: newResults(reqs...),
		e:       ee,
	}
}
