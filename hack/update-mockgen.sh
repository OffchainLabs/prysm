#!/bin/bash

# Script to update mock files after proto/prysm/v1alpha1/services.proto changes.
# Use a space to separate mock destination from its interfaces.

set -euo pipefail
cd "$(dirname "$0")/.."

mockgen() { go tool mockgen "$@"; }
goimports() { go run golang.org/x/tools/cmd/goimports "$@"; }

mock_path="testing/mock"
iface_mock_path="testing/validator-mock"

# github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1
# ------------------------------------------------------
proto_mocks_v1alpha1=(
      "$mock_path/beacon_service_mock.go BeaconChainClient"
      "$mock_path/beacon_validator_server_mock.go BeaconNodeValidatorServer,BeaconNodeValidator_WaitForActivationServer,BeaconNodeValidator_WaitForChainStartServer,BeaconNodeValidator_StreamSlotsServer"
      "$mock_path/beacon_validator_client_mock.go BeaconNodeValidatorClient,BeaconNodeValidator_WaitForChainStartClient,BeaconNodeValidator_WaitForActivationClient,BeaconNodeValidator_StreamSlotsClient"
      "$mock_path/beacon_altair_validator_server_mock.go BeaconNodeValidator_StreamBlocksAltairServer"
      "$mock_path/node_service_mock.go NodeClient"
)

for ((i = 0; i < ${#proto_mocks_v1alpha1[@]}; i++)); do
    file=${proto_mocks_v1alpha1[i]% *};
    interfaces=${proto_mocks_v1alpha1[i]#* };
    echo "generating $file for interfaces: $interfaces";
    echo
    mockgen -package=mock -destination="$file" github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1 "$interfaces"
done

# github.com/OffchainLabs/prysm/v7/validator/client/iface
# --------------------------------------------------------
iface_mocks=(
      "$iface_mock_path/chain_client_mock.go ChainClient"
      "$iface_mock_path/prysm_chain_client_mock.go PrysmChainClient"
      "$iface_mock_path/node_client_mock.go NodeClient"
      "$iface_mock_path/validator_client_mock.go ValidatorClient"
)

for ((i = 0; i < ${#iface_mocks[@]}; i++)); do
    file=${iface_mocks[i]% *};
    interfaces=${iface_mocks[i]#* };
    echo "generating $file for interfaces: $interfaces";
    mockgen -package=validator_mock -destination="$file" github.com/OffchainLabs/prysm/v7/validator/client/iface "$interfaces"
done

goimports -w "$mock_path/."
gofmt -s -w "$mock_path/."

# github.com/OffchainLabs/prysm/v7/validator/client/beacon-api
# -------------------------------------------------------------
beacon_api_mock_path="validator/client/beacon-api/mock"
beacon_api_mocks=(
      "$beacon_api_mock_path/genesis_mock.go genesis.go"
      "$beacon_api_mock_path/duties_mock.go duties.go"
      "$beacon_api_mock_path/state_validators_mock.go state_validators.go"
      "$beacon_api_mock_path/beacon_block_converter_mock.go beacon_block_converter.go"
)

for ((i = 0; i < ${#beacon_api_mocks[@]}; i++)); do
    file=${beacon_api_mocks[i]% *};
    source=${beacon_api_mocks[i]#* };
    echo "generating $file for file: $source";
    mockgen -package=mock -source="validator/client/beacon-api/$source" -destination="$file"
done

# The JsonRestHandler interface (mocked here as Handler) lives in api/rest, not
# under validator/client/beacon-api, so it gets its own invocation.
echo "generating $beacon_api_mock_path/json_rest_handler_mock.go for file: api/rest/rest_handler.go"
mockgen -package=mock -source=api/rest/rest_handler.go -destination="$beacon_api_mock_path/json_rest_handler_mock.go" Handler

goimports -w "$beacon_api_mock_path/."
gofmt -s -w "$beacon_api_mock_path/."

# github.com/OffchainLabs/prysm/v7/crypto/bls
# --------------------------------------------
crypto_bls_common_mock_path="crypto/bls/common/mock"
crypto_bls_common_mocks=(
      "$crypto_bls_common_mock_path/interface_mock.go interface.go"
)

for ((i = 0; i < ${#crypto_bls_common_mocks[@]}; i++)); do
    file=${crypto_bls_common_mocks[i]% *};
    source=${crypto_bls_common_mocks[i]#* };
    echo "generating $file for file: $source";
    mockgen -package=mock -source="crypto/bls/common/$source" -destination="$file"
done

goimports -w "$crypto_bls_common_mock_path/."
gofmt -s -w "$crypto_bls_common_mock_path/."
