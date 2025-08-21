package beaconapi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

var getRequests = map[string]endpoint{
	"/beacon/genesis": newMetadata[structs.GetGenesisResponse](v1PathTemplate),
	"/beacon/states/{param1}/root": newMetadata[structs.GetStateRootResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/fork": newMetadata[structs.GetStateForkResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/finality_checkpoints": newMetadata[structs.GetFinalityCheckpointsResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/validators": newMetadata[structs.GetValidatorsResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		}),
		withQueryParams(func(_ primitives.Epoch) []string {
			return []string{"id=0,1"}
		})),
	"/beacon/states/{param1}/validators/{param2}": newMetadata[structs.GetValidatorResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head", "0"}
		})),
	"/beacon/states/{param1}/validator_balances": newMetadata[structs.GetValidatorBalancesResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		}),
		withQueryParams(func(_ primitives.Epoch) []string {
			return []string{"id=0,1"}
		})),
	"/beacon/states/{param1}/committees": newMetadata[structs.GetCommitteesResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		}),
		withQueryParams(func(_ primitives.Epoch) []string {
			return []string{"index=0"}
		})),
	"/beacon/states/{param1}/sync_committees": newMetadata[structs.GetSyncCommitteeResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().AltairForkEpoch),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/randao": newMetadata[structs.GetRandaoResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/pending_consolidations": newMetadata[structs.GetPendingConsolidationsResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().ElectraForkEpoch),
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/pending_deposits": newMetadata[structs.GetPendingDepositsResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().ElectraForkEpoch),
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/states/{param1}/pending_partial_withdrawals": newMetadata[structs.GetPendingPartialWithdrawalsResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().ElectraForkEpoch),
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/headers": newMetadata[structs.GetBlockHeadersResponse](v1PathTemplate),
	"/beacon/headers/{param1}": newMetadata[structs.GetBlockHeaderResponse](
		v1PathTemplate,
		withParams(func(currentEpoch primitives.Epoch) []string {
			slot := uint64(0)
			if currentEpoch > 0 {
				slot = (uint64(currentEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)) - 1
			}
			return []string{fmt.Sprintf("%v", slot)}
		})),
	"/beacon/blocks/{param1}": newMetadata[structs.GetBlockV2Response](
		v2PathTemplate,
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/blocks/{param1}/root": newMetadata[structs.BlockRootResponse](
		v1PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/blocks/{param1}/attestations": newMetadata[structs.GetBlockAttestationsV2Response](
		v2PathTemplate,
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/blob_sidecars/{param1}": newMetadata[structs.SidecarsResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().DenebForkEpoch),
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/rewards/block/{param1}": newMetadata[structs.BlockRewardsResponse](
		v1PathTemplate,
		withStart(params.BeaconConfig().AltairForkEpoch),
		withParams(func(_ primitives.Epoch) []string { return []string{"head"} })),
	"/beacon/blinded_blocks/{param1}": newMetadata[structs.GetBlockV2Response](
		v1PathTemplate,
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/beacon/pool/attestations": newMetadata[structs.ListAttestationsResponse](
		v2PathTemplate,
		withSanityCheckOnly()),
	"/beacon/pool/attester_slashings": newMetadata[structs.GetAttesterSlashingsResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/beacon/pool/proposer_slashings": newMetadata[structs.GetProposerSlashingsResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/beacon/pool/voluntary_exits": newMetadata[structs.ListVoluntaryExitsResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/beacon/pool/bls_to_execution_changes": newMetadata[structs.BLSToExecutionChangesPoolResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/builder/states/{param1}/expected_withdrawals": newMetadata[structs.ExpectedWithdrawalsResponse](
		v1PathTemplate,
		withStart(params.CapellaE2EForkEpoch),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/config/fork_schedule": newMetadata[structs.GetForkScheduleResponse](
		v1PathTemplate,
		withCustomEval(func(p interface{}, lh interface{}) error {
			pResp, ok := p.(*structs.GetForkScheduleResponse)
			if !ok {
				return fmt.Errorf(msgWrongJSON, &structs.GetForkScheduleResponse{}, p)
			}
			lhResp, ok := lh.(*structs.GetForkScheduleResponse)
			if !ok {
				return fmt.Errorf(msgWrongJSON, &structs.GetForkScheduleResponse{}, lh)
			}
			// remove all forks with far-future epoch
			for i := len(pResp.Data) - 1; i >= 0; i-- {
				if pResp.Data[i].Epoch == fmt.Sprintf("%d", params.BeaconConfig().FarFutureEpoch) {
					pResp.Data = append(pResp.Data[:i], pResp.Data[i+1:]...)
				}
			}
			for i := len(lhResp.Data) - 1; i >= 0; i-- {
				if lhResp.Data[i].Epoch == fmt.Sprintf("%d", params.BeaconConfig().FarFutureEpoch) {
					lhResp.Data = append(lhResp.Data[:i], lhResp.Data[i+1:]...)
				}
			}
			return compareJSON(pResp, lhResp)
		})),
	"/config/spec": newMetadata[structs.GetSpecResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/config/deposit_contract": newMetadata[structs.GetDepositContractResponse](v1PathTemplate),
	"/debug/beacon/states/{param1}": newMetadata[structs.GetBeaconStateV2Response](
		v2PathTemplate,
		withSanityCheckOnly(),
		withSsz(),
		withParams(func(_ primitives.Epoch) []string {
			return []string{"head"}
		})),
	"/debug/beacon/heads": newMetadata[structs.GetForkChoiceHeadsV2Response](
		v2PathTemplate,
		withSanityCheckOnly()),
	"/debug/fork_choice": newMetadata[structs.GetForkChoiceDumpResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/node/identity": newMetadata[structs.GetIdentityResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/node/peers": newMetadata[structs.GetPeersResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/node/peer_count": newMetadata[structs.GetPeerCountResponse](
		v1PathTemplate,
		withSanityCheckOnly()),
	"/node/version": newMetadata[structs.GetVersionResponse](
		v1PathTemplate,
		withCustomEval(func(p interface{}, _ interface{}) error {
			pResp, ok := p.(*structs.GetVersionResponse)
			if !ok {
				return fmt.Errorf(msgWrongJSON, &structs.ListAttestationsResponse{}, p)
			}
			if pResp.Data == nil {
				return errEmptyPrysmData
			}
			if !strings.Contains(pResp.Data.Version, "Prysm") {
				return errors.New("version response does not contain Prysm client name")
			}
			return nil
		})),
	"/node/syncing": newMetadata[structs.SyncStatusResponse](v1PathTemplate),
	"/validator/duties/proposer/{param1}": newMetadata[structs.GetProposerDutiesResponse](
		v1PathTemplate,
		withParams(func(currentEpoch primitives.Epoch) []string {
			return []string{fmt.Sprintf("%v", currentEpoch)}
		}),
		withCustomEval(func(p interface{}, lh interface{}) error {
			pResp, ok := p.(*structs.GetProposerDutiesResponse)
			if !ok {
				return fmt.Errorf(msgWrongJSON, &structs.GetProposerDutiesResponse{}, p)
			}
			lhResp, ok := lh.(*structs.GetProposerDutiesResponse)
			if !ok {
				return fmt.Errorf(msgWrongJSON, &structs.GetProposerDutiesResponse{}, lh)
			}
			if pResp.Data == nil {
				return errEmptyPrysmData
			}
			if lhResp.Data == nil {
				return errEmptyLighthouseData
			}
			if lhResp.Data[0].Slot == "0" {
				// Lighthouse returns a proposer for slot 0 and Prysm doesn't
				lhResp.Data = lhResp.Data[1:]
			}
			return compareJSON(pResp, lhResp)
		})),
	"/validator/blocks/{param1}": newMetadata[structs.ProduceBlockV3Response](
		v3PathTemplate,
		withSanityCheckOnly(),
		withParams(func(currentEpoch primitives.Epoch) []string {
			return []string{strconv.FormatUint(uint64(currentEpoch)*uint64(params.BeaconConfig().SlotsPerEpoch)+uint64(params.BeaconConfig().SlotsPerEpoch)/2+1, 10)}
		}),
		withQueryParams(func(_ primitives.Epoch) []string {
			return []string{
				"randao_reveal=0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505",
			}
		})),
}

var (
	postRequests = map[string]endpoint{
		"/beacon/states/{param1}/validators": newMetadata[structs.GetValidatorsResponse](
			v1PathTemplate,
			withParams(func(_ primitives.Epoch) []string {
				return []string{"head"}
			}),
			withPOSTObj(func() interface{} {
				return struct {
					Ids      []string `json:"ids"`
					Statuses []string `json:"statuses"`
				}{Ids: []string{"0", "1"}, Statuses: nil}
			}())),
		"/beacon/states/{param1}/validator_balances": newMetadata[structs.GetValidatorBalancesResponse](
			v1PathTemplate,
			withParams(func(_ primitives.Epoch) []string {
				return []string{"head"}
			}),
			withPOSTObj(func() []string {
				return []string{"0", "1"}
			}())),
		"/beacon/states/{param1}/validator_identities": newMetadata[structs.GetValidatorIdentitiesResponse](
			v1PathTemplate,
			withSanityCheckOnly(), // LH doesn't support the endpoint
			withSsz(),
			withParams(func(_ primitives.Epoch) []string { return []string{"head"} }),
			withPOSTObj([]string{"0", "1"})),
		"/beacon/rewards/sync_committee/{param1}": newMetadata[structs.SyncCommitteeRewardsResponse](
			v1PathTemplate,
			withStart(params.BeaconConfig().AltairForkEpoch),
			withParams(func(_ primitives.Epoch) []string { return []string{"head"} })),
		"/beacon/rewards/attestations/{param1}": newMetadata[structs.AttestationRewardsResponse](
			v1PathTemplate,
			withStart(params.BeaconConfig().AltairForkEpoch),
			withParams(func(currentEpoch primitives.Epoch) []string {
				return []string{fmt.Sprintf("%v", currentEpoch-2)}
			})),
		"/validator/duties/attester/{param1}": newMetadata[structs.GetAttesterDutiesResponse](
			v1PathTemplate,
			withParams(func(currentEpoch primitives.Epoch) []string {
				return []string{fmt.Sprintf("%v", currentEpoch)}
			}),
			withPOSTObj(func() []string {
				validatorIndices := make([]string, 64)
				for i := range validatorIndices {
					validatorIndices[i] = fmt.Sprintf("%d", i)
				}
				return validatorIndices
			}())),
		"/validator/duties/sync/{param1}": newMetadata[structs.GetSyncCommitteeDutiesResponse](
			v1PathTemplate,
			withStart(params.AltairE2EForkEpoch),
			withParams(func(currentEpoch primitives.Epoch) []string {
				return []string{fmt.Sprintf("%v", currentEpoch)}
			}),
			withPOSTObj(func() []string {
				validatorIndices := make([]string, 64)
				for i := range validatorIndices {
					validatorIndices[i] = fmt.Sprintf("%d", i)
				}
				return validatorIndices
			}())),
		"/validator/liveness/{param1}": newMetadata[structs.GetLivenessResponse](
			v1PathTemplate,
			withSanityCheckOnly(),
			withParams(func(currentEpoch primitives.Epoch) []string {
				return []string{fmt.Sprintf("%v", currentEpoch)}
			}),
			withPOSTObj([]string{"0", "1"})),
	}
)
