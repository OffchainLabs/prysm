package eth_test

import (
	"testing"

	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func TestBeaconBlock_Fuzz(t *testing.T) {
	// Phase 0 Full
	fuzzCopies(t, &eth.SignedBeaconBlock{})
	fuzzCopies(t, &eth.BeaconBlock{})
	fuzzCopies(t, &eth.BeaconBlockBody{})
	// Altair Full
	fuzzCopies(t, &eth.SignedBeaconBlockAltair{})
	fuzzCopies(t, &eth.BeaconBlockAltair{})
	fuzzCopies(t, &eth.BeaconBlockBodyAltair{})
	// Bellatrix Full
	fuzzCopies(t, &eth.SignedBeaconBlockBellatrix{})
	fuzzCopies(t, &eth.BeaconBlockBellatrix{})
	fuzzCopies(t, &eth.BeaconBlockBodyBellatrix{})
	// Bellatrix Blinded
	fuzzCopies(t, &eth.SignedBlindedBeaconBlockBellatrix{})
	fuzzCopies(t, &eth.BlindedBeaconBlockBellatrix{})
	fuzzCopies(t, &eth.BlindedBeaconBlockBodyBellatrix{})
	// Capella Full
	fuzzCopies(t, &eth.SignedBeaconBlockCapella{})
	fuzzCopies(t, &eth.BeaconBlockCapella{})
	fuzzCopies(t, &eth.BeaconBlockBodyCapella{})
	// Capella Blinded
	fuzzCopies(t, &eth.SignedBlindedBeaconBlockCapella{})
	fuzzCopies(t, &eth.BlindedBeaconBlockCapella{})
	fuzzCopies(t, &eth.BlindedBeaconBlockBodyCapella{})
	// Deneb Full
	fuzzCopies(t, &eth.SignedBeaconBlockDeneb{})
	fuzzCopies(t, &eth.BeaconBlockDeneb{})
	fuzzCopies(t, &eth.BeaconBlockBodyDeneb{})
	// Deneb Blinded
	fuzzCopies(t, &eth.SignedBlindedBeaconBlockDeneb{})
	fuzzCopies(t, &eth.BlindedBeaconBlockDeneb{})
	fuzzCopies(t, &eth.BlindedBeaconBlockBodyDeneb{})
	// Electra Full
	fuzzCopies(t, &eth.SignedBeaconBlockElectra{})
	fuzzCopies(t, &eth.BeaconBlockElectra{})
	fuzzCopies(t, &eth.BeaconBlockBodyElectra{})
	// Electra Blinded
	fuzzCopies(t, &eth.SignedBlindedBeaconBlockElectra{})
	fuzzCopies(t, &eth.BlindedBeaconBlockElectra{})
	fuzzCopies(t, &eth.BlindedBeaconBlockBodyElectra{})
}

func TestCopyBeaconBlockFields_Fuzz(t *testing.T) {
	fuzzCopies(t, &eth.Eth1Data{})
	fuzzCopies(t, &eth.ProposerSlashing{})
	fuzzCopies(t, &eth.SignedBeaconBlockHeader{})
	fuzzCopies(t, &eth.BeaconBlockHeader{})
	fuzzCopies(t, &eth.Deposit{})
	fuzzCopies(t, &eth.Deposit_Data{})
	fuzzCopies(t, &eth.SignedVoluntaryExit{})
	fuzzCopies(t, &eth.VoluntaryExit{})
	fuzzCopies(t, &eth.SyncAggregate{})
	fuzzCopies(t, &eth.SignedBLSToExecutionChange{})
	fuzzCopies(t, &eth.BLSToExecutionChange{})
	fuzzCopies(t, &eth.HistoricalSummary{})
	fuzzCopies(t, &eth.PendingDeposit{})
}
