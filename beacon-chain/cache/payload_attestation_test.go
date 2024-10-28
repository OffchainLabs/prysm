package cache

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestPayloadAttestationCache(t *testing.T) {
	p := &PayloadAttestationCache{}

	//Test Has seen
	root := [32]byte{'r'}
	idx := uint64(5)
	require.Equal(t, false, p.Seen(root, idx))

	// Test Add
	msg := &eth.PayloadAttestationMessage{
		Signature: bls.NewAggregateSignature().Marshal(),
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: root[:],
			Slot:            1,
			PayloadStatus:   primitives.PAYLOAD_PRESENT,
		},
	}

	// Add new root
	require.NoError(t, p.Add(msg, idx))
	require.Equal(t, true, p.Seen(root, idx))
	require.Equal(t, root, p.root)
	att := p.attestations[primitives.PAYLOAD_PRESENT]
	indices := att.AggregationBits.BitIndices()
	require.DeepEqual(t, []int{int(idx)}, indices)
	singleSig := bytesutil.SafeCopyBytes(msg.Signature)
	require.DeepEqual(t, singleSig, att.Signature)

	// Test Seen
	require.Equal(t, true, p.Seen(root, idx))
	require.Equal(t, false, p.Seen(root, idx+1))

	// Add another attestation on the same data
	msg2 := &eth.PayloadAttestationMessage{
		Signature: bls.NewAggregateSignature().Marshal(),
		Data:      att.Data,
	}
	idx2 := uint64(7)
	require.NoError(t, p.Add(msg2, idx2))
	att = p.attestations[primitives.PAYLOAD_PRESENT]
	indices = att.AggregationBits.BitIndices()
	require.DeepEqual(t, []int{int(idx), int(idx2)}, indices)
	require.DeepNotEqual(t, att.Signature, msg.Signature)

	// Try again the same index
	require.NoError(t, p.Add(msg2, idx2))
	att2 := p.attestations[primitives.PAYLOAD_PRESENT]
	indices = att.AggregationBits.BitIndices()
	require.DeepEqual(t, []int{int(idx), int(idx2)}, indices)
	require.DeepEqual(t, att, att2)

	// Test Seen
	require.Equal(t, true, p.Seen(root, idx2))
	require.Equal(t, false, p.Seen(root, idx2+1))

	// Add another payload status for a different payload status
	msg3 := &eth.PayloadAttestationMessage{
		Signature: bls.NewAggregateSignature().Marshal(),
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: root[:],
			Slot:            1,
			PayloadStatus:   primitives.PAYLOAD_WITHHELD,
		},
	}
	idx3 := uint64(17)

	require.NoError(t, p.Add(msg3, idx3))
	att3 := p.attestations[primitives.PAYLOAD_WITHHELD]
	indices3 := att3.AggregationBits.BitIndices()
	require.DeepEqual(t, []int{int(idx3)}, indices3)
	require.DeepEqual(t, singleSig, att3.Signature)

	// Add a different root
	root2 := [32]byte{'s'}
	msg.Data.BeaconBlockRoot = root2[:]
	require.NoError(t, p.Add(msg, idx))
	require.Equal(t, root2, p.root)
	require.Equal(t, true, p.Seen(root2, idx))
	require.Equal(t, false, p.Seen(root, idx))
	att = p.attestations[primitives.PAYLOAD_PRESENT]
	indices = att.AggregationBits.BitIndices()
	require.DeepEqual(t, []int{int(idx)}, indices)
}
