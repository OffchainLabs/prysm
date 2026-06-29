package kurtosis

import (
	"strings"
	"testing"
)

func TestPrysmDoppelgangerValidatorScript(t *testing.T) {
	cfg := PrysmDoppelgangerValidatorConfig{
		ServiceName:      "vc-doppelganger-prysm",
		Image:            "gcr.io/offchainlabs/prysm/validator:latest",
		KeystoreArtifact: "1-prysm-geth-0-127",
		BeaconRPC:        "cl-2-prysm-geth:4000",
		BeaconREST:       "http://cl-2-prysm-geth:3500",
	}

	script := prysmDoppelgangerValidatorScript(cfg)
	for _, want := range []string{
		`name="vc-doppelganger-prysm"`,
		`image="gcr.io/offchainlabs/prysm/validator:latest"`,
		`"/network-configs": Directory(artifact_names=["el_cl_genesis_data"])`,
		`"/validator-keys": Directory(artifact_names=["1-prysm-geth-0-127"])`,
		`"/prysm-password": Directory(artifact_names=["prysm-password"])`,
		`"--wallet-dir=/validator-keys/prysm"`,
		`"--wallet-password-file=/prysm-password/prysm-password.txt"`,
		`"--beacon-rpc-provider=cl-2-prysm-geth:4000"`,
		`"--beacon-rest-api-provider=http://cl-2-prysm-geth:3500"`,
		`"--enable-doppelganger"`,
		`"--disable-monitoring=true"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestPrysmDoppelgangerValidatorConfigValidate(t *testing.T) {
	cfg := PrysmDoppelgangerValidatorConfig{
		ServiceName:      "vc-doppelganger-prysm",
		Image:            "validator:latest",
		KeystoreArtifact: "1-prysm-geth-0-127",
		BeaconRPC:        "cl-2-prysm-geth:4000",
		BeaconREST:       "http://cl-2-prysm-geth:3500",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}

	cfg.BeaconRPC = ""
	if err := cfg.validate(); err == nil {
		t.Fatal("expected missing beacon RPC to fail validation")
	}
}
