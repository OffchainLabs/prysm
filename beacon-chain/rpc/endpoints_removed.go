package rpc

import (
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

// removedEndpoint returns an endpoint responding with 410 Gone for a route that Prysm used to
// serve but which has since been removed from the Beacon API specification. Naming the
// replacement route turns breakage of outdated tooling into an actionable migration hint
// instead of a silent 404. An empty replacement means the route was retired without a successor.
func removedEndpoint(name, template string, methods []string, replacement string) endpoint {
	msg := "This endpoint was removed from the Beacon API specification and is no longer supported."
	if replacement == "" {
		msg += " It has no replacement."
	} else {
		msg = fmt.Sprintf("%s Use %s instead.", msg, replacement)
	}
	return endpoint{
		template: template,
		name:     "removed." + name,
		handler: func(w http.ResponseWriter, _ *http.Request) {
			httputil.HandleError(w, msg, http.StatusGone)
		},
		methods: methods,
	}
}

func (*Service) removedEndpoints() []endpoint {
	return []endpoint{
		removedEndpoint("GetBlock", "/eth/v1/beacon/blocks/{block_id}", []string{http.MethodGet}, "/eth/v2/beacon/blocks/{block_id}"),
		removedEndpoint("GetBlockAttestations", "/eth/v1/beacon/blocks/{block_id}/attestations", []string{http.MethodGet}, "/eth/v2/beacon/blocks/{block_id}/attestations"),
		removedEndpoint("PublishBlock", "/eth/v1/beacon/blocks", []string{http.MethodPost}, "/eth/v2/beacon/blocks"),
		removedEndpoint("PublishBlindedBlock", "/eth/v1/beacon/blinded_blocks", []string{http.MethodPost}, "/eth/v2/beacon/blinded_blocks"),
		removedEndpoint("ListAttestations", "/eth/v1/beacon/pool/attestations", []string{http.MethodGet}, "/eth/v2/beacon/pool/attestations"),
		removedEndpoint("SubmitAttestations", "/eth/v1/beacon/pool/attestations", []string{http.MethodPost}, "/eth/v2/beacon/pool/attestations"),
		removedEndpoint("GetAttesterSlashings", "/eth/v1/beacon/pool/attester_slashings", []string{http.MethodGet}, "/eth/v2/beacon/pool/attester_slashings"),
		removedEndpoint("SubmitAttesterSlashings", "/eth/v1/beacon/pool/attester_slashings", []string{http.MethodPost}, "/eth/v2/beacon/pool/attester_slashings"),
		removedEndpoint("GetDepositSnapshot", "/eth/v1/beacon/deposit_snapshot", []string{http.MethodGet}, ""),
		removedEndpoint("ExpectedWithdrawals", "/eth/v1/builder/states/{state_id}/expected_withdrawals", []string{http.MethodGet}, ""),
		removedEndpoint("GetAggregateAttestation", "/eth/v1/validator/aggregate_attestation", []string{http.MethodGet}, "/eth/v2/validator/aggregate_attestation"),
		removedEndpoint("SubmitAggregateAndProofs", "/eth/v1/validator/aggregate_and_proofs", []string{http.MethodPost}, "/eth/v2/validator/aggregate_and_proofs"),
		removedEndpoint("ProduceBlock", "/eth/v1/validator/blocks/{slot}", []string{http.MethodGet}, "/eth/v3/validator/blocks/{slot}"),
		removedEndpoint("ProduceBlockV2", "/eth/v2/validator/blocks/{slot}", []string{http.MethodGet}, "/eth/v3/validator/blocks/{slot}"),
		removedEndpoint("ProduceBlindedBlock", "/eth/v1/validator/blinded_blocks/{slot}", []string{http.MethodGet}, "/eth/v3/validator/blocks/{slot}"),
	}
}

// removedDebugEndpoints is registered only alongside the live debug endpoints so that the whole
// debug namespace stays dark when debug endpoints are disabled.
func (*Service) removedDebugEndpoints() []endpoint {
	return []endpoint{
		removedEndpoint("GetBeaconState", "/eth/v1/debug/beacon/states/{state_id}", []string{http.MethodGet}, "/eth/v2/debug/beacon/states/{state_id}"),
		removedEndpoint("GetForkChoiceHeads", "/eth/v1/debug/beacon/heads", []string{http.MethodGet}, "/eth/v2/debug/beacon/heads"),
	}
}
