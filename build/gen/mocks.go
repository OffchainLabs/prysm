package main

import (
	"fmt"
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

type mockSpecs struct {
	reflect   []reflectMock // reflection-based mocks (proto + iface interfaces)
	beaconAPI []sourceMock  // source-based mocks under validator/client/beacon-api
	bls       []sourceMock  // source-based mocks under crypto/bls/common

	mockPath          string
	beaconAPIMockPath string
	blsMockPath       string
}

func mockSpecsList() mockSpecs {
	const (
		mockPath      = "testing/mock"
		ifaceMockPath = "testing/validator-mock"
		ifacePkg      = "github.com/OffchainLabs/prysm/v7/validator/client/iface"

		beaconAPIMockPath = "validator/client/beacon-api/mock"
		blsMockPath       = "crypto/bls/common/mock"
	)

	v1alpha1 := []reflectMock{
		{mockPath + "/beacon_service_mock.go", "mock", v1alpha1Pkg, "BeaconChainClient"},
		{mockPath + "/beacon_validator_server_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidatorServer,BeaconNodeValidator_WaitForActivationServer,BeaconNodeValidator_WaitForChainStartServer,BeaconNodeValidator_StreamSlotsServer"},
		{mockPath + "/beacon_validator_client_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidatorClient,BeaconNodeValidator_WaitForChainStartClient,BeaconNodeValidator_WaitForActivationClient,BeaconNodeValidator_StreamSlotsClient"},
		{mockPath + "/beacon_altair_validator_server_mock.go", "mock", v1alpha1Pkg, "BeaconNodeValidator_StreamBlocksAltairServer"},
		{mockPath + "/node_service_mock.go", "mock", v1alpha1Pkg, "NodeClient"},
	}

	iface := []reflectMock{
		{ifaceMockPath + "/chain_client_mock.go", "validator_mock", ifacePkg, "ChainClient"},
		{ifaceMockPath + "/prysm_chain_client_mock.go", "validator_mock", ifacePkg, "PrysmChainClient"},
		{ifaceMockPath + "/node_client_mock.go", "validator_mock", ifacePkg, "NodeClient"},
		{ifaceMockPath + "/validator_client_mock.go", "validator_mock", ifacePkg, "ValidatorClient"},
	}

	beaconAPI := []sourceMock{
		{beaconAPIMockPath + "/genesis_mock.go", "mock", "validator/client/beacon-api/genesis.go", nil},
		{beaconAPIMockPath + "/duties_mock.go", "mock", "validator/client/beacon-api/duties.go", nil},
		{beaconAPIMockPath + "/state_validators_mock.go", "mock", "validator/client/beacon-api/state_validators.go", nil},
		{beaconAPIMockPath + "/beacon_block_converter_mock.go", "mock", "validator/client/beacon-api/beacon_block_converter.go", nil},
		{beaconAPIMockPath + "/json_rest_handler_mock.go", "mock", "api/rest/rest_handler.go", []string{"Handler"}},
	}

	bls := []sourceMock{
		{blsMockPath + "/interface_mock.go", "mock", "crypto/bls/common/interface.go", nil},
	}

	return mockSpecs{
		reflect:           append(v1alpha1, iface...),
		beaconAPI:         beaconAPI,
		bls:               bls,
		mockPath:          mockPath,
		beaconAPIMockPath: beaconAPIMockPath,
		blsMockPath:       blsMockPath,
	}
}

func genMocks() error {
	specs := mockSpecsList()

	for _, mock := range specs.reflect {
		fmt.Printf("generating %s for interfaces: %s\n", mock.dest, mock.interfaces)
		if err := mockgen("-package="+mock.pkg, "-destination="+mock.dest, mock.importPath, mock.interfaces); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(specs.mockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(specs.mockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	for _, mock := range specs.beaconAPI {
		fmt.Printf("Generating %s for file: %s\n", mock.dest, mock.source)
		args := append([]string{"-package=" + mock.pkg, "-source=" + mock.source, "-destination=" + mock.dest}, mock.extra...)
		if err := mockgen(args...); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(specs.beaconAPIMockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(specs.beaconAPIMockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	for _, mock := range specs.bls {
		fmt.Printf("Generating %s for file: %s\n", mock.dest, mock.source)
		if err := mockgen("-package="+mock.pkg, "-source="+mock.source, "-destination="+mock.dest); err != nil {
			return fmt.Errorf("mockgen: %w", err)
		}
	}

	if err := goimports(specs.blsMockPath + "/."); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify(specs.blsMockPath + "/."); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	return nil
}
