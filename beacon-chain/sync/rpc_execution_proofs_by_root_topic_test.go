package sync

import (
	"io"
	"sync"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/execproofs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

func TestExecutionProofsByRootRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	protocolID := protocol.ID(p2p.RPCExecutionProofsByRootTopicV1) + "/" + encoder.ProtocolSuffixSSZSnappy

	t.Run("wrong message type", func(t *testing.T) {
		service := &Service{}
		err := service.executionProofsByRootRPCHandler(t.Context(), nil, nil)
		require.ErrorContains(t, "message is not type ExecutionProofsByRootRequest", err)
	})

	t.Run("invalid request - count_needed is 0", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: true,
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		service := &Service{cfg: &config{p2p: localP2P}, rateLimiter: newRateLimiter(localP2P)}
		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			code, errMsg, err := readStatusCodeNoDeadline(stream, localP2P.Encoding())
			require.NoError(t, err)
			require.Equal(t, responseCodeInvalidRequest, code)
			require.Equal(t, "count_needed must be greater than 0", errMsg)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		blockRoot := bytesutil.PadTo([]byte("blockroot"), 32)
		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot,
			CountNeeded: 0, // Invalid: must be > 0
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) >= 0)

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NotNil(t, err)
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) < 0)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("zkVM disabled - returns empty", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: false, // Disabled
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		execProofPool := execproofs.NewPool()
		service := &Service{
			cfg: &config{
				p2p:           localP2P,
				execProofPool: execProofPool,
			},
			rateLimiter: newRateLimiter(localP2P),
		}
		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			// Should receive no proofs (stream should end)
			_, err := ReadChunkedExecutionProof(stream, localP2P, true)
			require.ErrorIs(t, err, io.EOF)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		blockRoot := bytesutil.PadTo([]byte("blockroot"), 32)
		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot,
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("no proofs available", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: true,
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		execProofPool := execproofs.NewPool()
		service := &Service{
			cfg: &config{
				p2p:           localP2P,
				execProofPool: execProofPool,
			},
			rateLimiter: newRateLimiter(localP2P),
		}
		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			// Should receive no proofs (stream should end)
			_, err := ReadChunkedExecutionProof(stream, localP2P, true)
			require.ErrorIs(t, err, io.EOF)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		blockRoot := bytesutil.PadTo([]byte("blockroot"), 32)
		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot,
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("nominal - returns requested proofs", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: true,
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		// Create execution proof pool with some proofs
		execProofPool := execproofs.NewPool()
		blockRoot := [32]byte{0x01, 0x02, 0x03}

		// Add 3 proofs for the same block
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)
		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}
		proof2 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(2),
			ProofData: []byte("proof2"),
		}
		proof3 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(3),
			ProofData: []byte("proof3"),
		}

		execProofPool.Insert(proof1)
		execProofPool.Insert(proof2)
		execProofPool.Insert(proof3)

		beaconDB := testDB.SetupDB(t)
		service := &Service{
			cfg: &config{
				p2p:           localP2P,
				beaconDB:      beaconDB,
				clock:         clock,
				execProofPool: execProofPool,
				chain:         &chainMock.ChainService{},
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			proofs := make([]*ethpb.ExecutionProof, 0, 2)

			for i := range 2 {
				isFirstChunk := i == 0
				proof, err := ReadChunkedExecutionProof(stream, remoteP2P, isFirstChunk)
				if errors.Is(err, io.EOF) {
					break
				}

				assert.NoError(t, err)
				proofs = append(proofs, proof)
			}

			assert.Equal(t, 2, len(proofs))
			// Should receive proof1 and proof2 (first 2 in pool)
			assert.DeepEqual(t, blockRoot[:], proofs[0].BlockRoot)
			assert.DeepEqual(t, blockRoot[:], proofs[1].BlockRoot)
			assert.Equal(t, primitives.ExecutionProofId(1), proofs[0].ProofId)
			assert.Equal(t, primitives.ExecutionProofId(2), proofs[1].ProofId)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NoError(t, err)
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) >= 0)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("filters already_have proofs", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: true,
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		// Create execution proof pool with some proofs
		execProofPool := execproofs.NewPool()
		blockRoot := [32]byte{0x01, 0x02, 0x03}

		// Add 4 proofs for the same block
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)
		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}
		proof2 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(2),
			ProofData: []byte("proof2"),
		}
		proof3 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(3),
			ProofData: []byte("proof3"),
		}
		proof4 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(4),
			ProofData: []byte("proof4"),
		}

		execProofPool.Insert(proof1)
		execProofPool.Insert(proof2)
		execProofPool.Insert(proof3)
		execProofPool.Insert(proof4)

		beaconDB := testDB.SetupDB(t)
		service := &Service{
			cfg: &config{
				p2p:           localP2P,
				beaconDB:      beaconDB,
				clock:         clock,
				execProofPool: execProofPool,
				chain:         &chainMock.ChainService{},
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			proofs := make([]*ethpb.ExecutionProof, 0, 2)

			for i := range 3 {
				isFirstChunk := i == 0
				proof, err := ReadChunkedExecutionProof(stream, remoteP2P, isFirstChunk)
				if errors.Is(err, io.EOF) {
					break
				}

				assert.NoError(t, err)
				proofs = append(proofs, proof)
			}

			// Should skip proof1 and proof2 (already_have), and return proof3 and proof4
			assert.Equal(t, 2, len(proofs))
			assert.Equal(t, primitives.ExecutionProofId(3), proofs[0].ProofId)
			assert.Equal(t, primitives.ExecutionProofId(4), proofs[1].ProofId)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{1, 2}, // Already have proof1 and proof2
		}

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("partial send - less proofs than requested", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			EnableZkvm: true,
		})
		defer resetCfg()

		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		// Create execution proof pool with only 2 proofs
		execProofPool := execproofs.NewPool()
		blockRoot := [32]byte{0x01, 0x02, 0x03}

		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)
		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}
		proof2 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(2),
			ProofData: []byte("proof2"),
		}

		execProofPool.Insert(proof1)
		execProofPool.Insert(proof2)

		beaconDB := testDB.SetupDB(t)
		service := &Service{
			cfg: &config{
				p2p:           localP2P,
				beaconDB:      beaconDB,
				clock:         clock,
				execProofPool: execProofPool,
				chain:         &chainMock.ChainService{},
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			proofs := make([]*ethpb.ExecutionProof, 0, 5)

			for i := range 5 {
				isFirstChunk := i == 0
				proof, err := ReadChunkedExecutionProof(stream, remoteP2P, isFirstChunk)
				if errors.Is(err, io.EOF) {
					break
				}

				assert.NoError(t, err)
				proofs = append(proofs, proof)
			}

			// Should only receive 2 proofs (not 5 as requested)
			assert.Equal(t, 2, len(proofs))
			assert.Equal(t, primitives.ExecutionProofId(1), proofs[0].ProofId)
			assert.Equal(t, primitives.ExecutionProofId(2), proofs[1].ProofId)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 5, // Request 5 but only 2 available
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		err = service.executionProofsByRootRPCHandler(t.Context(), req, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})
}

func TestValidateExecutionProofsByRootRequest(t *testing.T) {
	t.Run("invalid - count_needed is 0", func(t *testing.T) {
		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   bytesutil.PadTo([]byte("blockroot"), 32),
			CountNeeded: 0,
			AlreadyHave: []primitives.ExecutionProofId{},
		}
		err := validateExecutionProofsByRootRequest(req)
		require.ErrorContains(t, "count_needed must be greater than 0", err)
	})

	t.Run("valid", func(t *testing.T) {
		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   bytesutil.PadTo([]byte("blockroot"), 32),
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{},
		}
		err := validateExecutionProofsByRootRequest(req)
		require.NoError(t, err)
	})
}

func TestSendExecutionProofsByRootRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	protocolID := protocol.ID(p2p.RPCExecutionProofsByRootTopicV1) + "/" + encoder.ProtocolSuffixSSZSnappy

	t.Run("count_needed is 0 - returns error", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		blockRoot := bytesutil.PadTo([]byte("blockroot"), 32)

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot,
			CountNeeded: 0,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.ErrorContains(t, "count_needed must be greater than 0", err)
		require.Equal(t, 0, len(proofs))
	})

	t.Run("success - receives requested proofs", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		blockRoot := [32]byte{0x01, 0x02, 0x03}
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)

		// Create proofs to send back
		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}
		proof2 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(2),
			ProofData: []byte("proof2"),
		}

		// Setup remote to send proofs
		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer func() {
				_ = stream.Close()
			}()

			// Read the request (we don't validate it in this test)
			_ = &ethpb.ExecutionProofsByRootRequest{}

			// Send proof1
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof1))

			// Send proof2
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof2))
		})

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 2,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.NoError(t, err)
		require.Equal(t, 2, len(proofs))
		assert.Equal(t, primitives.ExecutionProofId(1), proofs[0].ProofId)
		assert.Equal(t, primitives.ExecutionProofId(2), proofs[1].ProofId)
		assert.DeepEqual(t, blockRoot[:], proofs[0].BlockRoot)
		assert.DeepEqual(t, blockRoot[:], proofs[1].BlockRoot)
	})

	t.Run("partial response - EOF before count_needed", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		blockRoot := [32]byte{0x01, 0x02, 0x03}
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)

		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}

		// Setup remote to send only 1 proof (but we request 5)
		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer func() {
				_ = stream.Close()
			}()
			// Send only proof1
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof1))
		})

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 5, // Request 5 but only get 1
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.NoError(t, err)
		require.Equal(t, 1, len(proofs)) // Only received 1
		assert.Equal(t, primitives.ExecutionProofId(1), proofs[0].ProofId)
	})

	t.Run("invalid block root - validation fails", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		requestedRoot := [32]byte{0x01, 0x02, 0x03}
		wrongRoot := [32]byte{0xFF, 0xFF, 0xFF}
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)

		// Create proof with wrong block root
		proof1 := &ethpb.ExecutionProof{
			BlockRoot: wrongRoot[:], // Wrong root!
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer func() {
				_ = stream.Close()
			}()
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof1))
		})

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   requestedRoot[:],
			CountNeeded: 1,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.ErrorContains(t, "does not match requested root", err)
		require.Equal(t, 0, len(proofs))
	})

	t.Run("already_have proof - validation fails", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		blockRoot := [32]byte{0x01, 0x02, 0x03}
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)

		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(1),
			ProofData: []byte("proof1"),
		}

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer func() {
				_ = stream.Close()
			}()
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof1))
		})

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 1,
			AlreadyHave: []primitives.ExecutionProofId{1}, // Already have proof_id 1
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.ErrorContains(t, "received proof we already have", err)
		require.Equal(t, 0, len(proofs))
	})

	t.Run("invalid proof_id - validation fails", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		remoteP2P := p2ptest.NewTestP2P(t)
		localP2P.Connect(remoteP2P)

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
		blockRoot := [32]byte{0x01, 0x02, 0x03}
		blockHash := bytesutil.PadTo([]byte("blockhash"), 32)

		proof1 := &ethpb.ExecutionProof{
			BlockRoot: blockRoot[:],
			BlockHash: blockHash,
			Slot:      primitives.Slot(10),
			ProofId:   primitives.ExecutionProofId(255), // Invalid proof_id (max valid is 7)
			ProofData: []byte("proof1"),
		}

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer func() {
				_ = stream.Close()
			}()
			require.NoError(t, WriteExecutionProofChunk(stream, remoteP2P.Encoding(), proof1))
		})

		req := &ethpb.ExecutionProofsByRootRequest{
			BlockRoot:   blockRoot[:],
			CountNeeded: 1,
			AlreadyHave: []primitives.ExecutionProofId{},
		}

		proofs, err := SendExecutionProofsByRootRequest(t.Context(), clock, localP2P, remoteP2P.PeerID(), req)
		require.ErrorContains(t, "invalid proof_id", err)
		require.Equal(t, 0, len(proofs))
	})
}
