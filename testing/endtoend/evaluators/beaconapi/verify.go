package beaconapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	params2 "github.com/OffchainLabs/prysm/v6/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v6/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// MultiClientVerifyIntegrity tests Beacon API endpoints.
// It compares responses from Prysm and other beacon nodes such as Lighthouse.
// The evaluator is executed on every odd-numbered epoch.
var MultiClientVerifyIntegrity = e2etypes.Evaluator{
	Name:       "beacon_api_multi-client_verify_integrity_epoch_%d",
	Policy:     policies.EveryNEpochs(1, 2),
	Evaluation: verify,
}

const (
	v1PathTemplate = "http://localhost:%d/eth/v1"
	v2PathTemplate = "http://localhost:%d/eth/v2"
	v3PathTemplate = "http://localhost:%d/eth/v3"
)

func verify(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	for beaconNodeIdx := range conns {
		if err := run(beaconNodeIdx); err != nil {
			return err
		}
	}
	return nil
}

func run(nodeIdx int) error {
	genesisResp := &structs.GetGenesisResponse{}
	if err := doJSONGETRequest(v1PathTemplate, "/beacon/genesis", nodeIdx, genesisResp); err != nil {
		return errors.Wrap(err, "error getting genesis data")
	}
	genesisTime, err := strconv.ParseInt(genesisResp.Data.GenesisTime, 10, 64)
	if err != nil {
		return errors.Wrap(err, "could not parse genesis time")
	}
	currentEpoch := slots.EpochsSinceGenesis(time.Unix(genesisTime, 0))

	for path, m := range getRequests {
		if currentEpoch < m.getStart() {
			continue
		}

		apiPath := pathFromParams(path, m.getParams(currentEpoch), m.getQueryParams(currentEpoch))

		if m.sanityCheckOnlyEnabled() {
			resp := m.getPResp()
			if err = doJSONGETRequest(m.getBasePath(), apiPath, nodeIdx, resp); err != nil {
				return errors.Wrapf(err, "issue during Prysm JSON GET request for path %s", apiPath)
			}
			if resp == nil {
				return fmt.Errorf("nil response from Prysm JSON GET request for path %s", apiPath)
			}
			if m.sszEnabled() {
				sszResp, err := doSSZGETRequest(m.getBasePath(), apiPath, nodeIdx)
				if err != nil {
					return errors.Wrapf(err, "issue during Prysm SSZ GET request for path %s", apiPath)
				}
				if sszResp == nil {
					return fmt.Errorf("nil response from Prysm SSZ GET request for path %s", apiPath)
				}
			}
		} else {
			if err = compareGETJSON(nodeIdx, m.getBasePath(), apiPath, m.getPResp(), m.getLHResp(), m.getCustomEval()); err != nil {
				return err
			}
			if m.sszEnabled() {
				b, err := compareGETSSZ(nodeIdx, m.getBasePath(), apiPath)
				if err != nil {
					return err
				}
				m.setSszResp(b)
			}
		}
	}

	for path, m := range postRequests {
		if currentEpoch < m.getStart() {
			continue
		}

		apiPath := pathFromParams(path, m.getParams(currentEpoch), m.getQueryParams(currentEpoch))

		if m.sanityCheckOnlyEnabled() {
			resp := m.getPResp()
			if err = doJSONPOSTRequest(m.getBasePath(), apiPath, nodeIdx, m.getPOSTObj(), resp); err != nil {
				return errors.Wrapf(err, "issue during Prysm JSON POST request for path %s", apiPath)
			}
			if resp == nil {
				return fmt.Errorf("nil response from Prysm JSON POST request for path %s", apiPath)
			}
			if m.sszEnabled() {
				sszResp, err := doSSZPOSTRequest(m.getBasePath(), apiPath, nodeIdx, m.getPOSTObj())
				if err != nil {
					return errors.Wrapf(err, "issue during Prysm SSZ POST request for path %s", apiPath)
				}
				if sszResp == nil {
					return fmt.Errorf("nil response from Prysm SSZ POST request for path %s", apiPath)
				}
			}
		} else {
			if err = comparePOSTJSON(nodeIdx, m.getBasePath(), apiPath, m.getPOSTObj(), m.getPResp(), m.getLHResp(), m.getCustomEval()); err != nil {
				return err
			}
			if m.sszEnabled() {
				b, err := comparePOSTSSZ(nodeIdx, m.getBasePath(), apiPath, m.getPOSTObj())
				if err != nil {
					return err
				}
				m.setSszResp(b)
			}
		}
	}

	return postEvaluation(nodeIdx, getRequests, currentEpoch)
}

// postEvaluation performs additional evaluation after all requests have been completed.
// It is useful for things such as checking if specific fields match between endpoints.
func postEvaluation(nodeIdx int, requests map[string]endpoint, epoch primitives.Epoch) error {
	// verify that block SSZ responses have the correct structure
	blockData := requests["/beacon/blocks/{param1}"]
	blindedBlockData := requests["/beacon/blinded_blocks/{param1}"]
	if epoch < params.BeaconConfig().AltairForkEpoch {
		b := &ethpb.SignedBeaconBlock{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBeaconBlock{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	} else if epoch < params.BeaconConfig().BellatrixForkEpoch {
		b := &ethpb.SignedBeaconBlockAltair{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBeaconBlockAltair{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	} else if epoch < params.BeaconConfig().CapellaForkEpoch {
		b := &ethpb.SignedBeaconBlockBellatrix{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBlindedBeaconBlockBellatrix{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	} else if epoch < params.BeaconConfig().DenebForkEpoch {
		b := &ethpb.SignedBeaconBlockCapella{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBlindedBeaconBlockCapella{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	} else if epoch < params.BeaconConfig().ElectraForkEpoch {
		b := &ethpb.SignedBeaconBlockDeneb{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBlindedBeaconBlockDeneb{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	} else {
		b := &ethpb.SignedBeaconBlockElectra{}
		if err := b.UnmarshalSSZ(blockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
		bb := &ethpb.SignedBlindedBeaconBlockElectra{}
		if err := bb.UnmarshalSSZ(blindedBlockData.getSszResp()); err != nil {
			return errors.Wrap(err, msgSSZUnmarshalFailed)
		}
	}

	// verify that dependent root of proposer duties matches block header
	blockHeaderData := requests["/beacon/headers/{param1}"]
	header, ok := blockHeaderData.getPResp().(*structs.GetBlockHeaderResponse)
	if !ok {
		return fmt.Errorf(msgWrongJSON, &structs.GetBlockHeaderResponse{}, blockHeaderData.getPResp())
	}
	dutiesData := requests["/validator/duties/proposer/{param1}"]
	duties, ok := dutiesData.getPResp().(*structs.GetProposerDutiesResponse)
	if !ok {
		return fmt.Errorf(msgWrongJSON, &structs.GetProposerDutiesResponse{}, dutiesData.getPResp())
	}
	if header.Data.Root != duties.DependentRoot {
		return fmt.Errorf("header root %s does not match duties root %s ", header.Data.Root, duties.DependentRoot)
	}

	// perform a health check
	basePath := fmt.Sprintf(v1PathTemplate, params2.TestParams.Ports.PrysmBeaconNodeHTTPPort+nodeIdx)
	resp, err := http.Get(basePath + "/node/health")
	if err != nil {
		return errors.Wrap(err, "could not perform a health check")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check response's status code is %d", resp.StatusCode)
	}

	syncingData := requests["/node/syncing"]
	sync, ok := syncingData.getPResp().(*structs.SyncStatusResponse)
	if !ok {
		return fmt.Errorf(msgWrongJSON, &structs.SyncStatusResponse{}, syncingData.getPResp())
	}
	headSlot := sync.Data.HeadSlot

	// get attestation data (it needs the current slot)
	if err = compareGETJSON(
		nodeIdx,
		v1PathTemplate,
		fmt.Sprintf("/validator/attestation_data?slot=%s&committee_index=0", headSlot),
		&structs.AttestationData{},
		&structs.AttestationData{},
		nil); err != nil {
		return err
	}

	return nil
}

func compareGETJSON(nodeIdx int, base, path string, pResp, lhResp interface{}, customEval func(interface{}, interface{}) error) error {
	if err := doJSONGETRequest(base, path, nodeIdx, pResp); err != nil {
		return errors.Wrapf(err, "issue during Prysm JSON GET request for path %s", path)
	}
	if err := doJSONGETRequest(base, path, nodeIdx, lhResp, "Lighthouse"); err != nil {
		return errors.Wrapf(err, "issue during Lighthouse JSON GET request for path %s", path)
	}
	if pResp == nil {
		return errEmptyPrysmData
	}
	if lhResp == nil {
		return errEmptyLighthouseData
	}
	if customEval != nil {
		return customEval(pResp, lhResp)
	} else {
		return compareJSON(pResp, lhResp)
	}
}

func comparePOSTJSON(nodeIdx int, base, path string, postObj, pResp, lhResp interface{}, customEval func(interface{}, interface{}) error) error {
	if err := doJSONPOSTRequest(base, path, nodeIdx, postObj, pResp); err != nil {
		return errors.Wrapf(err, "issue during Prysm JSON POST request for path %s", path)
	}
	if err := doJSONPOSTRequest(base, path, nodeIdx, postObj, lhResp, "Lighthouse"); err != nil {
		return errors.Wrapf(err, "issue during Lighthouse JSON POST request for path %s", path)
	}
	if pResp == nil {
		return errEmptyPrysmData
	}
	if lhResp == nil {
		return errEmptyLighthouseData
	}
	if customEval != nil {
		return customEval(pResp, lhResp)
	} else {
		return compareJSON(pResp, lhResp)
	}
}

func compareGETSSZ(nodeIdx int, base, path string) ([]byte, error) {
	pResp, err := doSSZGETRequest(base, path, nodeIdx)
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Prysm SSZ GET request for path %s", path)
	}
	lhResp, err := doSSZGETRequest(base, path, nodeIdx, "Lighthouse")
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Lighthouse SSZ GET request for path %s", path)
	}
	if !bytes.Equal(pResp, lhResp) {
		return nil, errors.New("Prysm SSZ response does not match Lighthouse SSZ response")
	}
	return pResp, nil
}

func comparePOSTSSZ(nodeIdx int, base, path string, postObj interface{}) ([]byte, error) {
	pResp, err := doSSZPOSTRequest(base, path, nodeIdx, postObj)
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Prysm SSZ POST request for path %s", path)
	}
	lhResp, err := doSSZPOSTRequest(base, path, nodeIdx, postObj, "Lighthouse")
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Lighthouse SSZ POST request for path %s", path)
	}
	if !bytes.Equal(pResp, lhResp) {
		return nil, errors.New("Prysm SSZ response does not match Lighthouse SSZ response")
	}
	return pResp, nil
}

func compareJSON(pResp, lhResp interface{}) error {
	if !reflect.DeepEqual(pResp, lhResp) {
		p, err := json.Marshal(pResp)
		if err != nil {
			return errors.Wrap(err, "failed to marshal Prysm response to JSON")
		}
		lh, err := json.Marshal(lhResp)
		if err != nil {
			return errors.Wrap(err, "failed to marshal Lighthouse response to JSON")
		}
		return fmt.Errorf("Prysm response %s does not match Lighthouse response %s", string(p), string(lh))
	}
	return nil
}

func pathFromParams(path string, params []string, queryParams []string) string {
	apiPath := path
	for i := range params {
		apiPath = strings.Replace(apiPath, fmt.Sprintf("{param%d}", i+1), params[i], 1)
	}
	for i := range queryParams {
		if i == 0 {
			apiPath = apiPath + "?" + queryParams[i]
		} else {
			apiPath = apiPath + "&" + queryParams[i]
		}
	}
	return apiPath
}
