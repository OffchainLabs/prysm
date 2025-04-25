package endtoend

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ev "github.com/OffchainLabs/prysm/v6/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/evaluators/beaconapi"
	e2eParams "github.com/OffchainLabs/prysm/v6/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func e2eMinimal(t *testing.T, cfg *params.BeaconChainConfig, cfgo ...types.E2EConfigOpt) *testRunner {
	params.SetupTestConfigCleanup(t)
	require.NoError(t, params.SetActive(cfg))
	require.NoError(t, e2eParams.Init(t, e2eParams.StandardBeaconCount))

	// Run for 12 epochs if not in long-running to confirm long-running has no issues.
	var err error
	epochsToRun := 16
	epochStr, longRunning := os.LookupEnv("E2E_EPOCHS")
	if longRunning {
		epochsToRun, err = strconv.Atoi(epochStr)
		require.NoError(t, err)
	}
	seed := 0
	seedStr, isValid := os.LookupEnv("E2E_SEED")
	if isValid {
		seed, err = strconv.Atoi(seedStr)
		require.NoError(t, err)
	}
	tracingPort := e2eParams.TestParams.Ports.JaegerTracingPort
	tracingEndpoint := fmt.Sprintf("127.0.0.1:%d", tracingPort)
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsAreActive,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.VerifyBlockGraffiti,
		ev.PeersCheck,
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.SubmitWithdrawal,
		ev.ValidatorsHaveWithdrawn,
		ev.ProcessesDepositsInBlocks,
		ev.ActivatesDepositedValidators,
		ev.DepositedValidatorsAreActive,
		ev.ValidatorsVoteWithTheMajority,
		ev.ColdStateCheckpoint,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.ValidatorSyncParticipation,
		ev.FeeRecipientIsPresent,
		//ev.TransactionsPresent, TODO: Re-enable Transaction evaluator once it tx pool issues are fixed.
	}
	evals = addIfForkSet(evals, cfg.AltairForkEpoch, ev.AltairForkTransition)
	evals = addIfForkSet(evals, cfg.BellatrixForkEpoch, ev.BellatrixForkTransition)
	evals = addIfForkSet(evals, cfg.CapellaForkEpoch, ev.CapellaForkTransition)
	evals = addIfForkSet(evals, cfg.DenebForkEpoch, ev.DenebForkTransition)
	evals = addIfForkSet(evals, cfg.ElectraForkEpoch, ev.ElectraForkTransition)

	testConfig := &types.E2EConfig{
		BeaconFlags: []string{
			fmt.Sprintf("--slots-per-archive-point=%d", params.BeaconConfig().SlotsPerEpoch*16),
			fmt.Sprintf("--tracing-endpoint=http://%s", tracingEndpoint),
			"--enable-tracing",
			"--trace-sample-fraction=1.0",
		},
		ValidatorFlags:      []string{},
		EpochsToRun:         uint64(epochsToRun),
		TestSync:            true,
		TestFeature:         true,
		TestDeposits:        true,
		UsePrysmShValidator: false,
		UsePprof:            true,
		TracingSinkEndpoint: tracingEndpoint,
		Evaluators:          evals,
		EvalInterceptor:     defaultInterceptor,
		Seed:                int64(seed),
	}
	for _, o := range cfgo {
		o(testConfig)
	}
	if testConfig.UseBuilder {
		testConfig.Evaluators = append(testConfig.Evaluators, ev.BuilderIsActive)
	}

	return newTestRunner(t, testConfig)
}

func e2eMainnet(t *testing.T, usePrysmSh, useMultiClient bool, cfg *params.BeaconChainConfig, cfgo ...types.E2EConfigOpt) *testRunner {
	params.SetupTestConfigCleanup(t)
	require.NoError(t, params.SetActive(cfg))
	if useMultiClient {
		require.NoError(t, e2eParams.InitMultiClient(t, e2eParams.StandardBeaconCount, e2eParams.StandardLighthouseNodeCount))
	} else {
		require.NoError(t, e2eParams.Init(t, e2eParams.StandardBeaconCount))
	}
	// Run for 10 epochs if not in long-running to confirm long-running has no issues.
	var err error
	epochsToRun := 16
	epochStr, longRunning := os.LookupEnv("E2E_EPOCHS")
	if longRunning {
		epochsToRun, err = strconv.Atoi(epochStr)
		require.NoError(t, err)
	}
	seed := 0
	seedStr, isValid := os.LookupEnv("E2E_SEED")
	if isValid {
		seed, err = strconv.Atoi(seedStr)
		require.NoError(t, err)
	}
	tracingPort := e2eParams.TestParams.Ports.JaegerTracingPort
	tracingEndpoint := fmt.Sprintf("127.0.0.1:%d", tracingPort)
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.SubmitWithdrawal,
		ev.ValidatorsHaveWithdrawn,
		ev.DepositedValidatorsAreActive,
		ev.ColdStateCheckpoint,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.FeeRecipientIsPresent,
		//ev.TransactionsPresent, TODO: Re-enable Transaction evaluator once it tx pool issues are fixed.
	}
	evals = addIfForkSet(evals, cfg.AltairForkEpoch, ev.AltairForkTransition)
	evals = addIfForkSet(evals, cfg.BellatrixForkEpoch, ev.BellatrixForkTransition)
	evals = addIfForkSet(evals, cfg.CapellaForkEpoch, ev.CapellaForkTransition)
	evals = addIfForkSet(evals, cfg.DenebForkEpoch, ev.DenebForkTransition)
	evals = addIfForkSet(evals, cfg.ElectraForkEpoch, ev.ElectraForkTransition)

	testConfig := &types.E2EConfig{
		BeaconFlags: []string{
			fmt.Sprintf("--slots-per-archive-point=%d", params.BeaconConfig().SlotsPerEpoch*16),
			fmt.Sprintf("--tracing-endpoint=http://%s", tracingEndpoint),
			"--enable-tracing",
			"--trace-sample-fraction=1.0",
		},
		ValidatorFlags:      []string{},
		EpochsToRun:         uint64(epochsToRun),
		TestSync:            true,
		TestFeature:         true,
		TestDeposits:        true,
		UseFixedPeerIDs:     true,
		UsePrysmShValidator: usePrysmSh,
		UsePprof:            true,
		TracingSinkEndpoint: tracingEndpoint,
		Evaluators:          evals,
		EvalInterceptor:     defaultInterceptor,
		Seed:                int64(seed),
	}
	for _, o := range cfgo {
		o(testConfig)
	}

	// In the event we use the cross-client e2e option, we add in an additional
	// evaluator for multiclient runs to verify the beacon api conformance.
	if testConfig.UseValidatorCrossClient {
		testConfig.Evaluators = append(testConfig.Evaluators, beaconapi.MultiClientVerifyIntegrity)
	}
	if testConfig.UseBuilder {
		testConfig.Evaluators = append(testConfig.Evaluators, ev.BuilderIsActive)
	}
	return newTestRunner(t, testConfig)
}

// addIfForkSet appends the specified transition if epoch is valid.
func addIfForkSet(
	evals []types.Evaluator,
	fork primitives.Epoch,
	transition types.Evaluator,
) []types.Evaluator {
	if fork != 0 && fork != params.BeaconConfig().FarFutureEpoch {
		evals = append(evals, transition)
	}
	return evals
}

func scenarioEvals(cfg *params.BeaconChainConfig) []types.Evaluator {
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.VerifyBlockGraffiti,
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.ColdStateCheckpoint,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.ValidatorSyncParticipation,
	}
	evals = addIfForkSet(evals, cfg.AltairForkEpoch, ev.AltairForkTransition)
	evals = addIfForkSet(evals, cfg.BellatrixForkEpoch, ev.BellatrixForkTransition)
	evals = addIfForkSet(evals, cfg.CapellaForkEpoch, ev.CapellaForkTransition)
	evals = addIfForkSet(evals, cfg.DenebForkEpoch, ev.DenebForkTransition)
	evals = addIfForkSet(evals, cfg.ElectraForkEpoch, ev.ElectraForkTransition)
	return evals
}

func scenarioEvalsMulti(cfg *params.BeaconChainConfig) []types.Evaluator {
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.ColdStateCheckpoint,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
	}
	evals = addIfForkSet(evals, cfg.AltairForkEpoch, ev.AltairForkTransition)
	evals = addIfForkSet(evals, cfg.BellatrixForkEpoch, ev.BellatrixForkTransition)
	evals = addIfForkSet(evals, cfg.CapellaForkEpoch, ev.CapellaForkTransition)
	evals = addIfForkSet(evals, cfg.DenebForkEpoch, ev.DenebForkTransition)
	evals = addIfForkSet(evals, cfg.ElectraForkEpoch, ev.ElectraForkTransition)
	return evals
}
