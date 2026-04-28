package sync

import (
	"context"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestUpdateMetrics_DataColumnTopicLabelFormatted verifies that updateMetrics
// emits properly formatted topic labels for the data column sidecar subnets.
// Prior to the fix, the generic topic loop fed DataColumnSubnetTopicFormat
// (which has both %x and %d verbs) into fmt.Sprintf with only the digest
// argument, producing the literal "%!d(MISSING)" placeholder in the
// p2p_topic_peer_count Prometheus label.
func TestUpdateMetrics_DataColumnTopicLabelFormatted(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)

	topicPeerCount.Reset()

	closedChan := make(chan struct{})
	close(closedChan)
	p2p := p2ptest.NewTestP2P(t)
	chain := &mockChain.ChainService{
		Genesis:        genesis.Time(),
		ValidatorsRoot: genesis.ValidatorsRoot(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			p2p:         p2p,
			chain:       chain,
			clock:       defaultClockWithTimeAtEpoch(0),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		initialSyncComplete: closedChan,
	}

	s.updateMetrics()

	ch := make(chan prometheus.Metric, 4096)
	topicPeerCount.Collect(ch)
	close(ch)

	var sawDataColumn bool
	for m := range ch {
		var pm dto.Metric
		require.NoError(t, m.Write(&pm))
		for _, lp := range pm.Label {
			if lp.GetName() != "topic" {
				continue
			}
			v := lp.GetValue()
			require.Equal(t, false, strings.Contains(v, "MISSING"),
				"topic label contains Sprintf error placeholder: %s", v)
			require.Equal(t, false, strings.Contains(v, "%!"),
				"topic label contains Sprintf error placeholder: %s", v)
			if strings.Contains(v, "data_column_sidecar_") {
				sawDataColumn = true
			}
		}
	}
	require.Equal(t, true, sawDataColumn,
		"expected updateMetrics to emit at least one data_column_sidecar topic label")
}
