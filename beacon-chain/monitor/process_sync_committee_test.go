package monitor

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestProcessSyncCommitteeContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)

	contrib := &ethpb.SignedContributionAndProof{
		Message: &ethpb.ContributionAndProof{
			AggregatorIndex: 1,
		},
	}

	s.processSyncCommitteeContribution(contrib)
	require.LogsContain(t, hook, "\"Sync committee aggregation processed\" prefix=monitor validatorIndex=1")
	require.LogsDoNotContain(t, hook, "validatorIndex=2")
}

func TestProcessSyncAggregate(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)
	beaconState, _ := util.DeterministicGenesisStateAltair(t, 256)

	block := &ethpb.BeaconBlockAltair{
		Slot: 2,
		Body: &ethpb.BeaconBlockBodyAltair{
			SyncAggregate: &ethpb.SyncAggregate{
				SyncCommitteeBits: bitfield.Bitvector512{
					0x31, 0xff, 0xff, 0xff, 0xff, 0x3f, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				},
			},
		},
	}

	wrappedBlock, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)

	s.processSyncAggregate(beaconState, wrappedBlock)
	require.LogsContain(t, hook, "\"Sync committee contribution included\" balanceChange=0 contribCount=1 expectedContribCount=4 newBalance=32000000000 prefix=monitor validatorIndex=1")
	require.LogsContain(t, hook, "\"Sync committee contribution included\" balanceChange=100000000 contribCount=2 expectedContribCount=2 newBalance=32000000000 prefix=monitor validatorIndex=12")
	require.LogsDoNotContain(t, hook, "validatorIndex=2")
}
