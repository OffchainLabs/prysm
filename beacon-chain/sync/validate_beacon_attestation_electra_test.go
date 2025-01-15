package sync

import (
	"context"
	"testing"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_validateCommitteeIndexElectra(t *testing.T) {
	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(1, true)
		ci, res, err := validateCommitteeIndexElectra(ctx, &ethpb.AttestationElectra{Data: &ethpb.AttestationData{}, CommitteeBits: cb})
		require.NoError(t, err)
		assert.Equal(t, pubsub.ValidationAccept, res)
		assert.Equal(t, primitives.CommitteeIndex(1), ci)
	})
	t.Run("non-zero data committee index", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(1, true)
		_, res, err := validateCommitteeIndexElectra(ctx, &ethpb.AttestationElectra{Data: &ethpb.AttestationData{CommitteeIndex: 1}, CommitteeBits: cb})
		assert.NotNil(t, err)
		assert.Equal(t, pubsub.ValidationReject, res)
	})
	t.Run("no committee bits set", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		_, res, err := validateCommitteeIndexElectra(ctx, &ethpb.AttestationElectra{Data: &ethpb.AttestationData{}, CommitteeBits: cb})
		assert.NotNil(t, err)
		assert.Equal(t, pubsub.ValidationReject, res)
	})
	t.Run("more than 1 committee bit set", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		cb.SetBitAt(1, true)
		_, res, err := validateCommitteeIndexElectra(ctx, &ethpb.AttestationElectra{Data: &ethpb.AttestationData{}, CommitteeBits: cb})
		assert.NotNil(t, err)
		assert.Equal(t, pubsub.ValidationReject, res)
	})
}
