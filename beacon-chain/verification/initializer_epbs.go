package verification

import (
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
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
