package types

import (
	"fmt"
	"math"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	log "github.com/sirupsen/logrus"
)

func InitForkCfg(start, end int, c *params.BeaconChainConfig) *params.BeaconChainConfig {
	c = c.Copy()
	if end < start {
		panic("end fork is less than the start fork") // lint:nopanic -- test code.
	}
	if start < version.Bellatrix {
		log.Fatal("E2e tests require starting from Bellatrix or later (pre-merge forks are not supported)") // lint:nopanic -- test code.
	}
	if start >= version.Altair {
		c.AltairForkEpoch = 0
	}
	if start >= version.Bellatrix {
		c.BellatrixForkEpoch = 0
	}
	if start >= version.Capella {
		c.CapellaForkEpoch = 0
	}
	if start >= version.Deneb {
		c.DenebForkEpoch = 0
	}
	if start >= version.Electra {
		c.ElectraForkEpoch = 0
	}
	if start >= version.Fulu {
		c.FuluForkEpoch = 0
	}

	if end < version.Fulu {
		c.FuluForkEpoch = math.MaxUint64
	}
	if end < version.Electra {
		c.ElectraForkEpoch = math.MaxUint64
	}
	if end < version.Deneb {
		c.DenebForkEpoch = math.MaxUint64
	}
	if end < version.Capella {
		c.CapellaForkEpoch = math.MaxUint64
	}
	if end < version.Bellatrix {
		c.BellatrixForkEpoch = math.MaxUint64
	}
	if end < version.Altair {
		c.AltairForkEpoch = math.MaxUint64
	}
	// Time TTD to line up roughly with the bellatrix fork epoch.
	// E2E sets EL block production rate equal to SecondsPerETH1Block to keep the math simple.
	// the chain starts post-merge (AKA post bellatrix) so TTD should be 0.
	ttd := uint64(c.BellatrixForkEpoch) * uint64(c.SlotsPerEpoch) * c.SecondsPerSlot
	c.TerminalTotalDifficulty = fmt.Sprintf("%d", ttd)

	// Update blob schedule to use the modified fork epochs.
	// Only include entries for forks that are enabled (not set to MaxUint64).
	c.BlobSchedule = nil
	if c.DenebForkEpoch != math.MaxUint64 {
		c.BlobSchedule = append(c.BlobSchedule, params.BlobScheduleEntry{
			Epoch: c.DenebForkEpoch, MaxBlobsPerBlock: uint64(c.DeprecatedMaxBlobsPerBlock),
		})
	}
	if c.ElectraForkEpoch != math.MaxUint64 {
		c.BlobSchedule = append(c.BlobSchedule, params.BlobScheduleEntry{
			Epoch: c.ElectraForkEpoch, MaxBlobsPerBlock: uint64(c.DeprecatedMaxBlobsPerBlockElectra),
		})
	}
	c.InitializeForkSchedule()
	return c
}
