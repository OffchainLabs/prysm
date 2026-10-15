package sync

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
)

const ilTestCurrentSlot = primitives.Slot(5)

type inclusionListTestFixture struct {
	s     *Service
	chain *mock.ChainService
	p     *p2ptest.TestP2P
}

// setupInclusionListGossip returns a service at Heze from genesis whose clock is one second into ilTestCurrentSlot.
func setupInclusionListGossip(t *testing.T) *inclusionListTestFixture {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	genesis := time.Now().Add(-time.Duration(ilTestCurrentSlot)*params.BeaconConfig().SlotDuration() - time.Second)
	p := p2ptest.NewTestP2P(t)
	chain := &mock.ChainService{Genesis: genesis}
	s := &Service{
		seenInclusionListCache: newSlotAwareCache(10),
		cfg: &config{
			p2p:               p,
			initialSync:       &mockSync.Sync{},
			chain:             chain,
			clock:             startup.NewClock(genesis, [32]byte{}),
			operationNotifier: &mock.MockOperationNotifier{},
		},
	}
	return &inclusionListTestFixture{s: s, chain: chain, p: p}
}

func testSignedInclusionList(t *testing.T, slot primitives.Slot, validator primitives.ValidatorIndex, txs ...[]byte) *ethpb.SignedInclusionList {
	t.Helper()
	if len(txs) == 0 {
		txs = [][]byte{{0x02, 0x01}}
	}
	l, err := enginev1.NewProgressiveTransactionList(txs)
	require.NoError(t, err)
	return &ethpb.SignedInclusionList{
		Message: &ethpb.InclusionList{
			Slot:           slot,
			ValidatorIndex: validator,
			DependentRoot:  bytes.Repeat([]byte{0xaa}, fieldparams.RootLength),
			Transactions:   l,
		},
		Signature: bytes.Repeat([]byte{0x01}, fieldparams.BLSSignatureLength),
	}
}

func (f *inclusionListTestFixture) toPubsub(t *testing.T, signed *ethpb.SignedInclusionList) *pubsub.Message {
	t.Helper()
	buf := new(bytes.Buffer)
	_, err := f.p.Encoding().EncodeGossip(buf, signed)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedInclusionList]()]
	topic = f.s.addDigestToTopic(topic, f.s.currentForkDigest())
	return &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}
}

func TestValidateInclusionListGossip(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		signed := testSignedInclusionList(t, ilTestCurrentSlot, 3)
		msg := f.toPubsub(t, signed)

		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
		require.Equal(t, 1, len(f.chain.InclusionListsReceived))
		got, ok := msg.ValidatorData.(*ethpb.SignedInclusionList)
		require.Equal(t, true, ok)
		require.DeepEqual(t, signed, got)
		require.Equal(t, 1, f.s.inclusionListSeenCount(inclusionListSeenKey(signed.Message)))
	})
	t.Run("self origin is accepted", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		f.s.cfg.initialSync = &mockSync.Sync{IsSyncing: true}

		result, err := f.s.validateInclusionListGossip(t.Context(), f.p.PeerID(), &pubsub.Message{})
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
		require.Equal(t, 0, len(f.chain.InclusionListsReceived))
	})
	t.Run("syncing is ignored", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		f.s.cfg.initialSync = &mockSync.Sync{IsSyncing: true}

		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, testSignedInclusionList(t, ilTestCurrentSlot, 3)))
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationIgnore, result)
		require.Equal(t, 0, len(f.chain.InclusionListsReceived))
	})
	t.Run("invalid topic is rejected", func(t *testing.T) {
		f := setupInclusionListGossip(t)

		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", &pubsub.Message{Message: &pb.Message{}})
		require.ErrorIs(t, err, p2p.ErrInvalidTopic)
		require.Equal(t, pubsub.ValidationReject, result)
	})
	t.Run("message above the size limit is rejected", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		// 8192 transaction bytes and one empty transaction: the byte limit holds, the message size limit does not.
		txs := make([][]byte, 0, params.BeaconConfig().MaxTransactionsBytesPerInclusionList+1)
		for range params.BeaconConfig().MaxTransactionsBytesPerInclusionList {
			txs = append(txs, []byte{0x01})
		}
		txs = append(txs, []byte{})
		signed := testSignedInclusionList(t, ilTestCurrentSlot, 3, txs...)
		require.Equal(t, true, signed.SizeSSZ() > fieldparams.MaxSignedInclusionListSize)

		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, signed))
		require.ErrorContains(t, "the maximum is", err)
		require.Equal(t, pubsub.ValidationReject, result)
		require.Equal(t, 0, len(f.chain.InclusionListsReceived))
	})
	t.Run("message at the size limit is accepted", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		txs := make([][]byte, 0, params.BeaconConfig().MaxTransactionsBytesPerInclusionList)
		for range params.BeaconConfig().MaxTransactionsBytesPerInclusionList {
			txs = append(txs, []byte{0x01})
		}
		signed := testSignedInclusionList(t, ilTestCurrentSlot, 3, txs...)
		require.Equal(t, fieldparams.MaxSignedInclusionListSize, signed.SizeSSZ())

		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, signed))
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
	t.Run("third message is ignored", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		for i, want := range []pubsub.ValidationResult{pubsub.ValidationAccept, pubsub.ValidationAccept, pubsub.ValidationIgnore} {
			signed := testSignedInclusionList(t, ilTestCurrentSlot, 3, []byte{0x02, byte(i)})
			result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, signed))
			require.NoError(t, err)
			require.Equal(t, want, result)
		}
		// The third message does not reach the store.
		require.Equal(t, 2, len(f.chain.InclusionListsReceived))

		t.Run("other validator, slot, or dependent root is counted apart", func(t *testing.T) {
			other := testSignedInclusionList(t, ilTestCurrentSlot, 4)
			result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, other))
			require.NoError(t, err)
			require.Equal(t, pubsub.ValidationAccept, result)

			other = testSignedInclusionList(t, ilTestCurrentSlot, 3)
			other.Message.DependentRoot = bytes.Repeat([]byte{0xbb}, fieldparams.RootLength)
			result, err = f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, other))
			require.NoError(t, err)
			require.Equal(t, pubsub.ValidationAccept, result)
			// Gossip accepts only the current slot, so the slot part of the key is checked on the key itself.
			previous := testSignedInclusionList(t, ilTestCurrentSlot-1, 3)
			require.NotEqual(t, inclusionListSeenKey(testSignedInclusionList(t, ilTestCurrentSlot, 3).Message), inclusionListSeenKey(previous.Message))
		})
	})
	t.Run("seen count only after validation", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		f.chain.OnInclusionListErr = blockchain.ErrInclusionListInvalidSignature
		signed := testSignedInclusionList(t, ilTestCurrentSlot, 3)
		for range 3 {
			result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, signed))
			require.ErrorIs(t, err, blockchain.ErrInclusionListInvalidSignature)
			require.Equal(t, pubsub.ValidationReject, result)
		}
		require.Equal(t, 0, f.s.inclusionListSeenCount(inclusionListSeenKey(signed.Message)))

		f.chain.OnInclusionListErr = nil
		result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, signed))
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
	t.Run("not the current slot is ignored", func(t *testing.T) {
		for name, slot := range map[string]primitives.Slot{"previous slot": ilTestCurrentSlot - 1, "next slot": ilTestCurrentSlot + 1} {
			t.Run(name, func(t *testing.T) {
				f := setupInclusionListGossip(t)
				result, err := f.s.validateInclusionListGossip(t.Context(), "peer", f.toPubsub(t, testSignedInclusionList(t, slot, 3)))
				require.ErrorContains(t, "is not the current slot", err)
				require.Equal(t, pubsub.ValidationIgnore, result)
				require.Equal(t, 0, len(f.chain.InclusionListsReceived))
			})
		}
	})
	t.Run("chain errors", func(t *testing.T) {
		tests := []struct {
			err  error
			want pubsub.ValidationResult
		}{
			{err: blockchain.ErrInclusionListNoTransactions, want: pubsub.ValidationIgnore},
			{err: errors.Wrap(blockchain.ErrInclusionListTooLarge, "8193 bytes"), want: pubsub.ValidationReject},
			{err: blockchain.ErrInclusionListEmptyTransaction, want: pubsub.ValidationReject},
			{err: errors.Wrap(blockchain.ErrInclusionListUnknownDependentBlock, "0xaa"), want: pubsub.ValidationIgnore},
			{err: errors.Wrap(blockchain.ErrInclusionListDependentBlockTooLate, "block slot 9"), want: pubsub.ValidationReject},
			{err: errors.Wrap(blockchain.ErrInclusionListInvalidDependentRoot, "0xaa"), want: pubsub.ValidationIgnore},
			{err: errors.Wrap(blockchain.ErrInclusionListDependentStateUnavailable, "0xaa"), want: pubsub.ValidationIgnore},
			{err: errors.Wrap(blockchain.ErrInclusionListNotInCommittee, "validator 3"), want: pubsub.ValidationReject},
			{err: errors.Wrap(blockchain.ErrInclusionListInvalidSignature, "bad"), want: pubsub.ValidationReject},
			{err: errors.Wrap(blockchain.ErrInclusionListOutOfWindow, "slot 6"), want: pubsub.ValidationIgnore},
			{err: blockchain.ErrInclusionListPreHeze, want: pubsub.ValidationIgnore},
			{err: blockchain.ErrNilInclusionList, want: pubsub.ValidationReject},
			{err: blockchain.ErrInclusionListDependentRootLength, want: pubsub.ValidationReject},
			{err: errors.New("could not compute the committee"), want: pubsub.ValidationIgnore},
		}
		for _, tc := range tests {
			t.Run(tc.err.Error(), func(t *testing.T) {
				f := setupInclusionListGossip(t)
				f.chain.OnInclusionListErr = tc.err
				signed := testSignedInclusionList(t, ilTestCurrentSlot, 3)
				msg := f.toPubsub(t, signed)

				result, err := f.s.validateInclusionListGossip(t.Context(), "peer", msg)
				require.ErrorIs(t, err, tc.err)
				require.Equal(t, tc.want, result)
				require.Equal(t, nil, msg.ValidatorData)
				require.Equal(t, 0, f.s.inclusionListSeenCount(inclusionListSeenKey(signed.Message)))
			})
		}
	})
}

func TestInclusionListSubscriber(t *testing.T) {
	t.Run("sends the event", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		events := make(chan *feed.Event, 1)
		sub := f.s.cfg.operationNotifier.OperationFeed().Subscribe(events)
		defer sub.Unsubscribe()
		signed := testSignedInclusionList(t, ilTestCurrentSlot, 3)

		require.NoError(t, f.s.inclusionListSubscriber(context.Background(), signed))
		select {
		case ev := <-events:
			require.Equal(t, feed.EventType(opfeed.InclusionListReceived), ev.Type)
			data, ok := ev.Data.(*opfeed.InclusionListReceivedData)
			require.Equal(t, true, ok)
			require.DeepEqual(t, signed, data.SignedInclusionList)
		case <-time.After(time.Second):
			t.Fatal("no inclusion list event")
		}
		// The subscriber does not process the list again.
		require.Equal(t, 0, len(f.chain.InclusionListsReceived))
	})
	t.Run("wrong message", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		require.ErrorIs(t, f.s.inclusionListSubscriber(context.Background(), &ethpb.SignedProposerPreferences{}), errWrongMessage)
	})
	t.Run("nil message", func(t *testing.T) {
		f := setupInclusionListGossip(t)
		require.ErrorIs(t, f.s.inclusionListSubscriber(context.Background(), &ethpb.SignedInclusionList{}), errNilMessage)
	})
}
