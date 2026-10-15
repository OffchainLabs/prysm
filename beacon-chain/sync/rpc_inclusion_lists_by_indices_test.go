package sync

import (
	"io"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	leakybucket "github.com/OffchainLabs/prysm/v7/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

type inclusionListRPCFixture struct {
	st        state.BeaconState
	keys      []bls.SecretKey
	root      [32]byte
	slot      primitives.Slot
	committee []primitives.ValidatorIndex
}

// setupInclusionListRPC activates every fork before Heze at genesis and Heze at epoch 1, with validatorsPerSlot active validators per slot.
func setupInclusionListRPC(t *testing.T, validatorsPerSlot uint64) *inclusionListRPCFixture {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	// The minimal build has minimal field sizes, so it needs the minimal config too.
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig().Copy()
	}
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	st, keys := util.DeterministicGenesisStateGloas(t, validatorsPerSlot*uint64(cfg.SlotsPerEpoch))
	require.NoError(t, st.SetDepositRequestsStartIndex(st.Eth1DepositIndex()))
	slot := cfg.SlotsPerEpoch + 2
	committee, err := helpers.InclusionListCommittee(t.Context(), st, slot)
	require.NoError(t, err)
	return &inclusionListRPCFixture{st: st, keys: keys, root: [32]byte{'d', 'e', 'p'}, slot: slot, committee: committee}
}

func (f *inclusionListRPCFixture) signedList(t *testing.T, slot primitives.Slot, validatorIndex primitives.ValidatorIndex, txs ...[]byte) *ethpb.SignedInclusionList {
	list, err := enginev1.NewProgressiveTransactionList(txs)
	require.NoError(t, err)
	msg := &ethpb.InclusionList{Slot: slot, ValidatorIndex: validatorIndex, DependentRoot: f.root[:], Transactions: list}
	domain, err := helpers.InclusionListSigningDomain(slots.ToEpoch(slot), f.st.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(msg, domain)
	require.NoError(t, err)
	return &ethpb.SignedInclusionList{Message: msg, Signature: f.keys[validatorIndex].Sign(signingRoot[:]).Marshal()}
}

// listAt returns a signed list for the committee member at the position.
func (f *inclusionListRPCFixture) listAt(t *testing.T, position int) *ethpb.SignedInclusionList {
	return f.signedList(t, f.slot, f.committee[position], []byte{byte(position + 1), 0xaa})
}

func (f *inclusionListRPCFixture) request(slot primitives.Slot, positions ...int) *p2ptypes.InclusionListsByIndicesReq {
	bits := bitfield.NewBitvector16()
	for _, p := range positions {
		bits.SetBitAt(uint64(p), true)
	}
	return &p2ptypes.InclusionListsByIndicesReq{Slot: slot, DependentRoot: f.root, Indices: [2]byte(bits)}
}

// connect returns the requester, the server, a server service with a clock at the current slot, and the chain that holds the store.
// Without a LookaheadStateCB, the chain gives the genesis state as the lookahead state.
func (f *inclusionListRPCFixture) connect(t *testing.T, store *cache.InclusionListStore, current primitives.Slot) (*p2ptest.TestP2P, *p2ptest.TestP2P, *Service, *mock.ChainService) {
	local, remote := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	t.Cleanup(func() { assert.NoError(t, local.BHost.Close()) })
	t.Cleanup(func() { assert.NoError(t, remote.BHost.Close()) })
	local.Connect(remote)
	clock := startup.NewClock(time.Now(), [32]byte(f.st.GenesisValidatorsRoot()), startup.WithSlotAsNow(current))
	chain := &mock.ChainService{State: f.st, ILStore: store}
	svc := &Service{
		cfg:         &config{p2p: remote, clock: clock, chain: chain},
		rateLimiter: newRateLimiter(remote),
	}
	t.Cleanup(svc.rateLimiter.free)
	return local, remote, svc, chain
}

func inclusionListTopic(p p2p.EncodingProvider) string {
	return p2p.RPCInclusionListsByIndicesTopicV1 + p.Encoding().ProtocolSuffix()
}

type inclusionListExchange struct {
	lists      []*ethpb.SignedInclusionList
	errMsg     string // The message of a response chunk with an error code.
	handlerErr error
}

// exchange sends the request to the server handler and reads the response chunks.
func exchange(t *testing.T, local, remote *p2ptest.TestP2P, svc *Service, req *p2ptypes.InclusionListsByIndicesReq) inclusionListExchange {
	handlerDone := make(chan error, 1)
	remote.SetStreamHandler(inclusionListTopic(remote), func(stream network.Stream) {
		got := new(p2ptypes.InclusionListsByIndicesReq)
		if err := remote.Encoding().DecodeWithMaxLength(stream, got); err != nil {
			handlerDone <- err
			return
		}
		handlerDone <- svc.inclusionListsByIndicesRPCHandler(t.Context(), got, stream)
	})
	stream, err := local.Send(t.Context(), req, p2p.RPCInclusionListsByIndicesTopicV1, remote.PeerID())
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	var result inclusionListExchange
	digest := params.ForkDigest(slots.ToEpoch(req.Slot))
	for {
		code, msg, err := ReadStatusCode(stream, local.Encoding())
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if code != responseCodeSuccess {
			result.errMsg = msg
			break
		}
		ctxBytes, err := readContextFromStream(stream)
		require.NoError(t, err)
		require.DeepEqual(t, digest[:], ctxBytes)
		signed := &ethpb.SignedInclusionList{}
		require.NoError(t, local.Encoding().DecodeWithMaxLength(stream, signed))
		result.lists = append(result.lists, signed)
	}
	select {
	case result.handlerErr = <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the inclusion lists handler")
	}
	return result
}

func TestInclusionListsByIndicesRPCHandler(t *testing.T) {
	// One committee of 16 distinct validators per slot.
	f := setupInclusionListRPC(t, fieldparams.InclusionListCommitteeSize)
	seen := make(map[primitives.ValidatorIndex]bool)
	for _, idx := range f.committee {
		require.Equal(t, false, seen[idx], "committee members must be distinct")
		seen[idx] = true
	}
	stored := make(map[int]*ethpb.SignedInclusionList)
	newStore := func(positions ...int) *cache.InclusionListStore {
		store := cache.NewInclusionListStore()
		for _, p := range positions {
			stored[p] = f.listAt(t, p)
			require.Equal(t, true, store.Process(stored[p], true))
		}
		return store
	}
	// countLookaheadCalls makes the chain count the lookahead state calls.
	countLookaheadCalls := func(chain *mock.ChainService) *int {
		calls := 0
		chain.LookaheadStateCB = func([32]byte, primitives.Epoch) (state.ReadOnlyBeaconState, error) {
			calls++
			return f.st, nil
		}
		return &calls
	}

	t.Run("serves the requested lists only and omits missing ones", func(t *testing.T) {
		store := newStore(0, 1, 2, 3)
		local, remote, svc, _ := f.connect(t, store, f.slot)
		got := exchange(t, local, remote, svc, f.request(f.slot, 1, 3, 5))
		require.NoError(t, got.handlerErr)
		require.Equal(t, "", got.errMsg)
		require.Equal(t, 2, len(got.lists))
		require.DeepSSZEqual(t, stored[1], got.lists[0])
		require.DeepSSZEqual(t, stored[3], got.lists[1])
	})

	t.Run("omits equivocators", func(t *testing.T) {
		store := newStore(0, 2)
		other := f.signedList(t, f.slot, f.committee[2], []byte{0xbb})
		require.Equal(t, false, store.Process(other, true))
		require.Equal(t, true, store.IsEquivocator(f.slot, f.root, f.committee[2]))
		_, _, ok := store.Get(f.slot, f.root, f.committee[2])
		require.Equal(t, true, ok, "the store keeps the first list of an equivocator")
		local, remote, svc, _ := f.connect(t, store, f.slot)
		got := exchange(t, local, remote, svc, f.request(f.slot, 0, 2))
		require.NoError(t, got.handlerErr)
		require.Equal(t, 1, len(got.lists))
		require.DeepSSZEqual(t, stored[0], got.lists[0])
	})

	t.Run("no stored list for the dependent root needs no state", func(t *testing.T) {
		store := newStore(0)
		local, remote, svc, chain := f.connect(t, store, f.slot)
		calls := countLookaheadCalls(chain)
		req := f.request(f.slot, 0)
		req.DependentRoot = [32]byte{'u', 'n', 'k', 'n', 'o', 'w', 'n'}
		got := exchange(t, local, remote, svc, req)
		require.NoError(t, got.handlerErr)
		require.Equal(t, 0, len(got.lists))
		require.Equal(t, 0, *calls)
	})

	t.Run("no requested position needs no state", func(t *testing.T) {
		store := newStore(0)
		local, remote, svc, chain := f.connect(t, store, f.slot)
		calls := countLookaheadCalls(chain)
		got := exchange(t, local, remote, svc, f.request(f.slot))
		require.NoError(t, got.handlerErr)
		require.Equal(t, "", got.errMsg)
		require.Equal(t, 0, len(got.lists))
		require.Equal(t, 0, *calls)
	})

	t.Run("previous slot is in the window", func(t *testing.T) {
		store := newStore(4)
		local, remote, svc, _ := f.connect(t, store, f.slot+1)
		got := exchange(t, local, remote, svc, f.request(f.slot, 4))
		require.NoError(t, got.handlerErr)
		require.Equal(t, 1, len(got.lists))
	})

	t.Run("ResourceUnavailable below the minimum request slot", func(t *testing.T) {
		store := newStore(0)
		local, remote, svc, _ := f.connect(t, store, f.slot+2)
		got := exchange(t, local, remote, svc, f.request(f.slot, 0))
		require.Equal(t, p2ptypes.ErrResourceUnavailable.Error(), got.errMsg)
		require.NoError(t, got.handlerErr)
	})

	t.Run("minimum request slot is not before the Heze fork", func(t *testing.T) {
		hezeStart := params.BeaconConfig().SlotsPerEpoch
		require.Equal(t, hezeStart, minimumInclusionListRequestSlot(0))
		require.Equal(t, hezeStart, minimumInclusionListRequestSlot(hezeStart))
		require.Equal(t, hezeStart+4, minimumInclusionListRequestSlot(hezeStart+5))
	})

	t.Run("rate limited", func(t *testing.T) {
		store := newStore(0, 1, 2)
		local, remote, svc, _ := f.connect(t, store, f.slot)
		svc.rateLimiter.limiterMap[inclusionListTopic(remote)] = leakybucket.NewCollector(0.000001, 2, time.Second, false)
		got := exchange(t, local, remote, svc, f.request(f.slot, 0, 1, 2))
		require.Equal(t, p2ptypes.ErrRateLimited.Error(), got.errMsg)
		require.ErrorIs(t, got.handlerErr, p2ptypes.ErrRateLimited)
	})

	t.Run("no lookahead state serves no lists", func(t *testing.T) {
		for _, lookaheadErr := range []error{blockchain.ErrInvalidDependentRoot, blockchain.ErrDependentStateUnavailable} {
			t.Run(lookaheadErr.Error(), func(t *testing.T) {
				store := newStore(0)
				local, remote, svc, chain := f.connect(t, store, f.slot)
				chain.LookaheadStateCB = func([32]byte, primitives.Epoch) (state.ReadOnlyBeaconState, error) {
					return nil, errors.Wrap(lookaheadErr, "lookahead")
				}
				got := exchange(t, local, remote, svc, f.request(f.slot, 0))
				require.NoError(t, got.handlerErr)
				require.Equal(t, "", got.errMsg)
				require.Equal(t, 0, len(got.lists))
			})
		}
	})

	t.Run("other lookahead state error is a server error", func(t *testing.T) {
		store := newStore(0)
		local, remote, svc, chain := f.connect(t, store, f.slot)
		chain.LookaheadStateCB = func([32]byte, primitives.Epoch) (state.ReadOnlyBeaconState, error) {
			return nil, errors.New("head state")
		}
		got := exchange(t, local, remote, svc, f.request(f.slot, 0))
		require.Equal(t, p2ptypes.ErrGeneric.Error(), got.errMsg)
		require.ErrorContains(t, "head state", got.handlerErr)
	})

	t.Run("invalid request type", func(t *testing.T) {
		local, remote, svc, _ := f.connect(t, cache.NewInclusionListStore(), f.slot)
		// Hold the stream open, a reset during negotiation fails NewStream.
		done := make(chan struct{})
		t.Cleanup(func() { close(done) })
		remote.BHost.SetStreamHandler(protocol.ID(inclusionListTopic(remote)), func(s network.Stream) {
			<-done
			_ = s.Reset()
		})
		stream, err := local.BHost.NewStream(t.Context(), remote.BHost.ID(), protocol.ID(inclusionListTopic(remote)))
		require.NoError(t, err)
		require.ErrorContains(t, "message is not type", svc.inclusionListsByIndicesRPCHandler(t.Context(), "not-a-request", stream))
	})

	t.Run("committee comes from the lookahead state of the lookahead epoch", func(t *testing.T) {
		// The list slot is in epoch 2, so the lookahead epoch is 1.
		slot := 2*params.BeaconConfig().SlotsPerEpoch + 1
		advanced, err := transition.ProcessSlots(t.Context(), f.st.Copy(), params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		committee, err := helpers.InclusionListCommittee(t.Context(), advanced, slot)
		require.NoError(t, err)
		store := cache.NewInclusionListStore()
		want := f.signedList(t, slot, committee[0], []byte{0x01})
		require.Equal(t, true, store.Process(want, true))
		local, remote, svc, chain := f.connect(t, store, slot)
		var gotRoot [32]byte
		var gotEpoch primitives.Epoch
		chain.LookaheadStateCB = func(root [32]byte, epoch primitives.Epoch) (state.ReadOnlyBeaconState, error) {
			gotRoot, gotEpoch = root, epoch
			return advanced, nil
		}
		got := exchange(t, local, remote, svc, f.request(slot, 0))
		require.NoError(t, got.handlerErr)
		require.Equal(t, f.root, gotRoot)
		require.Equal(t, primitives.Epoch(1), gotEpoch)
		require.Equal(t, 1, len(got.lists))
		require.DeepSSZEqual(t, want, got.lists[0])
	})

	t.Run("a validator at two requested positions gets one chunk", func(t *testing.T) {
		// Eight active validators per slot fill the 16 committee positions twice, so position i and i+8 hold the same validator.
		f := setupInclusionListRPC(t, 8)
		require.Equal(t, f.committee[0], f.committee[8])
		store := cache.NewInclusionListStore()
		want := f.listAt(t, 0)
		require.Equal(t, true, store.Process(want, true))
		local, remote, svc, _ := f.connect(t, store, f.slot)
		got := exchange(t, local, remote, svc, f.request(f.slot, 0, 8))
		require.NoError(t, got.handlerErr)
		require.Equal(t, 1, len(got.lists))
		require.DeepSSZEqual(t, want, got.lists[0])
	})
}
