package sync

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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
