// EIP-8243 wire union. SSZ unions are not modeled by protoc, so this type is
// hand-written rather than generated. See eip-8243-implementation-plan.md §4.
package eth

import (
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	fastssz "github.com/prysmaticlabs/fastssz"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Selector values for the SSZ union on the attestation gossip topic.
const (
	WireSelectorSingle uint8 = 0x00
	WireSelectorBatch  uint8 = 0x01
)

// ErrWireAttestationUnknownSelector is returned when SSZ-decoded bytes carry
// a selector outside {0x00, 0x01}. EIP-8243 reserves all other selectors;
// the implementation rejects them strictly rather than
// quietly accepting any non-zero byte.
var ErrWireAttestationUnknownSelector = fmt.Errorf("wire attestation: unknown selector")

// WireAttestation is the SSZ union published on the attestation subnet
// topic after the EIP-8243 fork. Exactly one of Single or Batch is non-nil,
// determined by Selector. Validation logic must unwrap this via Inner()
// before processing.
type WireAttestation struct {
	Selector uint8
	Single   *SingleAttestation
	Batch    *BatchAttestation
}

// Inner returns the typed inner attestation as eth.Att.
func (w *WireAttestation) Inner() (Att, error) {
	switch w.Selector {
	case WireSelectorSingle:
		if w.Single == nil {
			return nil, fmt.Errorf("wire attestation: nil single under selector 0x00")
		}
		return w.Single, nil
	case WireSelectorBatch:
		if w.Batch == nil {
			return nil, fmt.Errorf("wire attestation: nil batch under selector 0x01")
		}
		return w.Batch, nil
	default:
		return nil, fmt.Errorf("%w: %#x", ErrWireAttestationUnknownSelector, w.Selector)
	}
}

// ----------------------------------------------------------------------------
// fastssz.Marshaler / fastssz.Unmarshaler
// ----------------------------------------------------------------------------

// MarshalSSZ encodes the union as `selector_byte || ssz(inner)`.
func (w *WireAttestation) MarshalSSZ() ([]byte, error) {
	var inner []byte
	var err error
	switch w.Selector {
	case WireSelectorSingle:
		if w.Single == nil {
			return nil, fmt.Errorf("wire attestation: nil single under selector 0x00")
		}
		inner, err = w.Single.MarshalSSZ()
	case WireSelectorBatch:
		if w.Batch == nil {
			return nil, fmt.Errorf("wire attestation: nil batch under selector 0x01")
		}
		inner, err = w.Batch.MarshalSSZ()
	default:
		return nil, fmt.Errorf("%w: %#x", ErrWireAttestationUnknownSelector, w.Selector)
	}
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+len(inner))
	out[0] = w.Selector
	copy(out[1:], inner)
	return out, nil
}

// MarshalSSZTo appends the union encoding to buf.
func (w *WireAttestation) MarshalSSZTo(buf []byte) ([]byte, error) {
	b, err := w.MarshalSSZ()
	if err != nil {
		return buf, err
	}
	return append(buf, b...), nil
}

// SizeSSZ returns the byte length of the SSZ encoding.
func (w *WireAttestation) SizeSSZ() int {
	switch w.Selector {
	case WireSelectorSingle:
		if w.Single == nil {
			return 1
		}
		return 1 + w.Single.SizeSSZ()
	case WireSelectorBatch:
		if w.Batch == nil {
			return 1
		}
		return 1 + w.Batch.SizeSSZ()
	default:
		return 1
	}
}

// UnmarshalSSZ decodes `selector_byte || ssz(inner)`. Unknown selectors are
// rejected.
func (w *WireAttestation) UnmarshalSSZ(b []byte) error {
	if len(b) < 1 {
		return fmt.Errorf("wire attestation: empty buffer")
	}
	w.Selector = b[0]
	switch w.Selector {
	case WireSelectorSingle:
		w.Single = &SingleAttestation{}
		w.Batch = nil
		return w.Single.UnmarshalSSZ(b[1:])
	case WireSelectorBatch:
		w.Batch = &BatchAttestation{}
		w.Single = nil
		return w.Batch.UnmarshalSSZ(b[1:])
	default:
		return fmt.Errorf("%w: %#x", ErrWireAttestationUnknownSelector, w.Selector)
	}
}

// HashTreeRoot returns the HTR of the inner attestation. The selector is part
// of the wire framing only — never part of any consensus signing root — so we
// pass the inner HTR through.
func (w *WireAttestation) HashTreeRoot() ([32]byte, error) {
	inner, err := w.Inner()
	if err != nil {
		return [32]byte{}, err
	}
	return inner.HashTreeRoot()
}

// HashTreeRootWith satisfies the fastssz hashing interface by delegating to
// the inner attestation.
func (w *WireAttestation) HashTreeRootWith(hh *fastssz.Hasher) error {
	inner, err := w.Inner()
	if err != nil {
		return err
	}
	return inner.HashTreeRootWith(hh)
}

// ----------------------------------------------------------------------------
// google.golang.org/protobuf/proto.Message
//
// gossipTopicMappings stores types as proto.Message. We satisfy the interface
// for compatibility, but ProtoReflect is never exercised on a WireAttestation
// (only fastssz is) so returning a zero descriptor is safe.
// ----------------------------------------------------------------------------

// Reset clears the union back to its zero value.
func (w *WireAttestation) Reset() {
	*w = WireAttestation{}
}

// String returns a short human form, useful only in error messages.
func (w *WireAttestation) String() string {
	switch w.Selector {
	case WireSelectorSingle:
		return fmt.Sprintf("WireAttestation{selector=single, single=%v}", w.Single)
	case WireSelectorBatch:
		return fmt.Sprintf("WireAttestation{selector=batch, batch=%v}", w.Batch)
	default:
		return fmt.Sprintf("WireAttestation{selector=%#x}", w.Selector)
	}
}

// ProtoMessage is a tag method required by proto.Message.
func (*WireAttestation) ProtoMessage() {}

// ProtoReflect returns a no-op message. WireAttestation never travels through
// protoreflect (only fastssz), so a nil descriptor is acceptable for our flow.
func (*WireAttestation) ProtoReflect() protoreflect.Message {
	return nil
}

// ----------------------------------------------------------------------------
// eth.Att — delegated to inner
//
// All eth.Att methods on WireAttestation forward to the inner attestation.
// Pre-fork, sync pipeline asserts the decoded message as eth.Att; we keep that
// contract intact post-fork by making WireAttestation itself satisfy Att and
// transparently dispatching.
// ----------------------------------------------------------------------------

// Version reports the inner attestation's fork version, or the
// BatchAttestation fork tag when the inner is missing.
func (w *WireAttestation) Version() int {
	if w == nil {
		return version.BatchAttestation
	}
	switch w.Selector {
	case WireSelectorSingle:
		if w.Single != nil {
			return w.Single.Version()
		}
	case WireSelectorBatch:
		if w.Batch != nil {
			return w.Batch.Version()
		}
	}
	return version.BatchAttestation
}

// IsNil returns true when no inner attestation is present.
func (w *WireAttestation) IsNil() bool {
	if w == nil {
		return true
	}
	switch w.Selector {
	case WireSelectorSingle:
		return w.Single == nil || w.Single.IsNil()
	case WireSelectorBatch:
		return w.Batch == nil || w.Batch.IsNil()
	}
	return true
}

// IsSingle delegates to the inner.
func (w *WireAttestation) IsSingle() bool {
	return w != nil && w.Selector == WireSelectorSingle
}

// IsAggregated delegates to the inner.
func (w *WireAttestation) IsAggregated() bool {
	inner, err := w.Inner()
	if err != nil {
		return false
	}
	return inner.IsAggregated()
}

// Clone returns a deep copy of the union.
func (w *WireAttestation) Clone() Att {
	if w == nil {
		return nil
	}
	cp := &WireAttestation{Selector: w.Selector}
	switch w.Selector {
	case WireSelectorSingle:
		cp.Single = w.Single.Copy()
	case WireSelectorBatch:
		cp.Batch = w.Batch.Copy()
	}
	return cp
}

// GetAggregationBits delegates to the inner.
func (w *WireAttestation) GetAggregationBits() bitfield.Bitlist {
	inner, err := w.Inner()
	if err != nil {
		return nil
	}
	return inner.GetAggregationBits()
}

// GetAttestingIndex delegates to the inner.
func (w *WireAttestation) GetAttestingIndex() primitives.ValidatorIndex {
	inner, err := w.Inner()
	if err != nil {
		return 0
	}
	return inner.GetAttestingIndex()
}

// GetData delegates to the inner.
func (w *WireAttestation) GetData() *AttestationData {
	inner, err := w.Inner()
	if err != nil {
		return nil
	}
	return inner.GetData()
}

// CommitteeBitsVal delegates to the inner.
func (w *WireAttestation) CommitteeBitsVal() bitfield.Bitfield {
	inner, err := w.Inner()
	if err != nil {
		return nil
	}
	return inner.CommitteeBitsVal()
}

// GetSignature delegates to the inner.
func (w *WireAttestation) GetSignature() []byte {
	inner, err := w.Inner()
	if err != nil {
		return nil
	}
	return inner.GetSignature()
}

// SetSignature delegates to the inner.
func (w *WireAttestation) SetSignature(sig []byte) {
	inner, err := w.Inner()
	if err != nil {
		return
	}
	inner.SetSignature(sig)
}

// GetCommitteeIndex delegates to the inner.
func (w *WireAttestation) GetCommitteeIndex() primitives.CommitteeIndex {
	inner, err := w.Inner()
	if err != nil {
		return 0
	}
	return inner.GetCommitteeIndex()
}

// Compile-time assertions.
var _ fastssz.Marshaler = (*WireAttestation)(nil)
var _ fastssz.Unmarshaler = (*WireAttestation)(nil)
var _ Att = (*WireAttestation)(nil)
