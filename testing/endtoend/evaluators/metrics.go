package evaluators

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/genesis"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const maxMemStatsBytes = 2000000000 // 2 GiB.

// MetricsCheck performs a check on metrics to make sure caches are functioning, and
// overall health is good. Not checking the first epoch so the sample size isn't too small.
var MetricsCheck = types.Evaluator{
	Name:       "metrics_check_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: metricsTest,
}

type equalityTest struct {
	name  string
	topic string
	value int
}

type comparisonTest struct {
	name               string
	topic1             string
	topic2             string
	expectedComparison float64
}

var metricLessThanTests = []equalityTest{
	{
		name:  "memory usage",
		topic: "go_memstats_alloc_bytes",
		value: maxMemStatsBytes,
	},
}

const (
	p2pFailValidationTopic = "p2p_message_failed_validation_total{topic=\"%s/ssz_snappy\"}"
	p2pReceivedTotalTopic  = "p2p_message_received_total{topic=\"%s/ssz_snappy\"}"
)

var metricComparisonTests = []comparisonTest{
	{
		name:               "beacon aggregate and proof",
		topic1:             fmt.Sprintf(p2pFailValidationTopic, p2p.AggregateAndProofSubnetTopicFormat),
		topic2:             fmt.Sprintf(p2pReceivedTotalTopic, p2p.AggregateAndProofSubnetTopicFormat),
		expectedComparison: 0.8,
	},
	{
		name:               "committee index beacon attestations",
		topic1:             fmt.Sprintf(p2pFailValidationTopic, formatTopic(p2p.AttestationSubnetTopicFormat)),
		topic2:             fmt.Sprintf(p2pReceivedTotalTopic, formatTopic(p2p.AttestationSubnetTopicFormat)),
		expectedComparison: 0.15,
	},
	{
		name:               "committee cache",
		topic1:             "committee_cache_miss",
		topic2:             "committee_cache_hit",
		expectedComparison: 0.01,
	},
	{
		name:               "hot state cache",
		topic1:             "hot_state_cache_miss",
		topic2:             "hot_state_cache_hit",
		expectedComparison: 0.01,
	},
}

func metricsTest(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	currentSlot := slots.CurrentSlot(genesis.Time())
	currentEpoch := slots.ToEpoch(currentSlot)
	forkDigest := params.ForkDigest(currentEpoch)

	if err := checkHeadSlots(conns...); err != nil {
		return err
	}

	for i := range conns {
		response, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort+i))
		if err != nil {
			// Continue if the connection fails, regular flake.
			continue
		}
		dataInBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		pageContent := string(dataInBytes)
		if err = response.Body.Close(); err != nil {
			return err
		}
		time.Sleep(connTimeDelay)

		for _, test := range metricLessThanTests {
			topic := test.topic
			if strings.Contains(topic, "%x") {
				topic = fmt.Sprintf(topic, forkDigest)
			}
			if err = metricCheckLessThan(pageContent, topic, test.value); err != nil {
				return errors.Wrapf(err, "failed %s check", test.name)
			}
		}
		for _, test := range metricComparisonTests {
			topic1 := test.topic1
			if strings.Contains(topic1, "%x") {
				topic1 = fmt.Sprintf(topic1, forkDigest)
			}
			topic2 := test.topic2
			if strings.Contains(topic2, "%x") {
				topic2 = fmt.Sprintf(topic2, forkDigest)
			}
			if err = metricCheckComparison(pageContent, topic1, topic2, test.expectedComparison); err != nil {
				return err
			}
		}
	}
	return nil
}

// headSlotCheckTimeout bounds how long checkHeadSlots keeps polling before it gives up:
// one slot, so a block that is still propagating when the evaluator starts can arrive.
func headSlotCheckTimeout() time.Duration {
	return params.BeaconConfig().SlotDuration()
}

// skippedSlotTolerance is how far a node's head may trail the wall-clock slot when no node
// has a block for the missing slots, i.e. they were skipped network-wide (for example
// proposals missed under load) rather than missed by that node. A quarter of an epoch
// keeps a stalled chain failing the check.
func skippedSlotTolerance() primitives.Slot {
	return params.BeaconConfig().SlotsPerEpoch / 4
}

// checkHeadSlots verifies that every node's chain head is keeping up with the wall clock.
func checkHeadSlots(conns ...*grpc.ClientConn) error {
	if len(conns) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), headSlotCheckTimeout())
	defer cancel()

	genesisResp, err := eth.NewNodeClient(conns[0]).GetGenesis(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	clients := make([]eth.BeaconChainClient, len(conns))
	for i, conn := range conns {
		clients[i] = eth.NewBeaconChainClient(conn)
	}
	return waitForHeadsNearClock(ctx, genesisResp.GenesisTime.AsTime(), skippedSlotTolerance(), clients...)
}

// waitForHeadsNearClock polls every node's chain head until compareHeadSlots accepts all
// of them, or ctx expires, in which case the last comparison error is returned.
func waitForHeadsNearClock(ctx context.Context, genesisTime time.Time, tolerance primitives.Slot, clients ...eth.BeaconChainClient) error {
	ticker := time.NewTicker(connTimeDelay)
	defer ticker.Stop()

	var lastErr error
	for {
		heads := make([]primitives.Slot, len(clients))
		g, gctx := errgroup.WithContext(ctx)
		for i, client := range clients {
			g.Go(func() error {
				chainHead, err := client.GetChainHead(gctx, &emptypb.Empty{})
				if err != nil {
					return errors.Wrapf(err, "connection number=%d", i)
				}
				heads[i] = chainHead.HeadSlot
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			if ctx.Err() != nil && lastErr != nil {
				return lastErr
			}
			return err
		}
		// Read the clock after the heads: a slot boundary crossed while fetching can only
		// make the clock later than the heads, which the one-slot allowance covers.
		if lastErr = compareHeadSlots(heads, slots.CurrentSlot(genesisTime), tolerance); lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return lastErr
		case <-ticker.C:
		}
	}
}

// compareHeadSlots checks each head slot against the wall-clock slot. A head at the clock
// slot, or one slot behind it (the current slot's block may still be on its way), always
// passes. A head that trails by more is accepted only when it is at, or within one slot
// of, the highest head across all nodes and the shortfall is within tolerance: no node
// has a block for the missing slots, so they were skipped, not missed by this node.
// Anything else is a node lagging the chain, or a chain that has stalled.
func compareHeadSlots(heads []primitives.Slot, timeSlot, tolerance primitives.Slot) error {
	var highest primitives.Slot
	for _, head := range heads {
		highest = max(highest, head)
	}
	for i, head := range heads {
		if head > timeSlot {
			return fmt.Errorf("node %d head slot %d is ahead of wall-clock slot %d", i, head, timeSlot)
		}
		shortfall := timeSlot - head
		if shortfall <= 1 {
			continue
		}
		if highest-head <= 1 && shortfall <= tolerance {
			continue
		}
		return fmt.Errorf(
			"node %d head slot %d trails wall-clock slot %d by %d slots (highest head across nodes %d, skipped-slot tolerance %d)",
			i, head, timeSlot, shortfall, highest, tolerance,
		)
	}
	return nil
}

func metricCheckLessThan(pageContent, topic string, value int) error {
	topicValue, err := valueOfTopic(pageContent, topic)
	if err != nil {
		return err
	}
	if topicValue >= value {
		return fmt.Errorf(
			"unexpected result for metric %s, expected less than %d, received %d",
			topic,
			value,
			topicValue,
		)
	}
	return nil
}

func metricCheckComparison(pageContent, topic1, topic2 string, comparison float64) error {
	topic2Value, err := valueOfTopic(pageContent, topic2)
	// If we can't find the first topic (error metrics), then assume the test passes.
	if topic2Value != -1 {
		return nil
	}
	if err != nil {
		return err
	}
	topic1Value, err := valueOfTopic(pageContent, topic1)
	if topic1Value != -1 {
		return nil
	}
	if err != nil {
		return err
	}
	topicComparison := float64(topic1Value) / float64(topic2Value)
	if topicComparison >= comparison {
		return fmt.Errorf(
			"unexpected result for comparison between metric %s and metric %s, expected comparison to be %.2f, received %.2f",
			topic1,
			topic2,
			comparison,
			topicComparison,
		)
	}
	return nil
}

func valueOfTopic(pageContent, topic string) (int, error) {
	regexExp, err := regexp.Compile(topic + " ")
	if err != nil {
		return -1, errors.Wrap(err, "could not create regex expression")
	}
	indexesFound := regexExp.FindAllStringIndex(pageContent, 8)
	if indexesFound == nil {
		return -1, fmt.Errorf("no strings found for %s", topic)
	}
	var result float64
	for i, stringIndex := range indexesFound {
		// Only performing every third result found since there are 2 comments above every metric.
		if i == 0 || i%2 != 0 {
			continue
		}
		startOfValue := stringIndex[1]
		endOfValue := strings.Index(pageContent[startOfValue:], "\n")
		if endOfValue == -1 {
			return -1, fmt.Errorf("could not find next space in %s", pageContent[startOfValue:])
		}
		metricValue := pageContent[startOfValue : startOfValue+endOfValue]
		floatResult, err := strconv.ParseFloat(metricValue, 64)
		if err != nil {
			return -1, errors.Wrapf(err, "could not parse %s for int", metricValue)
		}
		result += floatResult
	}
	return int(result), nil
}

func formatTopic(topic string) string {
	replacedD := strings.Replace(topic, "%d", "\\w*", 1)
	return replacedD
}
