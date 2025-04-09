package eth_test

import (
	"testing"

	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func TestCopyAttestation_Fuzz(t *testing.T) {
	fuzzCopies(t, &eth.Checkpoint{})
	fuzzCopies(t, &eth.AttestationData{})
	fuzzCopies(t, &eth.Attestation{})
	fuzzCopies(t, &eth.AttestationElectra{})
	fuzzCopies(t, &eth.PendingAttestation{})
	fuzzCopies(t, &eth.IndexedAttestation{})
	fuzzCopies(t, &eth.IndexedAttestationElectra{})
	fuzzCopies(t, &eth.AttesterSlashing{})
	fuzzCopies(t, &eth.AttesterSlashingElectra{})
}
