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

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	params2 "github.com/prysmaticlabs/prysm/v5/testing/endtoend/params"
	"github.com/prysmaticlabs/prysm/v5/testing/endtoend/policies"
	e2etypes "github.com/prysmaticlabs/prysm/v5/testing/endtoend/types"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
	if err := doJSONGetRequest(v1PathTemplate, "/beacon/genesis", nodeIdx, genesisResp); err != nil {
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
		apiPath := path
		if m.getParams(currentEpoch) != nil {
			apiPath = pathFromParams(path, m.getParams(currentEpoch))
		}

		if m.sanityCheckOnlyEnabled() {
			resp := m.getPResp()
			if err = doJSONGetRequest(m.getBasePath(), apiPath, nodeIdx, resp); err != nil {
				return errors.Wrapf(err, "issue during Prysm JSON GET request for path %s", apiPath)
			}
			if resp == nil {
				return fmt.Errorf("nil response from Prysm JSON GET request for path %s", apiPath)
			}
			if m.sszEnabled() {
				sszResp, err := doSSZGetRequest(m.getBasePath(), apiPath, nodeIdx)
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
		apiPath := path
		if m.getParams(currentEpoch) != nil {
			apiPath = pathFromParams(path, m.getParams(currentEpoch))
		}
		if err = comparePOSTJSON(nodeIdx, m.getBasePath(), apiPath, m.getPOSTObj(), m.getPResp(), m.getLHResp(), m.getCustomEval()); err != nil {
			return err
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
		return fmt.Errorf(msgWrongJson, &structs.GetBlockHeaderResponse{}, blockHeaderData.getPResp())
	}
	dutiesData := requests["/validator/duties/proposer/{param1}"]
	duties, ok := dutiesData.getPResp().(*structs.GetProposerDutiesResponse)
	if !ok {
		return fmt.Errorf(msgWrongJson, &structs.GetProposerDutiesResponse{}, dutiesData.getPResp())
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

	return nil
}

func compareGETJSON(nodeIdx int, base, path string, pResp, lhResp interface{}, customEval func(interface{}, interface{}) error) error {
	if err := doJSONGetRequest(base, path, nodeIdx, pResp); err != nil {
		return errors.Wrapf(err, "issue during Prysm JSON GET request for path %s", path)
	}
	if err := doJSONGetRequest(base, path, nodeIdx, lhResp, "Lighthouse"); err != nil {
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
	if err := doJSONPostRequest(base, path, nodeIdx, postObj, pResp); err != nil {
		return errors.Wrapf(err, "issue during Prysm JSON POST request for path %s", path)
	}
	if err := doJSONPostRequest(base, path, nodeIdx, postObj, lhResp, "Lighthouse"); err != nil {
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
	pResp, err := doSSZGetRequest(base, path, nodeIdx)
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Prysm SSZ GET request for path %s", path)
	}
	lhResp, err := doSSZGetRequest(base, path, nodeIdx, "Lighthouse")
	if err != nil {
		return nil, errors.Wrapf(err, "issue during Lighthouse SSZ GET request for path %s", path)
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

func pathFromParams(path string, params []string) string {
	apiPath := path
	for i := range params {
		apiPath = strings.Replace(apiPath, fmt.Sprintf("{param%d}", i+1), params[i], 1)
	}
	return apiPath
}
