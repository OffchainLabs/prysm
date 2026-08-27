package peers_test

import (
	"crypto/rand"
	mrand "math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/consensus-types/wrapper"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/eth/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestStatus(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})
	require.NotNil(t, p, "p not created")
}

func TestPeerExplicitAdd(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err, "Failed to create ID")
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	p.Add(new(enr.Record), id, address, direction)

	resAddress, err := p.Address(id)
	require.NoError(t, err)
	assert.Equal(t, true, address.Equal(resAddress), "Unexpected address")

	resDirection, err := p.Direction(id)
	require.NoError(t, err)
	assert.Equal(t, direction, resDirection, "Unexpected direction")

	// Update with another explicit add
	address2, err := ma.NewMultiaddr("/ip4/52.23.23.253/tcp/30000/ipfs/QmfAgkmjiZNZhr2wFN9TwaRgHouMTBT6HELyzE5A3BT2wK/p2p-circuit")
	require.NoError(t, err)
	direction2 := network.DirOutbound
	p.Add(new(enr.Record), id, address2, direction2)

	resAddress2, err := p.Address(id)
	require.NoError(t, err)
	assert.Equal(t, true, address2.Equal(resAddress2), "Unexpected address")

	resDirection2, err := p.Direction(id)
	require.NoError(t, err)
	assert.Equal(t, direction2, resDirection2, "Unexpected direction")
}

func TestPeerNoENR(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err, "Failed to create ID")
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	p.Add(nil, id, address, direction)

	retrievedENR, err := p.ENR(id)
	require.NoError(t, err, "Could not retrieve chainstate")
	var nilENR *enr.Record
	assert.Equal(t, nilENR, retrievedENR, "Wanted a nil enr to be saved")
}

func TestPeerNoOverwriteENR(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err, "Failed to create ID")
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	record := new(enr.Record)
	record.Set(enr.WithEntry("test", []byte{'a'}))
	p.Add(record, id, address, direction)
	// try to overwrite
	p.Add(nil, id, address, direction)

	retrievedENR, err := p.ENR(id)
	require.NoError(t, err, "Could not retrieve chainstate")
	require.NotNil(t, retrievedENR, "Wanted a non-nil enr")
}

func TestErrUnknownPeer(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err)

	_, err = p.Address(id)
	assert.ErrorContains(t, peerdata.ErrPeerUnknown.Error(), err)

	_, err = p.Direction(id)
	assert.ErrorContains(t, peerdata.ErrPeerUnknown.Error(), err)

	_, err = scoring.PeerStatus(id)
	assert.ErrorContains(t, peerdata.ErrPeerUnknown.Error(), err)

	_, err = p.ConnectionState(id)
	assert.ErrorContains(t, peerdata.ErrPeerUnknown.Error(), err)

	assert.Equal(t, true, scoring.ChainStateLastUpdated(id).IsZero())

	assert.Equal(t, 0, scoring.BadResponseCount(id))
}

func TestPeerCommitteeIndices(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err, "Failed to create ID")
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	record := new(enr.Record)
	record.Set(enr.WithEntry("test", []byte{'a'}))
	p.Add(record, id, address, direction)
	bitV := bitfield.NewBitvector64()
	for i := range 64 {
		if i == 2 || i == 8 || i == 9 {
			bitV.SetBitAt(uint64(i), true)
		}
	}
	p.SetMetadata(id, wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: 2,
		Attnets:   bitV,
	}))

	wantedIndices := []uint64{2, 8, 9}

	indices, err := p.CommitteeIndices(id)
	require.NoError(t, err, "Could not retrieve committee indices")
	assert.DeepEqual(t, wantedIndices, indices)
}

func TestPeerSubscribedToSubnet(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	// Add some peers with different states
	numPeers := 2
	for i := 0; i < numPeers; i++ {
		addPeer(t, p, peers.Connected)
	}
	expectedPeer := p.All()[1]
	bitV := bitfield.NewBitvector64()
	for i := range 64 {
		if i == 2 || i == 8 || i == 9 {
			bitV.SetBitAt(uint64(i), true)
		}
	}
	p.SetMetadata(expectedPeer, wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: 2,
		Attnets:   bitV,
	}))
	numPeers = 3
	for i := 0; i < numPeers; i++ {
		addPeer(t, p, peers.Disconnected)
	}
	ps := p.SubscribedToSubnet(2)
	assert.Equal(t, 1, len(ps), "Unexpected num of peers")
	assert.Equal(t, expectedPeer, ps[0])

	ps = p.SubscribedToSubnet(8)
	assert.Equal(t, 1, len(ps), "Unexpected num of peers")
	assert.Equal(t, expectedPeer, ps[0])

	ps = p.SubscribedToSubnet(9)
	assert.Equal(t, 1, len(ps), "Unexpected num of peers")
	assert.Equal(t, expectedPeer, ps[0])
}

func TestPeerImplicitAdd(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err)

	connectionState := peers.Connecting
	p.SetConnectionState(id, connectionState)

	resConnectionState, err := p.ConnectionState(id)
	require.NoError(t, err)

	assert.Equal(t, connectionState, resConnectionState, "Unexpected connection state")
}

func TestPeerChainState(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err)
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	p.Add(new(enr.Record), id, address, direction)

	oldChainStartLastUpdated := scoring.ChainStateLastUpdated(id)

	finalizedEpoch := primitives.Epoch(123)
	setChainState(scoring, id, &pb.StatusV2{FinalizedEpoch: finalizedEpoch})

	resChainState, err := scoring.PeerStatus(id)
	require.NoError(t, err)
	assert.Equal(t, finalizedEpoch, resChainState.FinalizedEpoch, "Unexpected finalized epoch")

	newChainStartLastUpdated := scoring.ChainStateLastUpdated(id)
	if !newChainStartLastUpdated.After(oldChainStartLastUpdated) {
		t.Errorf("Last updated did not increase: old %v new %v", oldChainStartLastUpdated, newChainStartLastUpdated)
	}
}

func TestPeerWithNilChainState(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err)
	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	p.Add(new(enr.Record), id, address, direction)

	setChainState(scoring, id, nil)

	resChainState, err := scoring.PeerStatus(id)
	require.Equal(t, peerscoring.ErrNoPeerStatus, err)
	var nothing *pb.StatusV2
	require.Equal(t, resChainState, nothing)
}

func TestPeerBadResponses(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	id, err := peer.Decode("16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR")
	require.NoError(t, err)
	{
		_, err := id.MarshalBinary()
		require.NoError(t, err)
	}

	assert.NoError(t, scoring.IsPeerGreyListed(id), "Peer grey-listed when should be good")

	address, err := ma.NewMultiaddr("/ip4/213.202.254.180/tcp/13000")
	require.NoError(t, err, "Failed to create address")
	direction := network.DirInbound
	p.Add(new(enr.Record), id, address, direction)

	assert.Equal(t, 0, scoring.BadResponseCount(id), "Unexpected bad responses")
	assert.NoError(t, scoring.IsPeerGreyListed(id), "Peer grey-listed when should be good")

	scoring.RecordBadResponse(id, peerscoring.Unknown, "test")
	assert.Equal(t, 1, scoring.BadResponseCount(id), "Unexpected bad responses")
	assert.NoError(t, scoring.IsPeerGreyListed(id), "Peer grey-listed when should be good")

	scoring.RecordBadResponse(id, peerscoring.Unknown, "test")
	assert.Equal(t, 2, scoring.BadResponseCount(id), "Unexpected bad responses")
	assert.NotNil(t, scoring.IsPeerGreyListed(id), "Peer not grey-listed when it should be")

	scoring.RecordBadResponse(id, peerscoring.Unknown, "test")
	assert.Equal(t, 3, scoring.BadResponseCount(id), "Unexpected bad responses")
	assert.NotNil(t, scoring.IsPeerGreyListed(id), "Peer not grey-listed when it should be")
}

func TestAddMetaData(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	// Add some peers with different states
	numPeers := 5
	for range numPeers {
		addPeer(t, p, peers.Connected)
	}
	newPeer := p.All()[2]

	newMetaData := &pb.MetaDataV0{
		SeqNumber: 8,
		Attnets:   bitfield.NewBitvector64(),
	}
	p.SetMetadata(newPeer, wrapper.WrappedMetadataV0(newMetaData))

	md, err := p.Metadata(newPeer)
	require.NoError(t, err)
	assert.Equal(t, newMetaData.SeqNumber, md.SequenceNumber(), "Unexpected sequence number")
}

func TestPeerConnectionStatuses(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	// Add some peers with different states
	numPeersDisconnected := 11
	for range numPeersDisconnected {
		addPeer(t, p, peers.Disconnected)
	}
	numPeersConnecting := 7
	for range numPeersConnecting {
		addPeer(t, p, peers.Connecting)
	}
	numPeersConnected := 43
	for range numPeersConnected {
		addPeer(t, p, peers.Connected)
	}
	numPeersDisconnecting := 4
	for range numPeersDisconnecting {
		addPeer(t, p, peers.Disconnecting)
	}

	// Now confirm the states
	assert.Equal(t, numPeersDisconnected, len(p.Disconnected()), "Unexpected number of disconnected peers")
	assert.Equal(t, numPeersConnecting, len(p.Connecting()), "Unexpected number of connecting peers")
	assert.Equal(t, numPeersConnected, len(p.Connected()), "Unexpected number of connected peers")
	assert.Equal(t, numPeersDisconnecting, len(p.Disconnecting()), "Unexpected number of disconnecting peers")
	numPeersActive := numPeersConnecting + numPeersConnected
	assert.Equal(t, numPeersActive, len(p.Active()), "Unexpected number of active peers")
	numPeersInactive := numPeersDisconnecting + numPeersDisconnected
	assert.Equal(t, numPeersInactive, len(p.Inactive()), "Unexpected number of inactive peers")
	numPeersAll := numPeersActive + numPeersInactive
	assert.Equal(t, numPeersAll, len(p.All()), "Unexpected number of peers")
}

func TestPeerValidTime(t *testing.T) {
	maxBadResponses := 2
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses)),
	})

	numPeersConnected := 6
	for range numPeersConnected {
		addPeer(t, p, peers.Connected)
	}

	allPeers := p.All()

	// Add for 1st peer
	p.SetNextValidTime(allPeers[0], time.Now().Add(-1*time.Second))
	p.SetNextValidTime(allPeers[1], time.Now().Add(1*time.Second))
	p.SetNextValidTime(allPeers[2], time.Now().Add(10*time.Second))

	assert.Equal(t, true, p.IsReadyToDial(allPeers[0]))
	assert.Equal(t, false, p.IsReadyToDial(allPeers[1]))
	assert.Equal(t, false, p.IsReadyToDial(allPeers[2]))

	nextVal, err := p.NextValidTime(allPeers[3])
	require.NoError(t, err)
	assert.Equal(t, true, nextVal.IsZero())
	assert.Equal(t, true, p.IsReadyToDial(allPeers[3]))

	nextVal, err = p.NextValidTime(allPeers[4])
	require.NoError(t, err)
	assert.Equal(t, true, nextVal.IsZero())
	assert.Equal(t, true, p.IsReadyToDial(allPeers[4]))

	nextVal, err = p.NextValidTime(allPeers[5])
	require.NoError(t, err)
	assert.Equal(t, true, nextVal.IsZero())
	assert.Equal(t, true, p.IsReadyToDial(allPeers[5]))

	// Now confirm the states
	assert.Equal(t, numPeersConnected, len(p.Connected()), "Unexpected number of connected peers")
}

func TestPrune(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	for i := 0; i < p.MaxPeerLimit()+100; i++ {
		if i%7 == 0 {
			// Peer added as disconnected.
			_ = addPeer(t, p, peers.Disconnected)
		}
		// Peer added to peer handler.
		_ = addPeer(t, p, peers.Connected)
	}

	disPeers := p.Disconnected()
	firstPID := disPeers[0]
	secondPID := disPeers[1]
	thirdPID := disPeers[2]

	// Make first peer a grey-listed peer.
	scoring.RecordBadResponse(firstPID, peerscoring.Unknown, "test")
	scoring.RecordBadResponse(firstPID, peerscoring.Unknown, "test")

	// Add bad response for p2.
	scoring.RecordBadResponse(secondPID, peerscoring.Unknown, "test")

	// Prune peers
	prunedPIDs := p.Prune()

	// Pruned peers are returned to the caller (used to prune other per-peer state).
	assert.Equal(t, true, slices.Contains(prunedPIDs, secondPID), "Expected pruned peer to be returned")
	assert.Equal(t, false, slices.Contains(prunedPIDs, firstPID), "Grey-listed peer must not be pruned")

	// Grey-listed peer is expected to still be kept in handler.
	_, err := p.ConnectionState(firstPID)
	assert.NoError(t, err, "error is supposed to be  nil")
	assert.Equal(t, 2, scoring.BadResponseCount(firstPID), "Did not get expected amount")

	// Not so good peer is pruned away so that we can reduce the
	// total size of the handler.
	_, err = p.ConnectionState(secondPID)
	assert.ErrorContains(t, "peer unknown", err)

	// Last peer has been removed.
	_, err = p.ConnectionState(thirdPID)
	assert.ErrorContains(t, "peer unknown", err)

	// The scorer forgets pruned peers along with the store.
	assert.Equal(t, 0, scoring.BadResponseCount(secondPID), "Expected scorer to forget pruned peer")
}

func TestPeerIPTracker(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{})
	defer resetCfg()
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	badIP := "211.227.218.116"
	var badPeers []peer.ID
	for i := range peers.CollocationLimit + 10 {
		port := strconv.Itoa(3000 + i)
		addr, err := ma.NewMultiaddr("/ip4/" + badIP + "/tcp/" + port)
		if err != nil {
			t.Fatal(err)
		}
		badPeers = append(badPeers, createPeer(t, p, addr, network.DirUnknown, peerdata.ConnectionState(ethpb.ConnectionState_DISCONNECTED)))
	}
	for _, pr := range badPeers {
		assert.NotNil(t, p.IsFromBadIP(pr), "peer with bad ip is not bad")
	}

	// Fill the store past its cap so pruning kicks in.
	for i := 0; i < p.MaxPeerLimit()+100; i++ {
		pid := addPeer(t, p, peers.Disconnected)
		scoring.RecordBadResponse(pid, peerscoring.Unknown, "test")
	}
	pruned := p.Prune()
	require.NotEqual(t, 0, len(pruned))

	// Colocated-IP peers survive pruning: forgetting them would reset the IP's colocation budget.
	for _, pr := range badPeers {
		assert.Equal(t, false, slices.Contains(pruned, pr), "colocated peer must not be pruned")
		assert.NotNil(t, p.IsFromBadIP(pr), "colocated peer must still be refused after pruning")
	}
}

func TestTrimmedOrderedPeers(t *testing.T) {
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	const (
		expectedTarget = primitives.Epoch(2)
		maxPeers       = 3
	)

	var mockroot2 [32]byte
	var mockroot3 [32]byte
	var mockroot4 [32]byte
	var mockroot5 [32]byte
	copy(mockroot2[:], "two")
	copy(mockroot3[:], "three")
	copy(mockroot4[:], "four")
	copy(mockroot5[:], "five")

	// Peer 1
	pid1 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid1, &pb.StatusV2{
		HeadSlot:       3 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedEpoch: 3,
		FinalizedRoot:  mockroot3[:],
	})

	// Peer 2
	pid2 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid2, &pb.StatusV2{
		HeadSlot:       4 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedEpoch: 4,
		FinalizedRoot:  mockroot4[:],
	})

	// Peer 3
	pid3 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid3, &pb.StatusV2{
		HeadSlot:       5 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedEpoch: 5,
		FinalizedRoot:  mockroot5[:],
	})

	// Peer 4
	pid4 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid4, &pb.StatusV2{
		HeadSlot:       2 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedEpoch: 2,
		FinalizedRoot:  mockroot2[:],
	})

	// Peer 5
	pid5 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid5, &pb.StatusV2{
		HeadSlot:       2 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedEpoch: 2,
		FinalizedRoot:  mockroot2[:],
	})

	target, pids := p.BestFinalized(0)
	assert.Equal(t, expectedTarget, target, "Incorrect target epoch retrieved")
	// addPeer called 5 times above
	assert.Equal(t, 5, len(pids), "Incorrect number of peers retrieved")

	// Expect the returned list to be ordered by finalized epoch and trimmed to max peers.
	assert.Equal(t, pid3, pids[0], "Incorrect first peer")
	assert.Equal(t, pid2, pids[1], "Incorrect second peer")
	assert.Equal(t, pid1, pids[2], "Incorrect third peer")
}

func TestConcurrentPeerLimitHolds(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1)),
	})
	assert.Equal(t, true, uint64(p.MaxPeerLimit()) > p.ConnectedPeerLimit(), "max peer limit doesn't exceed connected peer limit")
}

func TestAtInboundPeerLimit(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1)),
	})
	for range 15 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirOutbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	assert.Equal(t, false, p.IsAboveInboundLimit(), "Inbound limit exceeded")
	for range 31 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	assert.Equal(t, true, p.IsAboveInboundLimit(), "Inbound limit not exceeded")
}

func TestPrunePeers(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{})
	defer resetCfg()
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})
	for range 15 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirOutbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	// Assert there are no prunable peers.
	candidates, numToPrune := p.PruneCandidates()
	assert.Equal(t, 0, len(candidates))
	assert.Equal(t, uint64(0), numToPrune)

	for range 18 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// All inbound peers are candidates; only the excess over the limit is to be pruned.
	candidates, numToPrune = p.PruneCandidates()
	assert.Equal(t, 18, len(candidates))
	assert.Equal(t, uint64(3), numToPrune)

	// Add in more peers.
	for range 13 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// Set up bad scores for inbound peers.
	inboundPeers := p.InboundConnected()
	for i, pid := range inboundPeers {
		modulo := i % 5
		// Increment bad scores for peers.
		for range modulo {
			scoring.RecordBadResponse(pid, peerscoring.Unknown, "test")
		}
	}
	// Assert every inbound peer is a candidate and all peers more than max are to be pruned.
	candidates, numToPrune = p.PruneCandidates()
	assert.Equal(t, 31, len(candidates))
	assert.Equal(t, uint64(16), numToPrune)
	for _, pid := range candidates {
		dir, err := p.Direction(pid)
		require.NoError(t, err)
		assert.Equal(t, network.DirInbound, dir)
	}

	// At threshold 1 any strike grey-lists a peer, and grey-listed peers sort first.
	for _, pid := range candidates[:numToPrune] {
		assert.Equal(t, true, scoring.BadResponseCount(pid) > 0, "expected grey-listed peers to be pruned first")
	}
}

func TestPruneCandidates_InboundOnlyExcess(t *testing.T) {
	// PeerLimit 30 -> connected limit 30, inbound limit 24.
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(),
	})
	for range 26 {
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// Active (26) is under the connected limit, but inbound exceeds the inbound limit by 2.
	candidates, numToPrune := p.PruneCandidates()
	assert.Equal(t, 26, len(candidates))
	assert.Equal(t, uint64(2), numToPrune)
}

func TestSetConnectionStateStampsConnectedAt(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{PeerLimit: 30})
	pid := createPeer(t, p, nil, network.DirInbound, peers.Connecting)

	// Not yet connected: zero timestamp.
	connectedAt, err := p.ConnectedAt(pid)
	require.NoError(t, err)
	assert.Equal(t, true, connectedAt.IsZero())

	// The transition to Connected stamps the time.
	p.SetConnectionState(pid, peers.Connected)
	firstConnectedAt, err := p.ConnectedAt(pid)
	require.NoError(t, err)
	assert.Equal(t, false, firstConnectedAt.IsZero())

	// A redundant Connected update does not reset tenure.
	planted := firstConnectedAt.Add(-time.Hour)
	p.SetConnectedAt(pid, planted)
	p.SetConnectionState(pid, peers.Connected)
	connectedAt, err = p.ConnectedAt(pid)
	require.NoError(t, err)
	assert.Equal(t, planted, connectedAt)

	// Disconnecting keeps the stamp; reconnecting starts tenure afresh.
	p.SetConnectionState(pid, peers.Disconnected)
	connectedAt, err = p.ConnectedAt(pid)
	require.NoError(t, err)
	assert.Equal(t, planted, connectedAt)
	p.SetConnectionState(pid, peers.Connected)
	connectedAt, err = p.ConnectedAt(pid)
	require.NoError(t, err)
	assert.Equal(t, true, connectedAt.After(planted))

	_, err = p.ConnectedAt("unknown-peer")
	assert.ErrorContains(t, "peer unknown", err)
}

func TestPruneCandidatesGreylistedFirst(t *testing.T) {
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
		Rand:      mrand.New(mrand.NewSource(42)),
	})
	for range 15 {
		createPeer(t, p, nil, network.DirOutbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	for range 18 {
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	greyListed := make(map[peer.ID]bool)
	for i, pid := range p.InboundConnected() {
		if i < 5 {
			scoring.RecordBadResponse(pid, peerscoring.Unknown, "test")
			greyListed[pid] = true
		}
	}

	firstOrderings := make(map[string]bool)
	for range 100 {
		candidates, numToPrune := p.PruneCandidates()
		require.Equal(t, 18, len(candidates))
		require.Equal(t, uint64(3), numToPrune)
		// The grey-listed peers always occupy the front of the eviction order.
		var firstOrdering strings.Builder
		for i, pid := range candidates {
			assert.Equal(t, i < 5, greyListed[pid], "grey-listed peers must sort first")
			if i < 5 {
				firstOrdering.WriteString(pid.String())
			}
		}
		firstOrderings[firstOrdering.String()] = true
	}
	// The grey-listed front is shuffled, not in one fixed order.
	assert.Equal(t, true, len(firstOrderings) > 1, "expected shuffled grey-listed orderings")
}

func TestPruneCandidatesTenurePartitionAndEpsilon(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Rand:      mrand.New(mrand.NewSource(42)),
	})
	for range 15 {
		createPeer(t, p, nil, network.DirOutbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	for range 18 {
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// inbound[0] is the oldest peer, inbound[17] the youngest.
	inbound := p.InboundConnected()
	base := time.Now().Add(-24 * time.Hour)
	tenureIndex := make(map[peer.ID]int, len(inbound))
	for i, pid := range inbound {
		p.SetConnectedAt(pid, base.Add(time.Duration(i)*time.Minute))
		tenureIndex[pid] = i
	}

	// The oldest quarter (18/4 = 4 peers) is the protected tail, youngest-first.
	wantTail := []int{3, 2, 1, 0}
	epsilonRounds, prefixOrderings := 0, make(map[string]bool)
	for range 400 {
		candidates, _ := p.PruneCandidates()
		require.Equal(t, 18, len(candidates))
		seen := make(map[peer.ID]bool, len(candidates))
		for _, pid := range candidates {
			seen[pid] = true
		}
		require.Equal(t, 18, len(seen), "candidates must be a permutation of all inbound peers")

		tenureRound := true
		for i, want := range wantTail {
			if tenureIndex[candidates[14+i]] != want {
				tenureRound = false
				break
			}
		}
		if !tenureRound {
			epsilonRounds++
			continue
		}
		var prefix strings.Builder
		for _, pid := range candidates[:14] {
			require.Equal(t, true, tenureIndex[pid] >= 4, "protected old peer found in the unprotected prefix")
			prefix.WriteString(pid.String())
		}
		prefixOrderings[prefix.String()] = true
	}
	// Epsilon rounds ignore tenure ~5% of the time.
	assert.Equal(t, true, epsilonRounds >= 1 && epsilonRounds <= 60, "epsilon round count out of range: %d", epsilonRounds)
	// The unprotected prefix is shuffled between tenure rounds.
	assert.Equal(t, true, len(prefixOrderings) > 1, "expected shuffled unprotected prefixes")
}

func TestPruneCandidatesSmallCandidateSets(t *testing.T) {
	// PeerLimit 2 -> connected limit 2, inbound limit 1.
	newStatus := func() *peers.Status {
		return peers.NewStatus(t.Context(), &peers.StatusConfig{
			PeerLimit: 2,
			Rand:      mrand.New(mrand.NewSource(42)),
		})
	}

	// A single inbound peer is within both limits: nothing to prune.
	p := newStatus()
	createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	candidates, numToPrune := p.PruneCandidates()
	assert.Equal(t, 0, len(candidates))
	assert.Equal(t, uint64(0), numToPrune)

	// Two and three candidates: the whole set is returned, no tenure tail (len/4 = 0).
	for _, n := range []int{2, 3} {
		p = newStatus()
		for range n {
			createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
		}
		for range 20 {
			candidates, numToPrune = p.PruneCandidates()
			require.Equal(t, n, len(candidates))
			require.Equal(t, uint64(n-1), numToPrune)
		}
	}

	// Four candidates: exactly the single oldest peer forms the protected tail.
	p = newStatus()
	for range 4 {
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	inbound := p.InboundConnected()
	base := time.Now().Add(-24 * time.Hour)
	oldest := inbound[0]
	for i, pid := range inbound {
		p.SetConnectedAt(pid, base.Add(time.Duration(i)*time.Minute))
	}
	oldestLast := 0
	for range 100 {
		candidates, _ := p.PruneCandidates()
		require.Equal(t, 4, len(candidates))
		if candidates[3] == oldest {
			oldestLast++
		}
	}
	// Tenure rounds (95%) always put the oldest peer last; epsilon rounds may not.
	assert.Equal(t, true, oldestLast >= 60, "oldest peer evicted too eagerly: last in only %d/100 rounds", oldestLast)
}

func TestPrunePeers_TrustedPeers(t *testing.T) {
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(1))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	for range 15 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirOutbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}
	// Assert there are no prunable peers.
	candidates, numToPrune := p.PruneCandidates()
	assert.Equal(t, 0, len(candidates))
	assert.Equal(t, uint64(0), numToPrune)

	for range 18 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// All inbound peers are candidates; only the excess over the limit is to be pruned.
	candidates, numToPrune = p.PruneCandidates()
	assert.Equal(t, 18, len(candidates))
	assert.Equal(t, uint64(3), numToPrune)

	// Add in more peers.
	for range 13 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	var trustedPeers []peer.ID
	// Set up bad scores for inbound peers.
	inboundPeers := p.InboundConnected()
	for i, pid := range inboundPeers {
		modulo := i % 5
		// Increment bad scores for peers.
		for range modulo {
			scoring.RecordBadResponse(pid, peerscoring.Unknown, "test")
		}
		if modulo == 4 {
			trustedPeers = append(trustedPeers, pid)
		}
	}
	p.SetTrustedPeers(trustedPeers)

	// Assert we have correct trusted peers
	trustedPeers = p.GetTrustedPeers()
	assert.Equal(t, 6, len(trustedPeers))

	// Assert trusted peers are not candidates and all peers more than max are to be pruned.
	candidates, numToPrune = p.PruneCandidates()
	assert.Equal(t, 25, len(candidates))
	assert.Equal(t, uint64(16), numToPrune)

	// Check that trusted peers are not candidates for pruning.
	for _, pid := range candidates {
		for _, tPid := range trustedPeers {
			assert.NotEqual(t, pid.String(), tPid.String())
		}
	}

	// Add more peers to check if trusted peers can be pruned after they are deleted from trusted peer set.
	for range 9 {
		// Peer added to peer handler.
		createPeer(t, p, nil, network.DirInbound, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	// Delete trusted peers.
	p.DeleteTrustedPeers(trustedPeers)

	candidates, numToPrune = p.PruneCandidates()
	assert.Equal(t, 40, len(candidates))
	assert.Equal(t, uint64(25), numToPrune)

	// Check that formerly trusted peers are candidates for pruning again.
	for _, tPid := range trustedPeers {
		pruned := false
		for _, pid := range candidates {
			if pid.String() == tPid.String() {
				pruned = true
			}
		}
		assert.Equal(t, true, pruned)
	}

	// Assert have zero trusted peers
	trustedPeers = p.GetTrustedPeers()
	assert.Equal(t, 0, len(trustedPeers))

	for _, pid := range candidates {
		dir, err := p.Direction(pid)
		require.NoError(t, err)
		assert.Equal(t, network.DirInbound, dir)
	}

	// Grey-listed candidates come first; once a clean candidate appears no
	// grey-listed one may follow.
	seenClean := false
	for _, pid := range candidates {
		greyListed := scoring.IsPeerGreyListed(pid) != nil
		if !greyListed {
			seenClean = true
		}
		if seenClean {
			assert.Equal(t, false, greyListed, "grey-listed candidate found after a clean one")
		}
	}
}

func TestTrustedPeerCallbacks(t *testing.T) {
	added := make(map[peer.ID]int)
	removed := make(map[peer.ID]int)
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit:            30,
		OnTrustedPeerAdded:   func(pid peer.ID) { added[pid]++ },
		OnTrustedPeerRemoved: func(pid peer.ID) { removed[pid]++ },
	})

	pids := []peer.ID{"trusted-a", "trusted-b"}
	p.SetTrustedPeers(pids)
	assert.Equal(t, 1, added["trusted-a"])
	assert.Equal(t, 1, added["trusted-b"])
	assert.Equal(t, 0, len(removed))

	p.DeleteTrustedPeers(pids[:1])
	assert.Equal(t, 1, removed["trusted-a"])
	assert.Equal(t, 0, removed["trusted-b"])

	// Nil callbacks are safe.
	p = peers.NewStatus(t.Context(), &peers.StatusConfig{PeerLimit: 30})
	p.SetTrustedPeers(pids)
	p.DeleteTrustedPeers(pids)
}

func TestStatus_BestPeer(t *testing.T) {
	type peerConfig struct {
		headSlot       primitives.Slot
		finalizedEpoch primitives.Epoch
	}

	tests := []struct {
		name               string
		peers              []*peerConfig
		limitPeers         int
		ourFinalizedEpoch  primitives.Epoch
		targetEpoch        primitives.Epoch
		targetEpochSupport int // Denotes how many peers support returned epoch.
	}{
		{
			name: "head slot matches finalized epoch",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 3 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 3 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  0,
			targetEpoch:        4,
			targetEpochSupport: 4,
		},
		{
			// Peers are compared using their finalized epoch, head should not affect peer selection.
			// Test case below is a regression case: to ensure that only epoch is used indeed.
			// (Function sorts peers, and on equal head slot, produced incorrect results).
			name: "head slots equal for peers with different finalized epochs",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 4 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  0,
			targetEpoch:        4,
			targetEpochSupport: 4,
		},
		{
			name: "head slot significantly ahead of finalized epoch (long period of non-finality)",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  0,
			targetEpoch:        4,
			targetEpochSupport: 4,
		},
		{
			name: "ignore lower epoch peers",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 41 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 43 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 44 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 45 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 46 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  5,
			targetEpoch:        6,
			targetEpochSupport: 1,
		},
		{
			name: "combine peers from several epochs starting from epoch higher than ours",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 41 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 43 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 44 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 45 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 46 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 7, headSlot: 7 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 8, headSlot: 8 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  5,
			targetEpoch:        6,
			targetEpochSupport: 5,
		},
		{
			name: "limit number of returned peers",
			peers: []*peerConfig{
				{finalizedEpoch: 4, headSlot: 41 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 42 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 43 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 44 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 4, headSlot: 45 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 3, headSlot: 46 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 7, headSlot: 7 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 8, headSlot: 8 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         4,
			ourFinalizedEpoch:  5,
			targetEpoch:        6,
			targetEpochSupport: 4,
		},
		{
			name: "handle epoch ties",
			peers: []*peerConfig{
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 6, headSlot: 6 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 7, headSlot: 7 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 8, headSlot: 8 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 8, headSlot: 8 * params.BeaconConfig().SlotsPerEpoch},
				{finalizedEpoch: 8, headSlot: 8 * params.BeaconConfig().SlotsPerEpoch},
			},
			limitPeers:         15,
			ourFinalizedEpoch:  5,
			targetEpoch:        8,
			targetEpochSupport: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(2))
			p := peers.NewStatus(t.Context(), &peers.StatusConfig{
				PeerLimit: 30,
				Scoring:   scoring,
			})
			for _, peerConfig := range tt.peers {
				setChainState(scoring, addPeer(t, p, peers.Connected), &pb.StatusV2{
					FinalizedEpoch: peerConfig.finalizedEpoch,
					HeadSlot:       peerConfig.headSlot,
				})
			}
			epoch, pids := p.BestFinalized(tt.ourFinalizedEpoch)
			if len(pids) > tt.limitPeers {
				pids = pids[:tt.limitPeers]
			}
			assert.Equal(t, tt.targetEpoch, epoch, "Unexpected epoch retrieved")
			assert.Equal(t, tt.targetEpochSupport, len(pids), "Unexpected number of peers supporting retrieved epoch")
		})
	}
}

func TestBestFinalized_returnsMaxValue(t *testing.T) {
	maxBadResponses := 2
	maxPeers := 10
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	for i := 0; i <= maxPeers+100; i++ {
		p.Add(new(enr.Record), peer.ID(rune(i)), nil, network.DirOutbound)
		p.SetConnectionState(peer.ID(rune(i)), peers.Connected)
		setChainState(scoring, peer.ID(rune(i)), &pb.StatusV2{
			FinalizedEpoch: 10,
		})
	}

	_, pids := p.BestFinalized(0)
	if len(pids) > maxPeers {
		pids = pids[:maxPeers]
	}
	assert.Equal(t, maxPeers, len(pids), "Wrong number of peers returned")
}

func TestStatus_BestNonFinalized(t *testing.T) {
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(2))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})

	peerSlots := []primitives.Slot{32, 32, 32, 32, 235, 233, 258, 268, 270}
	for i, headSlot := range peerSlots {
		p.Add(new(enr.Record), peer.ID(rune(i)), nil, network.DirOutbound)
		p.SetConnectionState(peer.ID(rune(i)), peers.Connected)
		setChainState(scoring, peer.ID(rune(i)), &pb.StatusV2{
			HeadSlot: headSlot,
		})
	}

	expectedEpoch := primitives.Epoch(8)
	retEpoch, pids := p.BestNonFinalized(3, 5)
	assert.Equal(t, expectedEpoch, retEpoch, "Incorrect Finalized epoch retrieved")
	assert.Equal(t, 3, len(pids), "Unexpected number of peers")
}

func TestStatus_CurrentEpoch(t *testing.T) {
	maxBadResponses := 2
	scoring := peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(maxBadResponses))
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   scoring,
	})
	// Peer 1
	pid1 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid1, &pb.StatusV2{
		HeadSlot: params.BeaconConfig().SlotsPerEpoch * 4,
	})
	// Peer 2
	pid2 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid2, &pb.StatusV2{
		HeadSlot: params.BeaconConfig().SlotsPerEpoch * 5,
	})
	// Peer 3
	pid3 := addPeer(t, p, peers.Connected)
	setChainState(scoring, pid3, &pb.StatusV2{
		HeadSlot: params.BeaconConfig().SlotsPerEpoch * 4,
	})

	assert.Equal(t, primitives.Epoch(5), slots.ToEpoch(scoring.HighestHeadSlot()), "Expected current epoch to be 5")
}

func TestInbound(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})
	addr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/33333")
	require.NoError(t, err)
	inbound := createPeer(t, p, addr, network.DirInbound, peers.Connected)
	createPeer(t, p, addr, network.DirOutbound, peers.Connected)

	result := p.Inbound()
	require.Equal(t, 1, len(result))
	assert.Equal(t, inbound.String(), result[0].String())
}

func TestInboundConnected(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})

	addr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/33333")
	require.NoError(t, err)
	inbound := createPeer(t, p, addr, network.DirInbound, peers.Connected)
	createPeer(t, p, addr, network.DirInbound, peers.Connecting)

	result := p.InboundConnected()
	require.Equal(t, 1, len(result))
	assert.Equal(t, inbound.String(), result[0].String())
}

func TestInboundConnectedWithProtocol(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})

	addrsTCP := []string{
		"/ip4/127.0.0.1/tcp/33333",
		"/ip4/127.0.0.2/tcp/44444",
	}

	addrsQUIC := []string{
		"/ip4/192.168.1.3/udp/13000/quic-v1",
		"/ip4/192.168.1.4/udp/14000/quic-v1",
		"/ip4/192.168.1.5/udp/14000/quic-v1",
	}

	expectedTCP := make(map[string]bool, len(addrsTCP))
	for _, addr := range addrsTCP {
		multiaddr, err := ma.NewMultiaddr(addr)
		require.NoError(t, err)

		peer := createPeer(t, p, multiaddr, network.DirInbound, peers.Connected)
		expectedTCP[peer.String()] = true
	}

	expectedQUIC := make(map[string]bool, len(addrsQUIC))
	for _, addr := range addrsQUIC {
		multiaddr, err := ma.NewMultiaddr(addr)
		require.NoError(t, err)

		peer := createPeer(t, p, multiaddr, network.DirInbound, peers.Connected)
		expectedQUIC[peer.String()] = true
	}

	// TCP
	// ---

	actualTCP := p.InboundConnectedWithProtocol(peers.TCP)
	require.Equal(t, len(expectedTCP), len(actualTCP))

	for _, actualPeer := range actualTCP {
		_, ok := expectedTCP[actualPeer.String()]
		require.Equal(t, true, ok)
	}

	// QUIC
	// ----
	actualQUIC := p.InboundConnectedWithProtocol(peers.QUIC)
	require.Equal(t, len(expectedQUIC), len(actualQUIC))

	for _, actualPeer := range actualQUIC {
		_, ok := expectedQUIC[actualPeer.String()]
		require.Equal(t, true, ok)
	}
}

func TestOutbound(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})
	addr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/33333")
	require.NoError(t, err)
	createPeer(t, p, addr, network.DirInbound, peers.Connected)
	outbound := createPeer(t, p, addr, network.DirOutbound, peers.Connected)

	result := p.Outbound()
	require.Equal(t, 1, len(result))
	assert.Equal(t, outbound.String(), result[0].String())
}

func TestOutboundConnected(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})

	addr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/33333")
	require.NoError(t, err)
	inbound := createPeer(t, p, addr, network.DirOutbound, peers.Connected)
	createPeer(t, p, addr, network.DirOutbound, peers.Connecting)

	result := p.OutboundConnected()
	require.Equal(t, 1, len(result))
	assert.Equal(t, inbound.String(), result[0].String())
}

func TestOutboundConnectedWithProtocol(t *testing.T) {
	p := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		Scoring:   peerscoring.NewScorer(peerscoring.WithBadResponseGreyListThreshold(0)),
	})

	addrsTCP := []string{
		"/ip4/127.0.0.1/tcp/33333",
		"/ip4/127.0.0.2/tcp/44444",
	}

	addrsQUIC := []string{
		"/ip4/192.168.1.3/udp/13000/quic-v1",
		"/ip4/192.168.1.4/udp/14000/quic-v1",
		"/ip4/192.168.1.5/udp/14000/quic-v1",
	}

	expectedTCP := make(map[string]bool, len(addrsTCP))
	for _, addr := range addrsTCP {
		multiaddr, err := ma.NewMultiaddr(addr)
		require.NoError(t, err)

		peer := createPeer(t, p, multiaddr, network.DirOutbound, peers.Connected)
		expectedTCP[peer.String()] = true
	}

	expectedQUIC := make(map[string]bool, len(addrsQUIC))
	for _, addr := range addrsQUIC {
		multiaddr, err := ma.NewMultiaddr(addr)
		require.NoError(t, err)

		peer := createPeer(t, p, multiaddr, network.DirOutbound, peers.Connected)
		expectedQUIC[peer.String()] = true
	}

	// TCP
	// ---

	actualTCP := p.OutboundConnectedWithProtocol(peers.TCP)
	require.Equal(t, len(expectedTCP), len(actualTCP))

	for _, actualPeer := range actualTCP {
		_, ok := expectedTCP[actualPeer.String()]
		require.Equal(t, true, ok)
	}

	// QUIC
	// ----
	actualQUIC := p.OutboundConnectedWithProtocol(peers.QUIC)
	require.Equal(t, len(expectedQUIC), len(actualQUIC))

	for _, actualPeer := range actualQUIC {
		_, ok := expectedQUIC[actualPeer.String()]
		require.Equal(t, true, ok)
	}
}

// addPeer is a helper to add a peer with a given connection state)
func addPeer(t *testing.T, p *peers.Status, state peerdata.ConnectionState) peer.ID {
	// Set up some peers with different states
	mhBytes := []byte{0x11, 0x04}
	idBytes := make([]byte, 4)
	_, err := rand.Read(idBytes)
	require.NoError(t, err)
	mhBytes = append(mhBytes, idBytes...)
	id, err := peer.IDFromBytes(mhBytes)
	require.NoError(t, err)
	p.Add(new(enr.Record), id, nil, network.DirUnknown)
	p.SetConnectionState(id, state)
	p.SetMetadata(id, wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: 0,
		Attnets:   bitfield.NewBitvector64(),
	}))
	return id
}

func createPeer(t *testing.T, p *peers.Status, addr ma.Multiaddr,
	dir network.Direction, state peerdata.ConnectionState) peer.ID {
	mhBytes := []byte{0x11, 0x04}
	idBytes := make([]byte, 4)
	_, err := rand.Read(idBytes)
	require.NoError(t, err)
	mhBytes = append(mhBytes, idBytes...)
	id, err := peer.IDFromBytes(mhBytes)
	require.NoError(t, err)
	p.Add(new(enr.Record), id, addr, dir)
	p.SetConnectionState(id, state)
	return id
}

// setChainState records a validated chain status for the peer on the test scorer.
func setChainState(scoring *peerscoring.Scorer, pid peer.ID, st *pb.StatusV2) {
	scoring.SetPeerStatus(pid, st, nil)
}
