package evaluators

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc"
)

var forkDeadline = 1 * time.Minute

// AltairForkTransition ensures that the Altair hard fork has occurred successfully.
var AltairForkTransition = e2etypes.Evaluator{
	Name: "altair_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Altair
		if e2etypes.GenesisFork() >= version.Altair {
			return false
		}
		altair := policies.OnEpoch(params.BeaconConfig().AltairForkEpoch)
		return altair(e)
	},
	Evaluation: altairForkOccurs,
}

// BellatrixForkTransition ensures that the Bellatrix hard fork has occurred successfully.
var BellatrixForkTransition = e2etypes.Evaluator{
	Name: "bellatrix_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Bellatrix
		if e2etypes.GenesisFork() >= version.Bellatrix {
			return false
		}
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: bellatrixForkOccurs,
}

// CapellaForkTransition ensures that the Capella hard fork has occurred successfully.
var CapellaForkTransition = e2etypes.Evaluator{
	Name: "capella_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Capella
		if e2etypes.GenesisFork() >= version.Capella {
			return false
		}
		fEpoch := params.BeaconConfig().CapellaForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: capellaForkOccurs,
}

// DenebForkTransition ensures that the Deneb hard fork has occurred successfully
var DenebForkTransition = e2etypes.Evaluator{
	Name: "deneb_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Deneb
		if e2etypes.GenesisFork() >= version.Deneb {
			return false
		}
		fEpoch := params.BeaconConfig().DenebForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: denebForkOccurs,
}

// ElectraForkTransition ensures that the electra hard fork has occurred successfully
var ElectraForkTransition = e2etypes.Evaluator{
	Name: "electra_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Electra
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		fEpoch := params.BeaconConfig().ElectraForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: electraForkOccurs,
}

// FuluForkTransition ensures that the fulu hard fork has occurred successfully
var FuluForkTransition = e2etypes.Evaluator{
	Name: "fulu_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Fulu
		if e2etypes.GenesisFork() >= version.Fulu {
			return false
		}
		fEpoch := params.BeaconConfig().FuluForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: fuluForkOccurs,
}

func altairForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().AltairForkEpoch, version.Altair)
}

func bellatrixForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().BellatrixForkEpoch, version.Bellatrix)
}

func capellaForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().CapellaForkEpoch, version.Capella)
}

func denebForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().DenebForkEpoch, version.Deneb)
}

func electraForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().ElectraForkEpoch, version.Electra)
}

func fuluForkOccurs(_ *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	return forkOccurs(params.BeaconConfig().FuluForkEpoch, version.Fulu)
}

// forkOccurs polls the head block over the beacon API until the head reaches the fork's first
// slot, then asserts the block is of the expected fork version.
func forkOccurs(forkEpoch primitives.Epoch, expectedFork int) error {
	forkSlot, err := slots.EpochStart(forkEpoch)
	if err != nil {
		return err
	}
	wantVersion := version.String(expectedFork)
	url := fmt.Sprintf("http://localhost:%d/eth/v2/beacon/blocks/head", e2e.TestParams.Ports.PrysmBeaconNodeHTTPPort)

	ctx, cancel := context.WithTimeout(context.Background(), forkDeadline)
	defer cancel()
	ticker := time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	defer ticker.Stop()

	for {
		gotVersion, slot, err := headBlockVersionAndSlot(ctx, url)
		if err != nil {
			return err
		}
		if slot >= forkSlot {
			if gotVersion != wantVersion {
				return fmt.Errorf("wanted a %s block at slot %d but received %s", wantVersion, slot, gotVersion)
			}
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for a %s block at slot >= %d, head is at slot %d", wantVersion, forkSlot, slot)
		}
	}
}

// headBlockVersionAndSlot returns the fork version name and slot of the head block. Only the slot
// is decoded from the block itself, which is a string field in every fork's schema.
func headBlockVersionAndSlot(ctx context.Context, url string) (string, primitives.Slot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		e := httputil.DefaultJsonError{}
		if err = json.NewDecoder(httpResp.Body).Decode(&e); err != nil {
			return "", 0, err
		}
		return "", 0, fmt.Errorf("%s (status code %d)", e.Message, e.Code)
	}

	// Minimal decoding of the head block response to get the slot and version.
	resp := struct {
		Version string `json:"version"`
		Data    struct {
			Message struct {
				Slot string `json:"slot"`
			} `json:"message"`
		} `json:"data"`
	}{}
	if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return "", 0, err
	}

	slot, err := strconv.ParseUint(resp.Data.Message.Slot, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("could not parse head block slot %q: %w", resp.Data.Message.Slot, err)
	}

	return resp.Version, primitives.Slot(slot), nil
}
