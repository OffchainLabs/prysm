// Package evaluators defines functions which can peer into end to end
// tests to determine if a chain is running as required.
package evaluators

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

// AllNodesHaveSameHead ensures all nodes have the same head epoch. Checks finality and justification as well.
// Not checking head block root as it may change irregularly for the validator connected nodes.
var AllNodesHaveSameHead = e2etypes.Evaluator{
	Name:       "all_nodes_have_same_head_%d",
	Policy:     policies.AllEpochs,
	Evaluation: allNodesHaveSameHead,
}

func healthzCheck(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	count := len(conns)
	for i := range count {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort+i))
		if err != nil {
			// Continue if the connection fails, regular flake.
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("expected status code OK for beacon node %d, received %v with body %s", i, resp.StatusCode, body)
		}
		if err = resp.Body.Close(); err != nil {
			return err
		}
		time.Sleep(connTimeDelay)
	}

	for i := range count {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.ValidatorMetricsPort+i))
		if err != nil {
			// Continue if the connection fails, regular flake.
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("expected status code OK for validator client %d, received %v with body %s", i, resp.StatusCode, body)
		}
		if err = resp.Body.Close(); err != nil {
			return err
		}
		time.Sleep(connTimeDelay)
	}
	return nil
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
func waitForMidEpoch(conn *grpc.ClientConn) error {
	beaconClient := eth.NewBeaconChainClient(conn)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	midEpochSlot := slotsPerEpoch / 2

	for {
		chainHead, err := beaconClient.GetChainHead(context.Background(), &emptypb.Empty{})
		if err != nil {
			return err
		}
		slotInEpoch := chainHead.HeadSlot % slotsPerEpoch
		// If we're at least halfway into the epoch, we're safe
		if slotInEpoch >= midEpochSlot {
			// Wait 3/4 into the slot to ensure block propagation
			time.Sleep(time.Duration(secondsPerSlot) * time.Second * 3 / 4)
			return nil
		}
		// Wait for the remaining slots until mid-epoch
		slotsToWait := midEpochSlot - slotInEpoch
		time.Sleep(time.Duration(slotsToWait) * time.Duration(secondsPerSlot) * time.Second)
	}
}

// getHeadEpochs fetches the head epoch from all beacon nodes concurrently.
func getHeadEpochs(conns []*grpc.ClientConn) ([]primitives.Epoch, error) {
	epochs := make([]primitives.Epoch, len(conns))
	g, _ := errgroup.WithContext(context.Background())

	for i, conn := range conns {
		conIdx := i
		currConn := conn
		g.Go(func() error {
			beaconClient := eth.NewBeaconChainClient(currConn)
			chainHead, err := beaconClient.GetChainHead(context.Background(), &emptypb.Empty{})
			if err != nil {
				return errors.Wrapf(err, "connection number=%d", conIdx)
			}
			epochs[conIdx] = chainHead.HeadEpoch
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return epochs, nil
}

func allNodesHaveSameHead(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	// Wait until we're at least halfway into the epoch to avoid race conditions
	// at epoch boundaries where nodes may report different epochs.
	if err := waitForMidEpoch(conns[0]); err != nil {
		return errors.Wrap(err, "failed waiting for mid-epoch")
	}

	// First, wait for all nodes to reach the same epoch. Sync nodes may be
	// behind and need time to catch up. We poll every 2 seconds with a
	// 60 second timeout - this adapts to actual sync progress rather than
	// using fixed delays.
	const epochTimeout = 60 * time.Second
	const epochPollInterval = 2 * time.Second
	epochDeadline := time.Now().Add(epochTimeout)

	for time.Now().Before(epochDeadline) {
		epochs, err := getHeadEpochs(conns)
		if err != nil {
			return err
		}
		allSame := true
		for i := 1; i < len(epochs); i++ {
			if epochs[0] != epochs[i] {
				allSame = false
				break
			}
		}
		if allSame {
			break
		}
		time.Sleep(epochPollInterval)
	}

	// Now that epochs match (or timeout reached), do detailed head comparison
	// with a few retries to handle block propagation delays.
	const maxRetries = 5
	const retryDelay = 1 * time.Second
	var lastErr error

	for attempt := range maxRetries {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		headEpochs := make([]primitives.Epoch, len(conns))
		headBlockRoots := make([][]byte, len(conns))
		justifiedRoots := make([][]byte, len(conns))
		prevJustifiedRoots := make([][]byte, len(conns))
		finalizedRoots := make([][]byte, len(conns))
		chainHeads := make([]*eth.ChainHead, len(conns))
		g, _ := errgroup.WithContext(context.Background())

		for i, conn := range conns {
			conIdx := i
			currConn := conn
			g.Go(func() error {
				beaconClient := eth.NewBeaconChainClient(currConn)
				chainHead, err := beaconClient.GetChainHead(context.Background(), &emptypb.Empty{})
				if err != nil {
					return errors.Wrapf(err, "connection number=%d", conIdx)
				}
				headEpochs[conIdx] = chainHead.HeadEpoch
				headBlockRoots[conIdx] = chainHead.HeadBlockRoot
				justifiedRoots[conIdx] = chainHead.JustifiedBlockRoot
				prevJustifiedRoots[conIdx] = chainHead.PreviousJustifiedBlockRoot
				finalizedRoots[conIdx] = chainHead.FinalizedBlockRoot
				chainHeads[conIdx] = chainHead
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}

		lastErr = nil
		for i := range conns {
			if headEpochs[0] != headEpochs[i] {
				lastErr = fmt.Errorf(
					"received conflicting head epochs on node %d, expected %d, received %d",
					i,
					headEpochs[0],
					headEpochs[i],
				)
				break
			}
			if !bytes.Equal(headBlockRoots[0], headBlockRoots[i]) {
				lastErr = fmt.Errorf(
					"received conflicting head block roots on node %d, expected %#x, received %#x",
					i,
					headBlockRoots[0],
					headBlockRoots[i],
				)
				break
			}
			if !bytes.Equal(justifiedRoots[0], justifiedRoots[i]) {
				lastErr = fmt.Errorf(
					"received conflicting justified block roots on node %d, expected %#x, received %#x: %s and %s",
					i,
					justifiedRoots[0],
					justifiedRoots[i],
					chainHeads[0].String(),
					chainHeads[i].String(),
				)
				break
			}
			if !bytes.Equal(prevJustifiedRoots[0], prevJustifiedRoots[i]) {
				lastErr = fmt.Errorf(
					"received conflicting previous justified block roots on node %d, expected %#x, received %#x",
					i,
					prevJustifiedRoots[0],
					prevJustifiedRoots[i],
				)
				break
			}
			if !bytes.Equal(finalizedRoots[0], finalizedRoots[i]) {
				lastErr = fmt.Errorf(
					"received conflicting finalized epoch roots on node %d, expected %#x, received %#x",
					i,
					finalizedRoots[0],
					finalizedRoots[i],
				)
				break
			}
		}

		if lastErr == nil {
			return nil
		}
	}

	return lastErr
}
