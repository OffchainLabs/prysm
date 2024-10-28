package util

import (
	"crypto/rand"
	"math/big"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// GenerateRandomPayloadAttestationData generates a random PayloadAttestationData for testing purposes.
func GenerateRandomPayloadAttestationData(t *testing.T) *ethpb.PayloadAttestationData {
	// Generate a random BeaconBlockRoot
	randomBytes := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		t.Fatalf("Failed to generate random BeaconBlockRoot: %v", err)
	}

	// Generate a random Slot value
	randomSlot, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		t.Fatalf("Failed to generate random Slot: %v", err)
	}

	payloadStatuses := []primitives.PTCStatus{
		primitives.PAYLOAD_ABSENT,
		primitives.PAYLOAD_PRESENT,
		primitives.PAYLOAD_WITHHELD,
	}
	// Select a random PayloadStatus
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(payloadStatuses))))
	if err != nil {
		t.Fatalf("Failed to select random PayloadStatus: %v", err)
	}
	randomPayloadStatus := payloadStatuses[index.Int64()]

	return &ethpb.PayloadAttestationData{
		BeaconBlockRoot: randomBytes,
		Slot:            primitives.Slot(randomSlot.Uint64()),
		PayloadStatus:   randomPayloadStatus,
	}
}
