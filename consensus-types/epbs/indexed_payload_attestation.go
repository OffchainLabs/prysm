package epbs

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type IndexedPayloadAttestation struct {
	AttestingIndices []primitives.ValidatorIndex
	Data             *eth.PayloadAttestationData
	Signature        []byte
}

func (x *IndexedPayloadAttestation) GetAttestingIndices() []primitives.ValidatorIndex {
	if x != nil {
		return x.AttestingIndices
	}
	return []primitives.ValidatorIndex(nil)
}

func (x *IndexedPayloadAttestation) GetData() *eth.PayloadAttestationData {
	if x != nil {
		return x.Data
	}
	return nil
}

func (x *IndexedPayloadAttestation) GetSignature() []byte {
	if x != nil {
		return x.Signature
	}
	return nil
}
