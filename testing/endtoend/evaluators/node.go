// Package evaluators defines functions which can peer into end to end
// tests to determine if a chain is running as required.
package evaluators

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/OffchainLabs/prysm/v7/config/params"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Allow a very short delay after disconnecting to prevent connection refused issues.
var connTimeDelay = 50 * time.Millisecond

const (
	httpCheckAttempts              = 5
	httpCheckRetryDelay            = 200 * time.Millisecond
	headComparePollDelay           = time.Second
	headCompareRequiredConsecutive = 2
	minPeersForHeadCheck           = 1
)

// PeersConnect checks all beacon nodes and returns whether they are connected to each other as peers.
var PeersConnect = e2etypes.Evaluator{
	Name:       "peers_connect_epoch_%d",
	Policy:     policies.OnEpoch(0),
	Evaluation: peersConnect,
}

// HealthzCheck pings healthz and errors if it doesn't have the expected OK status.
var HealthzCheck = e2etypes.Evaluator{
	Name:       "healthz_check_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: healthzCheck,
}

// FinishedSyncing returns whether the beacon node with the given rpc port has finished syncing.
var FinishedSyncing = e2etypes.Evaluator{
	Name:       "finished_syncing_%d",
	Policy:     policies.AllEpochs,
	Evaluation: finishedSyncing,
}

// AllNodesHaveSameHead ensures all nodes converge on the same canonical head:
// epoch, head block root, justified root, previous justified root, and finalized root.
// We intentionally check head block root (unlike older behavior) because only comparing
// epochs can hide real divergence where nodes are on different blocks in the same slot/epoch.
// To avoid reintroducing flake, the evaluator now waits for readiness and requires
// convergence across consecutive samples before passing.
var AllNodesHaveSameHead = e2etypes.Evaluator{
	Name:       "all_nodes_have_same_head_%d",
	Policy:     policies.AllEpochs,
	Evaluation: allNodesHaveSameHead,
}

func healthzCheck(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	count := len(conns)
	for i := range count {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := getURLBodyWithRetries(ctx, fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort+i), httpCheckAttempts, httpCheckRetryDelay)
		cancel()
		if err != nil {
			return fmt.Errorf("healthz check failed for beacon node %d: %w", i, err)
		}
		time.Sleep(connTimeDelay)
	}

	for i := range count {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := getURLBodyWithRetries(ctx, fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.ValidatorMetricsPort+i), httpCheckAttempts, httpCheckRetryDelay)
		cancel()
		if err != nil {
			return fmt.Errorf("healthz check failed for validator client %d: %w", i, err)
		}
		time.Sleep(connTimeDelay)
	}
	return nil
}

func getURLBodyWithRetries(ctx context.Context, url string, attempts int, retryDelay time.Duration) ([]byte, error) {
	var lastErr error
	for attempt := range attempts {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(resp.Body)
			closeErr := resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if closeErr != nil {
				lastErr = closeErr
			} else if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("status code=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
			} else {
				return body, nil
			}
		}

		if attempt < attempts-1 {
			err := sleepWithContext(ctx, retryDelay)
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("request to %s failed after %d attempts: %w", url, attempts, lastErr)
}

func peersConnect(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	if len(conns) == 1 {
		return nil
	}
	ctx := context.Background()
	for _, conn := range conns {
		nodeClient := eth.NewNodeClient(conn)
		peersResp, err := nodeClient.ListPeers(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		expectedPeers := len(conns) - 1 + e2e.TestParams.LighthouseBeaconNodeCount
		if expectedPeers != len(peersResp.Peers) {
			return fmt.Errorf("unexpected amount of peers, expected %d, received %d", expectedPeers, len(peersResp.Peers))
		}
		time.Sleep(connTimeDelay)
	}
	return nil
}

func finishedSyncing(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	syncNodeClient := eth.NewNodeClient(conn)
	syncStatus, err := syncNodeClient.GetSyncStatus(context.Background(), &emptypb.Empty{})
	if err != nil {
		return err
	}
	if syncStatus.Syncing {
		return errors.New("expected node to have completed sync")
	}
	return nil
}

// waitForMidEpoch waits until we're at least halfway into the current epoch
// and 3/4 into the current slot. This prevents race conditions at epoch
// boundaries and slot boundaries where different nodes may report different heads.
func waitForMidEpoch(ctx context.Context, conn *grpc.ClientConn) error {
	beaconClient := eth.NewBeaconChainClient(conn)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	midEpochSlot := slotsPerEpoch / 2

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		chainHead, err := beaconClient.GetChainHead(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		slotInEpoch := chainHead.HeadSlot % slotsPerEpoch
		// If we're at least halfway into the epoch, we're safe
		if slotInEpoch >= midEpochSlot {
			// Wait 3/4 into the slot to ensure block propagation
			if err := sleepWithContext(ctx, time.Duration(secondsPerSlot)*time.Second*3/4); err != nil {
				return err
			}
			return nil
		}
		// Wait for the remaining slots until mid-epoch
		slotsToWait := midEpochSlot - slotInEpoch
		if err := sleepWithContext(ctx, time.Duration(slotsToWait)*time.Duration(secondsPerSlot)*time.Second); err != nil {
			return err
		}
	}
}

func allNodesHaveSameHead(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), params.EpochsDuration(2, params.BeaconConfig()))
	defer cancel()
	// Wait until we're at least halfway into the epoch to avoid race conditions
	// at epoch boundaries where nodes may report different epochs.
	if err := waitForAllMidEpoch(ctx, conns...); err != nil {
		return errors.Wrap(err, "failed waiting for mid-epoch")
	}

	consecutiveSuccesses := 0
	var lastErr error
	var lastHeads []*eth.ChainHead
	var lastPeers []int
	attempt := 0
	for {
		attempt++
		if ctx.Err() != nil {
			break
		}

		chainHeads, err := fetchAllChainHeads(ctx, conns)
		if err != nil {
			lastErr = errors.Wrap(err, "fetch chain heads")
			consecutiveSuccesses = 0
			if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
				break
			}
			continue
		}
		lastHeads = chainHeads

		peerCounts, err := fetchPeerCounts(ctx, conns)
		if err != nil {
			lastErr = errors.Wrap(err, "fetch peer counts")
			consecutiveSuccesses = 0
			if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
				break
			}
			continue
		}
		lastPeers = peerCounts

		if !allPeerCountsAtLeast(peerCounts, minPeersForHeadCheck) {
			lastErr = fmt.Errorf("nodes not ready: insufficient peers (min=%d)\n%s", minPeersForHeadCheck, summarizeHeads(chainHeads, peerCounts))
			consecutiveSuccesses = 0
			log.WithField("attempt", attempt).Warn("Insufficient peers for stable head comparison, retrying")
			if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
				break
			}
			continue
		}

		if anyOptimistic(chainHeads) {
			lastErr = fmt.Errorf("nodes not ready: optimistic head(s) observed\n%s", summarizeHeads(chainHeads, peerCounts))
			consecutiveSuccesses = 0
			log.WithField("attempt", attempt).Warn("Optimistic head status observed, retrying")
			if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
				break
			}
			continue
		}

		if err := compareChainHeads(chainHeads); err != nil {
			lastErr = fmt.Errorf("%w\n%s", err, summarizeHeads(chainHeads, peerCounts))
			consecutiveSuccesses = 0
			log.WithField("attempt", attempt).Warn("Chain head mismatch, retrying")
			if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
				break
			}
			continue
		}

		consecutiveSuccesses++
		if consecutiveSuccesses >= headCompareRequiredConsecutive {
			return nil
		}
		if err := sleepWithContext(ctx, headComparePollDelay); err != nil {
			break
		}
	}

	if lastErr == nil {
		lastErr = errors.New("head comparison did not converge before timeout")
	}
	if len(lastHeads) > 0 {
		return fmt.Errorf("head comparison failed: %w\n%s", lastErr, summarizeHeads(lastHeads, lastPeers))
	}
	return fmt.Errorf("head comparison failed: %w", lastErr)
}

// fetchAllChainHeads queries all beacon nodes for their chain head in parallel.
func fetchAllChainHeads(ctx context.Context, conns []*grpc.ClientConn) ([]*eth.ChainHead, error) {
	chainHeads := make([]*eth.ChainHead, len(conns))
	g, gctx := errgroup.WithContext(ctx)
	for i, conn := range conns {
		conIdx := i
		currConn := conn
		g.Go(func() error {
			beaconClient := eth.NewBeaconChainClient(currConn)
			chainHead, err := beaconClient.GetChainHead(gctx, &emptypb.Empty{})
			if err != nil {
				return errors.Wrapf(err, "connection number=%d", conIdx)
			}
			chainHeads[conIdx] = chainHead
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return chainHeads, nil
}

func fetchPeerCounts(ctx context.Context, conns []*grpc.ClientConn) ([]int, error) {
	peerCounts := make([]int, len(conns))
	g, gctx := errgroup.WithContext(ctx)
	for i, conn := range conns {
		conIdx := i
		currConn := conn
		g.Go(func() error {
			nodeClient := eth.NewNodeClient(currConn)
			peersResp, err := nodeClient.ListPeers(gctx, &emptypb.Empty{})
			if err != nil {
				return errors.Wrapf(err, "failed to list peers for connection number=%d", conIdx)
			}
			peerCounts[conIdx] = len(peersResp.Peers)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return peerCounts, nil
}

func allPeerCountsAtLeast(peerCounts []int, minPeers int) bool {
	for _, count := range peerCounts {
		if count < minPeers {
			return false
		}
	}
	return true
}

func anyOptimistic(chainHeads []*eth.ChainHead) bool {
	for _, head := range chainHeads {
		if head.GetOptimisticStatus() {
			return true
		}
	}
	return false
}

func summarizeHeads(chainHeads []*eth.ChainHead, peerCounts []int) string {
	var b strings.Builder
	for i, head := range chainHeads {
		peers := -1
		if i < len(peerCounts) {
			peers = peerCounts[i]
		}
		_, _ = fmt.Fprintf(
			&b,
			"node=%d slot=%d epoch=%d optimistic=%t peers=%d head=%#x justified=%#x finalized=%#x\n",
			i,
			head.HeadSlot,
			head.HeadEpoch,
			head.GetOptimisticStatus(),
			peers,
			head.HeadBlockRoot,
			head.JustifiedBlockRoot,
			head.FinalizedBlockRoot,
		)
	}
	return strings.TrimSpace(b.String())
}

// compareChainHeads checks that all chain heads agree on epoch, head root,
// justified root, previous justified root, and finalized root.
func compareChainHeads(chainHeads []*eth.ChainHead) error {
	for i := 1; i < len(chainHeads); i++ {
		if chainHeads[0].HeadEpoch != chainHeads[i].HeadEpoch {
			return fmt.Errorf(
				"received conflicting head epochs on node %d, expected %d, received %d",
				i,
				chainHeads[0].HeadEpoch,
				chainHeads[i].HeadEpoch,
			)
		}
		if !bytes.Equal(chainHeads[0].HeadBlockRoot, chainHeads[i].HeadBlockRoot) {
			return fmt.Errorf(
				"received conflicting head block roots on node %d (slot %d vs %d), expected %#x, received %#x",
				i,
				chainHeads[0].HeadSlot,
				chainHeads[i].HeadSlot,
				chainHeads[0].HeadBlockRoot,
				chainHeads[i].HeadBlockRoot,
			)
		}
		if !bytes.Equal(chainHeads[0].JustifiedBlockRoot, chainHeads[i].JustifiedBlockRoot) {
			return fmt.Errorf(
				"received conflicting justified block roots on node %d, expected %#x, received %#x: %s and %s",
				i,
				chainHeads[0].JustifiedBlockRoot,
				chainHeads[i].JustifiedBlockRoot,
				chainHeads[0].String(),
				chainHeads[i].String(),
			)
		}
		if !bytes.Equal(chainHeads[0].PreviousJustifiedBlockRoot, chainHeads[i].PreviousJustifiedBlockRoot) {
			return fmt.Errorf(
				"received conflicting previous justified block roots on node %d, expected %#x, received %#x",
				i,
				chainHeads[0].PreviousJustifiedBlockRoot,
				chainHeads[i].PreviousJustifiedBlockRoot,
			)
		}
		if !bytes.Equal(chainHeads[0].FinalizedBlockRoot, chainHeads[i].FinalizedBlockRoot) {
			return fmt.Errorf(
				"received conflicting finalized epoch roots on node %d, expected %#x, received %#x",
				i,
				chainHeads[0].FinalizedBlockRoot,
				chainHeads[i].FinalizedBlockRoot,
			)
		}
	}
	return nil
}

func waitForAllMidEpoch(ctx context.Context, conns ...*grpc.ClientConn) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, conn := range conns {
		currConn := conn
		g.Go(func() error {
			return waitForMidEpoch(gctx, currConn)
		})
	}
	return g.Wait()
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
