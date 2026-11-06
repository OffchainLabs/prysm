package node

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

func TestConfigureHistoricalSlasher(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(flags.HistoricalSlasherNode.Name, true, "")
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureHistoricalSlasher(cliCtx))

	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch*4, params.BeaconConfig().SlotsPerArchivedPoint)
	assert.LogsContain(t, hook,
		fmt.Sprintf(
			"Setting %d slots per archive point and %d max RPC page size for historical slasher usage",
			params.BeaconConfig().SlotsPerArchivedPoint,
			int(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().MaxAttestations))),
	)
}

func TestConfigureBuilderHeaderTimeout(t *testing.T) {
	newCliCtx := func(t *testing.T, timeout string) *cli.Context {
		set := flag.NewFlagSet("test", 0)
		set.Duration(flags.BuilderHeaderTimeout.Name, flags.BuilderHeaderTimeout.Value, "")
		if timeout != "" {
			require.NoError(t, set.Set(flags.BuilderHeaderTimeout.Name, timeout))
		}
		return cli.NewContext(&cli.App{}, set, nil)
	}

	t.Run("leaves the config untouched when unset", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		hook := logTest.NewGlobal()
		c := params.BeaconConfig().Copy()
		c.BuilderHeaderTimeout = 1234 * time.Millisecond
		require.NoError(t, params.SetActive(c))

		require.NoError(t, configureBuilderHeaderTimeout(newCliCtx(t, "")))

		assert.Equal(t, 1234*time.Millisecond, params.BeaconConfig().BuilderHeaderTimeout)
		assert.LogsDoNotContain(t, hook, "Overriding the builder API `getHeader` timeout")
	})

	for _, timeout := range []string{"0s", "-1ms"} {
		t.Run("rejects "+timeout, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)

			err := configureBuilderHeaderTimeout(newCliCtx(t, timeout))

			require.ErrorContains(t, fmt.Sprintf("--builder-header-timeout must be greater than 0, got %s", timeout), err)
			assert.Equal(t, params.BuilderProposalDelayTolerance, params.BeaconConfig().BuilderHeaderTimeout)
		})
	}

	// Any positive value is accepted: there is no upper bound, and no relay endpoint is required.
	t.Run("nominal", func(t *testing.T) {
		const timeout = "2500ms"
		params.SetupTestConfigCleanup(t)
		hook := logTest.NewGlobal()

		require.NoError(t, configureBuilderHeaderTimeout(newCliCtx(t, timeout)))

		expected, err := time.ParseDuration(timeout)
		require.NoError(t, err)
		assert.Equal(t, expected, params.BeaconConfig().BuilderHeaderTimeout)
		assert.LogsContain(t, hook, "Overriding the builder API `getHeader` timeout")
	})
}

func TestConfigureBuilderBidSchedule(t *testing.T) {
	newCliCtx := func(t *testing.T, timeout, delay, requests string) *cli.Context {
		set := flag.NewFlagSet("test", 0)
		set.Duration(flags.BuilderBidTimeout.Name, flags.BuilderBidTimeout.Value, "")
		set.Duration(flags.BuilderBidDelay.Name, flags.BuilderBidDelay.Value, "")
		set.Uint64(flags.BuilderBidRequests.Name, flags.BuilderBidRequests.Value, "")
		for name, v := range map[string]string{
			flags.BuilderBidTimeout.Name:  timeout,
			flags.BuilderBidDelay.Name:    delay,
			flags.BuilderBidRequests.Name: requests,
		} {
			if v != "" {
				require.NoError(t, set.Set(name, v))
			}
		}
		return cli.NewContext(&cli.App{}, set, nil)
	}

	t.Run("leaves the config untouched when unset", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		hook := logTest.NewGlobal()
		c := params.BeaconConfig().Copy()
		c.BuilderBidTimeout = 1234 * time.Millisecond
		require.NoError(t, params.SetActive(c))

		require.NoError(t, configureBuilderBidSchedule(newCliCtx(t, "", "", "")))

		assert.Equal(t, 1234*time.Millisecond, params.BeaconConfig().BuilderBidTimeout)
		assert.Equal(t, time.Duration(0), params.BeaconConfig().BuilderBidDelay)
		assert.Equal(t, uint64(1), params.BeaconConfig().BuilderBidRequests)
		assert.LogsDoNotContain(t, hook, "Overriding the builder API execution payload bid schedule")
	})

	for _, tc := range []struct {
		name                     string
		timeout, delay, requests string
		err                      string
	}{
		{name: "zero timeout", timeout: "0s", err: "--builder-bid-timeout must be greater than 0, got 0s"},
		{name: "negative timeout", timeout: "-1ms", err: "--builder-bid-timeout must be greater than 0, got -1ms"},
		{name: "negative delay", delay: "-1ms", err: "--builder-bid-delay must not be negative, got -1ms"},
		{name: "zero requests", requests: "0", err: "--builder-bid-requests must be greater than 0"},
		{name: "timeout alone past the window", timeout: "2501ms", err: "must not exceed 2.5s"},
		{name: "combination past the window", timeout: "600ms", delay: "800ms", requests: "3", err: "--builder-bid-delay (800ms) + --builder-bid-requests (3) * --builder-bid-timeout (600ms) must not exceed 2.5s"},
		{name: "huge request count", requests: "18446744073709551615", err: "must not exceed 2.5s"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)

			err := configureBuilderBidSchedule(newCliCtx(t, tc.timeout, tc.delay, tc.requests))

			require.ErrorContains(t, tc.err, err)
			assert.Equal(t, params.BuilderBidTolerance, params.BeaconConfig().BuilderBidTimeout)
			assert.Equal(t, uint64(1), params.BeaconConfig().BuilderBidRequests)
		})
	}

	for _, tc := range []struct {
		name                     string
		timeout, delay, requests string
		wantTimeout, wantDelay   time.Duration
		wantRequests             uint64
	}{
		{name: "timeout only", timeout: "900ms", wantTimeout: 900 * time.Millisecond, wantRequests: 1},
		{name: "within the window", timeout: "500ms", delay: "500ms", requests: "3", wantTimeout: 500 * time.Millisecond, wantDelay: 500 * time.Millisecond, wantRequests: 3},
		{name: "exactly at the window", timeout: "500ms", delay: "1s", requests: "3", wantTimeout: 500 * time.Millisecond, wantDelay: time.Second, wantRequests: 3},
	} {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			hook := logTest.NewGlobal()

			require.NoError(t, configureBuilderBidSchedule(newCliCtx(t, tc.timeout, tc.delay, tc.requests)))

			assert.Equal(t, tc.wantTimeout, params.BeaconConfig().BuilderBidTimeout)
			assert.Equal(t, tc.wantDelay, params.BeaconConfig().BuilderBidDelay)
			assert.Equal(t, tc.wantRequests, params.BeaconConfig().BuilderBidRequests)
			assert.LogsContain(t, hook, "Overriding the builder API execution payload bid schedule")
		})
	}
}

func TestConfigureSlotsPerArchivedPoint(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Int(flags.SlotsPerArchivedPoint.Name, 0, "")
	require.NoError(t, set.Set(flags.SlotsPerArchivedPoint.Name, strconv.Itoa(100)))
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureSlotsPerArchivedPoint(cliCtx))

	assert.Equal(t, primitives.Slot(100), params.BeaconConfig().SlotsPerArchivedPoint)
}

func TestConfigureProofOfWork(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Uint64(flags.ChainID.Name, 0, "")
	set.Uint64(flags.NetworkID.Name, 0, "")
	set.String(flags.DepositContractFlag.Name, "", "")
	require.NoError(t, set.Set(flags.ChainID.Name, strconv.Itoa(100)))
	require.NoError(t, set.Set(flags.NetworkID.Name, strconv.Itoa(200)))
	require.NoError(t, set.Set(flags.DepositContractFlag.Name, "deposit-contract"))
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureEth1Config(cliCtx))

	assert.Equal(t, uint64(100), params.BeaconConfig().DepositChainID)
	assert.Equal(t, uint64(200), params.BeaconConfig().DepositNetworkID)
	assert.Equal(t, "deposit-contract", params.BeaconConfig().DepositContractAddress)
}

func TestConfigureExecutionSetting(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(flags.SuggestedFeeRecipient.Name, "", "")
	set.Uint64(flags.TerminalTotalDifficultyOverride.Name, 0, "")
	set.String(flags.TerminalBlockHashOverride.Name, "", "")
	set.Uint64(flags.TerminalBlockHashActivationEpochOverride.Name, 0, "")

	require.NoError(t, set.Set(flags.TerminalTotalDifficultyOverride.Name, strconv.Itoa(100)))
	require.NoError(t, set.Set(flags.TerminalBlockHashOverride.Name, "0xA"))
	require.NoError(t, set.Set(flags.TerminalBlockHashActivationEpochOverride.Name, strconv.Itoa(200)))
	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "0xB"))
	cliCtx := cli.NewContext(&app, set, nil)
	err := configureExecutionSetting(cliCtx)
	assert.LogsContain(t, hook, "0xB is not a valid fee recipient address")
	require.NoError(t, err)

	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	cliCtx = cli.NewContext(&app, set, nil)
	err = configureExecutionSetting(cliCtx)
	require.NoError(t, err)
	assert.Equal(t, common.HexToAddress("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"), params.BeaconConfig().DefaultFeeRecipient)

	assert.LogsContain(t, hook,
		"is not a checksum Ethereum address",
	)
	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "0xaAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa"))
	cliCtx = cli.NewContext(&app, set, nil)
	err = configureExecutionSetting(cliCtx)
	require.NoError(t, err)
	assert.Equal(t, common.HexToAddress("0xaAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa"), params.BeaconConfig().DefaultFeeRecipient)

	assert.Equal(t, "100", params.BeaconConfig().TerminalTotalDifficulty)
	assert.Equal(t, common.HexToHash("0xA"), params.BeaconConfig().TerminalBlockHash)
	assert.Equal(t, primitives.Epoch(200), params.BeaconConfig().TerminalBlockHashActivationEpoch)

}

func TestConfigureNetwork(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	bootstrapNodes := cli.StringSlice{}
	set.Var(&bootstrapNodes, cmd.BootstrapNode.Name, "")
	set.Int(flags.ContractDeploymentBlock.Name, 0, "")
	require.NoError(t, set.Set(cmd.BootstrapNode.Name, "node1"))
	require.NoError(t, set.Set(cmd.BootstrapNode.Name, "node2"))
	require.NoError(t, set.Set(flags.ContractDeploymentBlock.Name, strconv.Itoa(100)))
	cliCtx := cli.NewContext(&app, set, nil)

	configureNetwork(cliCtx)

	assert.DeepEqual(t, []string{"node1", "node2"}, params.BeaconNetworkConfig().BootstrapNodes)
	assert.Equal(t, uint64(100), params.BeaconNetworkConfig().ContractDeploymentBlock)
}

func TestConfigureNetwork_ConfigFile(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	context := cli.NewContext(&app, set, nil)

	require.NoError(t, os.WriteFile("flags_test.yaml", fmt.Appendf(nil, "%s:\n - %s\n - %s\n", cmd.BootstrapNode.Name,
		"node1",
		"node2"), 0666))

	require.NoError(t, set.Parse([]string{"test-command", "--" + cmd.ConfigFileFlag.Name, "flags_test.yaml"}))
	comFlags := cmd.WrapFlags([]cli.Flag{
		&cli.StringFlag{
			Name: cmd.ConfigFileFlag.Name,
		},
		&cli.StringSliceFlag{
			Name: cmd.BootstrapNode.Name,
		},
	})
	command := &cli.Command{
		Name:  "test-command",
		Flags: comFlags,
		Before: func(cliCtx *cli.Context) error {
			return cmd.LoadFlagsFromConfig(cliCtx, comFlags)
		},
		Action: func(cliCtx *cli.Context) error {
			require.Equal(t, true, cliCtx.IsSet(cmd.BootstrapNode.Name))

			require.Equal(t, strings.Join([]string{"node1", "node2"}, ","),
				strings.Join(cliCtx.StringSlice(cmd.BootstrapNode.Name), ","))
			return nil
		},
	}
	require.NoError(t, command.Run(context, context.Args().Slice()...))
	require.NoError(t, os.Remove("flags_test.yaml"))
}

func TestAliasFlag(t *testing.T) {
	// Create a new app with the flag
	app := &cli.App{
		Flags: []cli.Flag{flags.HTTPServerHost},
		Action: func(c *cli.Context) error {
			// Test if the alias works and sets the flag correctly
			if c.IsSet("grpc-gateway-host") && c.IsSet("http-host") {
				return nil
			}
			return cli.Exit("Alias or flag not set", 1)
		},
	}

	// Simulate command line arguments that include the alias
	args := []string{"app", "--grpc-gateway-host", "config.yml"}

	// Run the app with the simulated arguments
	err := app.Run(args)

	// Check if the alias set the flag correctly
	assert.NoError(t, err)
}

func TestConfigureGloasBuilderCircuitBreaker(t *testing.T) {
	newCtx := func(t *testing.T, args ...string) *cli.Context {
		app := cli.App{}
		set := flag.NewFlagSet("test", 0)
		set.Uint64(flags.BuilderAllowedFailures.Name, 0, "")
		set.Uint64(flags.BuilderCriticalFailures.Name, 2, "")
		set.Uint64(flags.BuilderBlacklistPeriod.Name, 1, "")
		set.Uint64(flags.BuilderCriticalBlacklistPeriod.Name, 256, "")
		set.Uint64(flags.BuilderRelayBlacklistPeriod.Name, 32, "")
		set.Uint64(flags.BuilderFailureBackOffPeriod.Name, 5, "")
		set.Uint64(flags.BuilderCriticalFailedBuilders.Name, 7, "")
		require.NoError(t, set.Parse(args))
		return cli.NewContext(&app, set, nil)
	}

	t.Run("no flags leaves defaults", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		before := params.BeaconConfig().BuilderBlacklistPeriod
		require.NoError(t, configureGloasBuilderCircuitBreaker(newCtx(t)))
		require.Equal(t, before, params.BeaconConfig().BuilderBlacklistPeriod)
	})

	t.Run("overrides apply", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		ctx := newCtx(t,
			"--"+flags.BuilderAllowedFailures.Name+"=2",
			"--"+flags.BuilderCriticalFailures.Name+"=4",
			"--"+flags.BuilderBlacklistPeriod.Name+"=3",
			"--"+flags.BuilderRelayBlacklistPeriod.Name+"=9",
			"--"+flags.BuilderCriticalFailedBuilders.Name+"=5",
		)
		require.NoError(t, configureGloasBuilderCircuitBreaker(ctx))
		cfg := params.BeaconConfig()
		require.Equal(t, uint64(2), cfg.BuilderAllowedFailures)
		require.Equal(t, uint64(4), cfg.BuilderCriticalFailures)
		require.Equal(t, primitives.Epoch(3), cfg.BuilderBlacklistPeriod)
		require.Equal(t, primitives.Epoch(9), cfg.BuilderRelayBlacklistPeriod)
		require.Equal(t, uint64(5), cfg.BuilderCriticalFailedBuilders)
	})

	invalid := []struct {
		name string
		args []string
	}{
		{"zero blacklist period", []string{"--" + flags.BuilderBlacklistPeriod.Name + "=0"}},
		{"unreachable critical ban", []string{
			"--" + flags.BuilderAllowedFailures.Name + "=3",
			"--" + flags.BuilderCriticalFailures.Name + "=3",
		}},
		{"critical shorter than first offense", []string{
			"--" + flags.BuilderBlacklistPeriod.Name + "=10",
			"--" + flags.BuilderCriticalBlacklistPeriod.Name + "=5",
		}},
		{"zero critical failed builders", []string{"--" + flags.BuilderCriticalFailedBuilders.Name + "=0"}},
		{"zero relay period", []string{"--" + flags.BuilderRelayBlacklistPeriod.Name + "=0"}},
		{"zero failure backoff period", []string{"--" + flags.BuilderFailureBackOffPeriod.Name + "=0"}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			require.NotNil(t, configureGloasBuilderCircuitBreaker(newCtx(t, tt.args...)))
		})
	}
}
