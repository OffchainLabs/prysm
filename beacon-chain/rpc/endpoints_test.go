package rpc

import (
	"net/http"
	"slices"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"golang.org/x/exp/maps"
)

func Test_endpoints(t *testing.T) {
	rewardsRoutes := map[string][]string{
		"/eth/v1/beacon/rewards/blocks/{block_id}":         {http.MethodGet},
		"/eth/v1/beacon/rewards/attestations/{epoch}":      {http.MethodPost},
		"/eth/v1/beacon/rewards/sync_committee/{block_id}": {http.MethodPost},
	}

	beaconRoutes := map[string][]string{
		"/eth/v1/beacon/genesis":                                     {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/root":                      {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/fork":                      {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/finality_checkpoints":      {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/validators":                {http.MethodGet, http.MethodPost},
		"/eth/v1/beacon/states/{state_id}/validators/{validator_id}": {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/validator_balances":        {http.MethodGet, http.MethodPost},
		"/eth/v1/beacon/states/{state_id}/committees":                {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/sync_committees":           {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/randao":                    {http.MethodGet},
		"/eth/v1/beacon/headers":                                     {http.MethodGet},
		"/eth/v1/beacon/headers/{block_id}":                          {http.MethodGet},
		"/eth/v1/beacon/blinded_blocks":                              {http.MethodPost},
		"/eth/v2/beacon/blinded_blocks":                              {http.MethodPost},
		"/eth/v1/beacon/blocks":                                      {http.MethodPost},
		"/eth/v2/beacon/blocks":                                      {http.MethodPost},
		"/eth/v2/beacon/blocks/{block_id}":                           {http.MethodGet},
		"/eth/v1/beacon/blocks/{block_id}/root":                      {http.MethodGet},
		"/eth/v1/beacon/blocks/{block_id}/attestations":              {http.MethodGet},
		"/eth/v2/beacon/blocks/{block_id}/attestations":              {http.MethodGet},
		"/eth/v1/beacon/blob_sidecars/{block_id}":                    {http.MethodGet},
		"/eth/v1/beacon/deposit_snapshot":                            {http.MethodGet},
		"/eth/v1/beacon/blinded_blocks/{block_id}":                   {http.MethodGet},
		"/eth/v1/beacon/pool/attestations":                           {http.MethodGet, http.MethodPost},
		"/eth/v2/beacon/pool/attestations":                           {http.MethodGet},
		"/eth/v1/beacon/pool/attester_slashings":                     {http.MethodGet, http.MethodPost},
		"/eth/v2/beacon/pool/attester_slashings":                     {http.MethodGet, http.MethodPost},
		"/eth/v1/beacon/pool/proposer_slashings":                     {http.MethodGet, http.MethodPost},
		"/eth/v1/beacon/pool/sync_committees":                        {http.MethodPost},
		"/eth/v1/beacon/pool/voluntary_exits":                        {http.MethodGet, http.MethodPost},
		"/eth/v1/beacon/pool/bls_to_execution_changes":               {http.MethodGet, http.MethodPost},
		"/prysm/v1/beacon/individual_votes":                          {http.MethodPost},
	}

	lightClientRoutes := map[string][]string{
		"/eth/v1/beacon/light_client/bootstrap/{block_root}": {http.MethodGet},
		"/eth/v1/beacon/light_client/updates":                {http.MethodGet},
		"/eth/v1/beacon/light_client/finality_update":        {http.MethodGet},
		"/eth/v1/beacon/light_client/optimistic_update":      {http.MethodGet},
	}

	builderRoutes := map[string][]string{
		"/eth/v1/builder/states/{state_id}/expected_withdrawals": {http.MethodGet},
	}

	blobRoutes := map[string][]string{
		"/eth/v1/beacon/blob_sidecars/{block_id}": {http.MethodGet},
	}

	configRoutes := map[string][]string{
		"/eth/v1/config/fork_schedule":    {http.MethodGet},
		"/eth/v1/config/spec":             {http.MethodGet},
		"/eth/v1/config/deposit_contract": {http.MethodGet},
	}

	debugRoutes := map[string][]string{
		"/eth/v2/debug/beacon/states/{state_id}": {http.MethodGet},
		"/eth/v2/debug/beacon/heads":             {http.MethodGet},
		"/eth/v1/debug/fork_choice":              {http.MethodGet},
	}

	eventsRoutes := map[string][]string{
		"/eth/v1/events": {http.MethodGet},
	}

	nodeRoutes := map[string][]string{
		"/eth/v1/node/identity":        {http.MethodGet},
		"/eth/v1/node/peers":           {http.MethodGet},
		"/eth/v1/node/peers/{peer_id}": {http.MethodGet},
		"/eth/v1/node/peer_count":      {http.MethodGet},
		"/eth/v1/node/version":         {http.MethodGet},
		"/eth/v1/node/syncing":         {http.MethodGet},
		"/eth/v1/node/health":          {http.MethodGet},
	}

	validatorRoutes := map[string][]string{
		"/eth/v1/validator/duties/attester/{epoch}":        {http.MethodPost},
		"/eth/v1/validator/duties/proposer/{epoch}":        {http.MethodGet},
		"/eth/v1/validator/duties/sync/{epoch}":            {http.MethodPost},
		"/eth/v2/validator/blocks/{slot}":                  {http.MethodGet},
		"/eth/v3/validator/blocks/{slot}":                  {http.MethodGet},
		"/eth/v1/validator/blinded_blocks/{slot}":          {http.MethodGet},
		"/eth/v1/validator/attestation_data":               {http.MethodGet},
		"/eth/v1/validator/aggregate_attestation":          {http.MethodGet},
		"/eth/v1/validator/aggregate_and_proofs":           {http.MethodPost},
		"/eth/v2/validator/aggregate_and_proofs":           {http.MethodPost},
		"/eth/v1/validator/beacon_committee_subscriptions": {http.MethodPost},
		"/eth/v1/validator/sync_committee_subscriptions":   {http.MethodPost},
		"/eth/v1/validator/beacon_committee_selections":    {http.MethodPost},
		"/eth/v1/validator/sync_committee_selections":      {http.MethodPost},
		"/eth/v1/validator/sync_committee_contribution":    {http.MethodGet},
		"/eth/v1/validator/contribution_and_proofs":        {http.MethodPost},
		"/eth/v1/validator/prepare_beacon_proposer":        {http.MethodPost},
		"/eth/v1/validator/register_validator":             {http.MethodPost},
		"/eth/v1/validator/liveness/{epoch}":               {http.MethodPost},
	}

	prysmBeaconRoutes := map[string][]string{
		"/prysm/v1/beacon/weak_subjectivity":                 {http.MethodGet},
		"/eth/v1/beacon/states/{state_id}/validator_count":   {http.MethodGet},
		"/prysm/v1/beacon/states/{state_id}/validator_count": {http.MethodGet},
		"/prysm/v1/beacon/chain_head":                        {http.MethodGet},
		"/prysm/v1/beacon/blobs":                             {http.MethodPost},
	}

	prysmNodeRoutes := map[string][]string{
		"/prysm/node/trusted_peers":              {http.MethodGet, http.MethodPost},
		"/prysm/v1/node/trusted_peers":           {http.MethodGet, http.MethodPost},
		"/prysm/node/trusted_peers/{peer_id}":    {http.MethodDelete},
		"/prysm/v1/node/trusted_peers/{peer_id}": {http.MethodDelete},
	}

	prysmValidatorRoutes := map[string][]string{
		"/prysm/validators/performance":           {http.MethodPost},
		"/prysm/v1/validators/performance":        {http.MethodPost},
		"/prysm/v1/validators/participation":      {http.MethodGet},
		"/prysm/v1/validators/active_set_changes": {http.MethodGet},
	}

	s := &Service{cfg: &Config{}}

	endpoints := s.endpoints(true, nil, nil, nil, nil, nil, nil)
	actualRoutes := make(map[string][]string, len(endpoints))
	for _, e := range endpoints {
		if _, ok := actualRoutes[e.template]; ok {
			actualRoutes[e.template] = append(actualRoutes[e.template], e.methods...)
		} else {
			actualRoutes[e.template] = e.methods
		}
	}
	expectedRoutes := combineMaps(beaconRoutes, builderRoutes, configRoutes, debugRoutes, eventsRoutes, nodeRoutes, validatorRoutes, rewardsRoutes, lightClientRoutes, blobRoutes, prysmValidatorRoutes, prysmNodeRoutes, prysmBeaconRoutes)

	assert.Equal(t, true, maps.EqualFunc(expectedRoutes, actualRoutes, func(actualMethods []string, expectedMethods []string) bool {
		return slices.Equal(expectedMethods, actualMethods)
	}))
}
