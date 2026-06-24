package sync

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestValidateWithBatchVerifier(t *testing.T) {
	_, keys, err := util.DeterministicDepositsAndKeys(10)
	assert.NoError(t, err)
	sig := keys[0].Sign(make([]byte, 32))
	badSig := keys[1].Sign(make([]byte, 32))
	validSet := &bls.SignatureBatch{
		Messages:     [][32]byte{{}},
		PublicKeys:   []bls.PublicKey{keys[0].PublicKey()},
		Signatures:   [][]byte{sig.Marshal()},
		Descriptions: []string{signing.UnknownSignature},
	}
	invalidSet := &bls.SignatureBatch{
		Messages:     [][32]byte{{}},
		PublicKeys:   []bls.PublicKey{keys[0].PublicKey()},
		Signatures:   [][]byte{badSig.Marshal()},
		Descriptions: []string{signing.UnknownSignature},
	}
	tests := []struct {
		name          string
		message       string
		set           *bls.SignatureBatch
		preFilledSets []*bls.SignatureBatch
		want          pubsub.ValidationResult
	}{
		{
			name:    "empty queue",
			message: "random",
			set:     validSet,
			want:    pubsub.ValidationAccept,
		},
		{
			name:    "invalid set",
			message: "random",
			set:     invalidSet,
			want:    pubsub.ValidationReject,
		},
		{
			name:          "invalid set in routine with valid set",
			message:       "random",
			set:           validSet,
			preFilledSets: []*bls.SignatureBatch{invalidSet},
			want:          pubsub.ValidationAccept,
		},
		{
			name:          "valid set in routine with invalid set",
			message:       "random",
			set:           invalidSet,
			preFilledSets: []*bls.SignatureBatch{validSet},
			want:          pubsub.ValidationReject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			svc := &Service{
				ctx:           ctx,
				cfg:           &config{batchVerifierLimit: verifierLimit},
				cancel:        cancel,
				signatureChan: make(chan *signatureVerifier, verifierLimit),
			}
			go svc.verifierRoutine()
			for _, st := range tt.preFilledSets {
				svc.signatureChan <- &signatureVerifier{set: st, resChan: make(chan error, 10)}
			}
			got, err := svc.validateWithBatchVerifier(t.Context(), tt.message, tt.set)
			if got != tt.want {
				t.Errorf("validateWithBatchVerifier() = %v, want %v", got, tt.want)
			}
			if err != nil && tt.want == pubsub.ValidationAccept {
				t.Errorf("Wanted no error but received: %v", err)
			}
			cancel()
		})
	}
}

// Regression test: verifyBatch must not mutate caller-provided SignatureBatch sets.
// Since validateWithBatchVerifier no longer copies the set, any mutation in the
// aggregation/dedup path would corrupt the caller's data.
func TestVerifyBatch_DoesNotMutateInputSets(t *testing.T) {
	_, keys, err := util.DeterministicDepositsAndKeys(10)
	assert.NoError(t, err)

	msg1 := [32]byte{'A'}
	msg2 := [32]byte{'B'}

	sig0 := keys[0].Sign(msg1[:])
	sig1 := keys[1].Sign(msg2[:])
	sig2 := keys[2].Sign(msg1[:]) // Same message as sig0 — triggers AggregateBatch

	set0 := &bls.SignatureBatch{
		Messages:     [][32]byte{msg1},
		PublicKeys:   []bls.PublicKey{keys[0].PublicKey()},
		Signatures:   [][]byte{sig0.Marshal()},
		Descriptions: []string{"sig0"},
	}
	set1 := &bls.SignatureBatch{
		Messages:     [][32]byte{msg2},
		PublicKeys:   []bls.PublicKey{keys[1].PublicKey()},
		Signatures:   [][]byte{sig1.Marshal()},
		Descriptions: []string{"sig1"},
	}
	set2 := &bls.SignatureBatch{
		Messages:     [][32]byte{msg1},
		PublicKeys:   []bls.PublicKey{keys[2].PublicKey()},
		Signatures:   [][]byte{sig2.Marshal()},
		Descriptions: []string{"sig2"},
	}
	// Duplicate of set0 to exercise RemoveDuplicates.
	set3 := &bls.SignatureBatch{
		Messages:     [][32]byte{msg1},
		PublicKeys:   []bls.PublicKey{keys[0].PublicKey()},
		Signatures:   [][]byte{sig0.Marshal()},
		Descriptions: []string{"sig0-dup"},
	}

	// Snapshot original state.
	orig0 := set0.Copy()
	orig1 := set1.Copy()
	orig2 := set2.Copy()
	orig3 := set3.Copy()

	batch := []*signatureVerifier{
		{set: set0, resChan: make(chan error, 1)},
		{set: set1, resChan: make(chan error, 1)},
		{set: set2, resChan: make(chan error, 1)},
		{set: set3, resChan: make(chan error, 1)},
	}

	verifyBatch(batch)

	// Drain results — verification should succeed.
	for _, v := range batch {
		assert.NoError(t, <-v.resChan)
	}

	// Assert caller-provided sets were not mutated.
	assert.DeepEqual(t, orig0, set0)
	assert.DeepEqual(t, orig1, set1)
	assert.DeepEqual(t, orig2, set2)
	assert.DeepEqual(t, orig3, set3)
}
