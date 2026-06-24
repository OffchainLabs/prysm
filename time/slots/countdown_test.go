package slots

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestCountdownToGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	logrus.SetLevel(logrus.DebugLevel)

	hook := logTest.NewGlobal()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.GenesisCountdownInterval = time.Millisecond * 500
	params.OverrideBeaconConfig(config)

	t.Run("normal countdown", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			defer hook.Reset()
			firstStringResult := "1s until chain genesis"
			genesisReached := "Chain genesis time reached"
			CountdownToGenesis(
				t.Context(),
				prysmTime.Now().Add(2*time.Second),
				params.BeaconConfig().MinGenesisActiveValidatorCount,
				[32]byte{},
			)
			require.LogsContain(t, hook, firstStringResult)
			require.LogsContain(t, hook, genesisReached)
		})
	})

	t.Run("close context", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			defer hook.Reset()
			ctx, cancel := context.WithCancel(t.Context())
			// Cancel after two ticks so both the 4s and 3s countdown logs are deterministic.
			time.AfterFunc(2500*time.Millisecond, cancel)
			CountdownToGenesis(
				ctx,
				prysmTime.Now().Add(5*time.Second),
				params.BeaconConfig().MinGenesisActiveValidatorCount,
				[32]byte{},
			)
			require.LogsContain(t, hook, "4s until chain genesis")
			require.LogsContain(t, hook, "3s until chain genesis")
			require.LogsContain(t, hook, "Context closed, exiting routine")
			require.LogsDoNotContain(t, hook, "Chain genesis time reached")
		})
	})
}
