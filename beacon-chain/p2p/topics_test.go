package p2p

import (
	"encoding/hex"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestAllTopics(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	params.OverrideBeaconConfig(cfg)
	s := &Service{}
	all := s.allTopicStrings()
	tops := map[string]struct{}{}
	for _, t := range all {
		tops[t] = struct{}{}
	}
	require.Equal(t, len(tops), len(all), "duplicate topics found")
	expected := []string{
		"/eth2/ad532ceb/sync_committee_contribution_and_proof/ssz_snappy",
		"/eth2/ad532ceb/beacon_aggregate_and_proof/ssz_snappy",
		"/eth2/ad532ceb/beacon_block/ssz_snappy",
		"/eth2/ad532ceb/bls_to_execution_change/ssz_snappy",
		"/eth2/afcaaba0/beacon_attestation_19/ssz_snappy",
		"/eth2/cc2c5cdb/data_column_sidecar_0/ssz_snappy",
		"/eth2/cc2c5cdb/data_column_sidecar_127/ssz_snappy",
	}
	forks := []primitives.Epoch{cfg.GenesisEpoch, cfg.AltairForkEpoch,
		cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch, cfg.DenebForkEpoch,
		cfg.ElectraForkEpoch, cfg.FuluForkEpoch}
	// sanity check: we should always have a block topic.
	// construct it by hand in case there are bugs in newTopic.
	for _, f := range forks {
		digest := params.ForkDigest(f)
		expected = append(expected, "/eth2/"+hex.EncodeToString(digest[:])+"/beacon_block/ssz_snappy")
	}
	for _, e := range expected {
		_, ok := tops[e]
		require.Equal(t, true, ok)
	}
	// we should have no data column subnets before fulu
	electraColumn := newSubnetTopic(cfg.ElectraForkEpoch, cfg.FuluForkEpoch,
		params.ForkDigest(params.BeaconConfig().ElectraForkEpoch),
		GossipDataColumnSidecarMessage,
		cfg.DataColumnSidecarSubnetCount-1)
	// we should have no blob sidecars before deneb or after electra
	blobBeforeDeneb := newSubnetTopic(cfg.DenebForkEpoch-1, cfg.DenebForkEpoch,
		params.ForkDigest(cfg.DenebForkEpoch-1),
		GossipBlobSidecarMessage,
		uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.DenebForkEpoch-1))-1)
	blobAfterElectra := newSubnetTopic(cfg.FuluForkEpoch, cfg.FarFutureEpoch,
		params.ForkDigest(cfg.FuluForkEpoch),
		GossipBlobSidecarMessage,
		uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch))-1)
	unexpected := []string{
		"/eth2/cc2c5cdb/data_column_sidecar_128/ssz_snappy",
		electraColumn.String(),
		blobBeforeDeneb.String(),
		blobAfterElectra.String(),
	}
	for _, e := range unexpected {
		_, ok := tops[e]
		require.Equal(t, false, ok)
	}
}
