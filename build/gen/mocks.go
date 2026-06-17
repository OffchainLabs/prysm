package main

import (
	"fmt"
	"slices"
)

func mockgen(args ...string) error {
	return sh("go", append([]string{"tool", "mockgen"}, args...)...)
}

const v1alpha1Pkg = "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"

type reflectMock struct {
	dest, pkg, importPath, interfaces string
}

type sourceMock struct {
	dest, pkg, source string
	extra             []string
}

func genMocks() error {
	const (
		mockPath      = "testing/mock"
		ifaceMockPath = "testing/validator-mock"
	)

	v1alpha1 := []reflectMock{
		{mockPath + "/beacon_service_mock.go", "mock", v1alpha1Pkg, "BeaconChainClient"},
		{mockPath + "/beacon_validator_server_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidatorServer,BeaconNodeValidator_WaitForActivationServer,BeaconNodeValidator_WaitForChainStartServer,BeaconNodeValidator_StreamSlotsServer"},
		{mockPath + "/beacon_validator_client_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidatorClient,BeaconNodeValidator_WaitForChainStartClient,BeaconNodeValidator_WaitForActivationClient,BeaconNodeValidator_StreamSlotsClient"},
		{mockPath + "/beacon_altair_validator_server_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidator_StreamBlocksAltairServer"},
		{mockPath + "/node_service_mock.go", "mock", v1alpha1Pkg, "NodeClient"},
	}

	const ifacePkg = "github.com/OffchainLabs/prysm/v7/validator/client/iface"
	iface := []reflectMock{
		{ifaceMockPath + "/chain_client_mock.go", "validator_mock", ifacePkg, "ChainClient"},
		{ifaceMockPath + "/prysm_chain_client_mock.go", "validator_mock", ifacePkg, "PrysmChainClient"},
		{ifaceMockPath + "/node_client_mock.go", "validator_mock", ifacePkg, "NodeClient"},
		{ifaceMockPath + "/validator_client_mock.go", "validator_mock", ifacePkg, "ValidatorClient"},
	}

	const beaconAPIMockPath = "validator/client/beacon-api/mock"
	beaconAPI := []sourceMock{
		{beaconAPIMockPath + "/genesis_mock.go", "mock", "validator/client/beacon-api/genesis.go", nil},
		{beaconAPIMockPath + "/duties_mock.go", "mock", "validator/client/beacon-api/duties.go", nil},
		{beaconAPIMockPath + "/state_validators_mock.go", "mock", "validator/client/beacon-api/state_validators.go", nil},
		{beaconAPIMockPath + "/beacon_block_converter_mock.go", "mock", "validator/client/beacon-api/beacon_block_converter.go", nil},
		{beaconAPIMockPath + "/json_rest_handler_mock.go", "mock", "api/rest/rest_handler.go", []string{"Handler"}},
	}

	const blsMockPath = "crypto/bls/common/mock"
	bls := []sourceMock{
		{blsMockPath + "/interface_mock.go", "mock", "crypto/bls/common/interface.go", nil},
	}

	for _, mock := range slices.Concat(v1alpha1, iface) {
		fmt.Printf("generating %s for interfaces: %s\n", mock.dest, mock.interfaces)
		if err := mockgen("-package="+mock.pkg, "-destination="+mock.dest, mock.importPath, mock.interfaces); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(mockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(mockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	for _, mock := range beaconAPI {
		fmt.Printf("Generating %s for file: %s\n", mock.dest, mock.source)
		args := append([]string{"-package=" + mock.pkg, "-source=" + mock.source, "-destination=" + mock.dest}, mock.extra...)
		if err := mockgen(args...); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(beaconAPIMockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(beaconAPIMockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	for _, mock := range bls {
		fmt.Printf("Generating %s for file: %s\n", mock.dest, mock.source)
		if err := mockgen("-package="+mock.pkg, "-source="+mock.source, "-destination="+mock.dest); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(blsMockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(blsMockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	return nil
}
