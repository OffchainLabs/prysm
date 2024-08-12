package cache

import (
	"errors"
	"sync"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var errNilPayloadAttestationMessage = errors.New("nil Payload Attestation Message")

// PayloadAttestationCache keeps a map of all the PTC votes that were seen,
// already aggregated. The key is the beacon block root.
type PayloadAttestationCache struct {
	root         [32]byte
	attestations [primitives.PAYLOAD_INVALID_STATUS]*eth.PayloadAttestation
	sync.Mutex
}

// Seen returns true if a vote for the given Beacon Block Root has already been processed
// for this Payload Timeliness Committee index. This will return true even if
// the Payload status differs.
func (p *PayloadAttestationCache) Seen(root [32]byte, idx uint64) bool {
	p.Lock()
	defer p.Unlock()
	if p.root != root {
		return false
	}
	for _, agg := range p.attestations {
		if agg == nil {
			continue
		}
		if agg.AggregationBits.BitAt(idx) {
			return true
		}
	}
	return false
}

// messageToPayloadAttestation creates a PayloadAttestation with a single
// aggregated bit from the passed PayloadAttestationMessage
func messageToPayloadAttestation(att *eth.PayloadAttestationMessage, idx uint64) *eth.PayloadAttestation {
	bits := primitives.NewPayloadAttestationAggregationBits()
	bits.SetBitAt(idx, true)
	data := &eth.PayloadAttestationData{
		BeaconBlockRoot: bytesutil.SafeCopyBytes(att.Data.BeaconBlockRoot),
		Slot:            att.Data.Slot,
		PayloadStatus:   att.Data.PayloadStatus,
	}
	return &eth.PayloadAttestation{
		AggregationBits: bits,
		Data:            data,
		Signature:       bytesutil.SafeCopyBytes(att.Signature),
	}
}

// aggregateSigFromMessage returns the aggregated signature from a Payload
// Attestation by adding the passed signature in the PayloadAttestationMessage,
// no signature validation is performed.
func aggregateSigFromMessage(aggregated *eth.PayloadAttestation, message *eth.PayloadAttestationMessage) ([]byte, error) {
	aggSig, err := bls.SignatureFromBytesNoValidation(aggregated.Signature)
	if err != nil {
		return nil, err
	}
	sig, err := bls.SignatureFromBytesNoValidation(message.Signature)
	if err != nil {
		return nil, err
	}
	return bls.AggregateSignatures([]bls.Signature{aggSig, sig}).Marshal(), nil
}

// Add adds a PayloadAttestationMessage to the internal cache of aggregated
// PayloadAttestations.
// If the index has already been seen for this attestation status the function does nothing.
// If the root is not the cached root, the function will clear the previous cache
// This function assumes that the message has already been validated. In
// particular that the signature is valid and that the block root corresponds to
// the given slot in the attestation data.
func (p *PayloadAttestationCache) Add(att *eth.PayloadAttestationMessage, idx uint64) error {
	if att == nil || att.Data == nil || att.Data.BeaconBlockRoot == nil {
		return errNilPayloadAttestationMessage
	}
	p.Lock()
	defer p.Unlock()
	root := [32]byte(att.Data.BeaconBlockRoot)
	if p.root != root {
		p.root = root
		p.attestations = [primitives.PAYLOAD_INVALID_STATUS]*eth.PayloadAttestation{}
	}
	agg := p.attestations[att.Data.PayloadStatus]
	if agg == nil {
		p.attestations[att.Data.PayloadStatus] = messageToPayloadAttestation(att, idx)
		return nil
	}
	if agg.AggregationBits.BitAt(idx) {
		return nil
	}
	sig, err := aggregateSigFromMessage(agg, att)
	if err != nil {
		return err
	}
	agg.Signature = sig
	agg.AggregationBits.SetBitAt(idx, true)
	return nil
}

// Get returns the aggregated PayloadAttestation for the given root and status
// if the root doesn't exist or status is invalid, the function returns nil.
func (p *PayloadAttestationCache) Get(root [32]byte, status primitives.PTCStatus) *eth.PayloadAttestation {
	p.Lock()
	defer p.Unlock()

	if p.root != root {
		return nil
	}
	if status >= primitives.PAYLOAD_INVALID_STATUS {
		return nil
	}

	return eth.CopyPayloadAttestation(p.attestations[status])
}

// Clear clears the internal map
func (p *PayloadAttestationCache) Clear() {
	p.Lock()
	defer p.Unlock()
	p.root = [32]byte{}
	p.attestations = [primitives.PAYLOAD_INVALID_STATUS]*eth.PayloadAttestation{}
}
