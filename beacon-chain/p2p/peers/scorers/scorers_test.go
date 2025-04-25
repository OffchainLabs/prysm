package scorers_test

import (
	"io"
	"math"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	resetCfg := features.InitWithReset(&features.Flags{
		EnablePeerScorer: true,
	})
	defer resetCfg()

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:            64,
		BlockBatchLimitBurstFactor: 10,
	})
	defer func() {
		flags.Init(resetFlags)
	}()
	os.Exit(m.Run())
}

// roundScore returns score rounded in accordance with the score manager's rounding factor.
func roundScore(score float64) float64 {
	return math.Round(score*scorers.ScoreRoundingFactor) / scorers.ScoreRoundingFactor
}
