package verification

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
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
