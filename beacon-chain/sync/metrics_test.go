package sync

import (
	"fmt"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestUpdateMetrics_TopicLabelsFullyFormatted(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)

	s := testForkWatcherService(t, 0)
	digest := params.ForkDigest(0)

	topicPeerCount.Reset()
	s.updateMetrics()

	ch := make(chan prometheus.Metric, 4096)
	topicPeerCount.Collect(ch)
	close(ch)

	labels := make(map[string]bool)
	for m := range ch {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		for _, l := range pb.GetLabel() {
			require.Equal(t, false, strings.Contains(l.GetValue(), "%!"),
				"topic label has an unfilled format verb: "+l.GetValue())
			labels[l.GetValue()] = true
		}
	}

	suffix := s.cfg.p2p.Encoding().ProtocolSuffix()
	for _, i := range []uint64{0, params.BeaconConfig().DataColumnSidecarSubnetCount - 1} {
		want := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, i) + suffix
		assert.Equal(t, true, labels[want], "missing metric for data column subnet topic "+want)
	}
}
