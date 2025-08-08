package validator

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/client/builder"
	blockchainTest "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	builderTest "github.com/OffchainLabs/prysm/v6/beacon-chain/builder/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	dbTest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	powtesting "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	v1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestServer_setExecutionData(t *testing.T) {
	hook := logTest.NewGlobal()

	ctx := t.Context()
	cfg := params.BeaconConfig().Copy()
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.SetupTestConfigCleanup(t)

	beaconDB := dbTest.SetupDB(t)
	capellaTransitionState, _ := util.DeterministicGenesisStateCapella(t, 1)
	wrappedHeaderCapella, err := blocks.WrappedExecutionPayloadHeaderCapella(&v1.ExecutionPayloadHeaderCapella{BlockNumber: 1})
	require.NoError(t, err)
	require.NoError(t, capellaTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderCapella))
	b2pbCapella := util.NewBeaconBlockCapella()
	b2rCapella, err := b2pbCapella.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), beaconDB, b2pbCapella)
	require.NoError(t, capellaTransitionState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Root: b2rCapella[:],
	}))
	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(t.Context(), []primitives.ValidatorIndex{0}, []common.Address{{}}))

	denebTransitionState, _ := util.DeterministicGenesisStateDeneb(t, 1)
	wrappedHeaderDeneb, err := blocks.WrappedExecutionPayloadHeaderDeneb(&v1.ExecutionPayloadHeaderDeneb{BlockNumber: 2})
	require.NoError(t, err)
	require.NoError(t, denebTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderDeneb))
	b2pbDeneb := util.NewBeaconBlockDeneb()
	b2rDeneb, err := b2pbDeneb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), beaconDB, b2pbDeneb)
	require.NoError(t, denebTransitionState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Root: b2rDeneb[:],
	}))

	withdrawals := []*v1.Withdrawal{{
		Index:          1,
		ValidatorIndex: 2,
		Address:        make([]byte, fieldparams.FeeRecipientLength),
		Amount:         3,
	}}
	id := &v1.PayloadIDBytes{0x1}

	ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadCapella{BlockNumber: 1, Withdrawals: withdrawals})
	require.NoError(t, err)
	vs := &Server{
		ExecutionEngineCaller: &powtesting.EngineClient{
			GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed},
			PayloadIDBytes:     id,
		},
		HeadFetcher:            &blockchainTest.ChainService{State: capellaTransitionState},
		FinalizationFetcher:    &blockchainTest.ChainService{},
		BeaconDB:               beaconDB,
		PayloadIDCache:         cache.NewPayloadIDCache(),
		BlockBuilder:           &builderTest.MockBuilderService{HasConfigured: true, Cfg: &builderTest.Config{BeaconDB: beaconDB}},
		ForkchoiceFetcher:      &blockchainTest.ChainService{},
		TrackedValidatorsCache: cache.NewTrackedValidatorsCache(),
	}
	gasLimit := uint64(30000000)
	t.Run("No builder configured. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.IsNil(t, builderBid)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block
	})
	t.Run("Builder configured. Builder Block has higher value. Incorrect withdrawals", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		bid := &ethpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BlockNumber:      2,
				Timestamp:        uint64(ti.Unix()),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo([]byte{1}, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkchoiceFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain
		b := blk.Block()

		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block because incorrect withdrawals
	})
	t.Run("Builder configured. Builder Block has higher value. Correct withdrawals.", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())
		bid := &ethpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BlockNumber:      2,
				Timestamp:        uint64(ti.Unix()),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				WithdrawalsRoot:  wr[:],
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo(builderValue, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(2), e.BlockNumber()) // Builder block
	})
	t.Run("Max builder boost factor should return builder", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())
		bid := &ethpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  wr[:],
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo(builderValue, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, math.MaxUint64)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(2), e.BlockNumber()) // builder block
	})
	t.Run("Builder builder has higher value but forced to local payload with builder boost factor", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())
		bid := &ethpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  wr[:],
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo(builderValue, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, 0)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // local block
	})
	t.Run("Builder configured. Local block has higher value", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		elBid := primitives.Uint64ToWei(2 * 1e9)
		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadCapella{BlockNumber: 3})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed, Bid: elBid}}
		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "\"Proposer: using local execution payload because min difference with local value was not attained\" builderGweiValue=1 localGweiValue=2")
	})
	t.Run("Builder configured. Builder block does not achieve min bid", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.MinBuilderBid = 5
		params.OverrideBeaconConfig(cfg)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		elBid := primitives.Uint64ToWei(2 * 1e9)
		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadCapella{BlockNumber: 3})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed, Bid: elBid}}
		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "\"Proposer: using local execution payload because min bid not attained\" builderGweiValue=1 minBuilderBid=5")
		cfg.MinBuilderBid = 0
		params.OverrideBeaconConfig(cfg)
	})
	t.Run("Builder configured. Local block and local boost has higher value", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.LocalBlockValueBoost = 1 // Boost 1%.
		params.OverrideBeaconConfig(cfg)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		elBid := primitives.Uint64ToWei(1 * 1e9)
		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadCapella{BlockNumber: 3})
		require.NoError(t, err)

		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed, Bid: elBid}}
		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		_, err = builderBid.Header()
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "builderGweiValue=1 localBoostPercentage=1 localGweiValue=1")
	})
	t.Run("Builder configured. Builder returns fault. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		vs.BlockBuilder = &builderTest.MockBuilderService{
			ErrGetHeader:  errors.New("fault"),
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadCapella{BlockNumber: 4})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed}}
		b := blk.Block()
		res, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.ErrorIs(t, consensus_types.ErrNilObjectWrapped, err) // Builder returns fault. Use local block
		require.IsNil(t, builderBid)
		_, bundle, err := setExecutionData(t.Context(), blk, res, nil, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(4), e.BlockNumber()) // Local block
	})
	t.Run("Can get local payload and blobs Deneb", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
		blk.SetSlot(1)
		require.NoError(t, err)
		vs.BlockBuilder = &builderTest.MockBuilderService{
			HasConfigured: false,
		}
		blobsBundle := &v1.BlobsBundle{
			KzgCommitments: [][]byte{{1, 2, 3}},
			Proofs:         [][]byte{{4, 5, 6}},
			Blobs:          [][]byte{{7, 8, 9}},
		}
		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadDeneb{BlockNumber: 4})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{
			PayloadIDBytes: id,
			GetPayloadResponse: &blocks.GetPayloadResponse{
				ExecutionData: ed,
				BlobsBundler:  blobsBundle,
				Bid:           primitives.ZeroWei(),
			},
		}
		blk.SetSlot(primitives.Slot(params.BeaconConfig().DenebForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
		res, err := vs.getLocalPayload(ctx, blk.Block(), capellaTransitionState)
		require.NoError(t, err)
		require.Equal(t, uint64(4), res.ExecutionData.BlockNumber())
		require.DeepEqual(t, res.BlobsBundler, blobsBundle)
	})
	t.Run("Can get builder payload and blobs in Deneb", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
		require.NoError(t, err)
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())

		bid := &ethpb.BuilderBidDeneb{
			Header: &v1.ExecutionPayloadHeaderDeneb{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  wr[:],
				BlobGasUsed:      123,
				ExcessBlobGas:    456,
				GasLimit:         gasLimit,
			},
			Pubkey:             sk.PublicKey().Marshal(),
			Value:              bytesutil.PadTo(builderValue, 32),
			BlobKzgCommitments: [][]byte{bytesutil.PadTo([]byte{2}, fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte{5}, fieldparams.BLSPubkeyLength)},
		}

		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidDeneb{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidDeneb:      sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		require.NoError(t, beaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))

		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadDeneb{BlockNumber: 4, Withdrawals: withdrawals})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{
			PayloadIDBytes:     id,
			GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed},
		}

		require.NoError(t, err)
		blk.SetSlot(primitives.Slot(params.BeaconConfig().DenebForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, blk.Block().Slot(), blk.Block().ProposerIndex(), gasLimit)
		require.NoError(t, err)
		builderPayload, err := builderBid.Header()
		require.NoError(t, err)
		dbid, ok := builderBid.(builder.BidDeneb)
		require.Equal(t, true, ok)
		builderKzgCommitments := dbid.BlobKzgCommitments()
		require.DeepEqual(t, bid.BlobKzgCommitments, builderKzgCommitments)
		require.Equal(t, bid.Header.BlockNumber, builderPayload.BlockNumber()) // header should be the same from block

		res, err := vs.getLocalPayload(ctx, blk.Block(), denebTransitionState)
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)

		got, err := blk.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.DeepEqual(t, bid.BlobKzgCommitments, got)
	})
	t.Run("Can get builder payload, blobs, and execution requests Electra", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
		ti, err := slots.StartTime(time.Now(), 0)
		require.NoError(t, err)
		sk, err := bls.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())

		requests := &v1.ExecutionRequests{
			Deposits: []*v1.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*v1.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*v1.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}

		bid := &ethpb.BuilderBidElectra{
			Header: &v1.ExecutionPayloadHeaderDeneb{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  wr[:],
				BlobGasUsed:      123,
				ExcessBlobGas:    456,
				GasLimit:         gasLimit,
			},
			Pubkey:             sk.PublicKey().Marshal(),
			Value:              bytesutil.PadTo(builderValue, 32),
			BlobKzgCommitments: [][]byte{bytesutil.PadTo([]byte{2}, fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte{5}, fieldparams.BLSPubkeyLength)},
			ExecutionRequests:  requests,
		}

		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &ethpb.SignedBuilderBidElectra{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidElectra:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		require.NoError(t, beaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*ethpb.ValidatorRegistrationV1{{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Timestamp:    uint64(time.Now().Unix()),
				GasLimit:     gasLimit,
				Pubkey:       make([]byte, fieldparams.BLSPubkeyLength)}}))
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(time.Now())
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		ed, err := blocks.NewWrappedExecutionData(&v1.ExecutionPayloadDeneb{BlockNumber: 4, Withdrawals: withdrawals})
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{
			PayloadIDBytes:     id,
			GetPayloadResponse: &blocks.GetPayloadResponse{ExecutionData: ed},
		}

		require.NoError(t, err)
		blk.SetSlot(0)
		require.NoError(t, err)
		builderBid, err := vs.getBuilderPayloadAndBlobs(ctx, blk.Block().Slot(), blk.Block().ProposerIndex(), gasLimit)
		require.NoError(t, err)
		builderPayload, err := builderBid.Header()
		require.NoError(t, err)
		eBid, ok := builderBid.(builder.BidElectra)
		require.Equal(t, true, ok)
		require.DeepEqual(t, bid.BlobKzgCommitments, eBid.BlobKzgCommitments())
		require.DeepEqual(t, bid.ExecutionRequests, eBid.ExecutionRequests())
		require.Equal(t, bid.Header.BlockNumber, builderPayload.BlockNumber()) // header should be the same from block

		res, err := vs.getLocalPayload(ctx, blk.Block(), denebTransitionState)
		require.NoError(t, err)
		_, bundle, err := setExecutionData(t.Context(), blk, res, builderBid, defaultBuilderBoostFactor)
		require.NoError(t, err)
		require.IsNil(t, bundle)

		got, err := blk.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.DeepEqual(t, bid.BlobKzgCommitments, got)

		gRequests, err := blk.Block().Body().ExecutionRequests()
		require.NoError(t, err)
		require.DeepEqual(t, bid.ExecutionRequests, gRequests)
	})
}

func TestServer_getPayloadHeader(t *testing.T) {
	genesis := time.Now().Add(-time.Duration(params.BeaconConfig().SlotsPerEpoch) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	params.SetupTestConfigCleanup(t)
	bc := params.BeaconConfig()
	bc.BellatrixForkEpoch = 1
	params.OverrideBeaconConfig(bc)
	fakeCapellaEpoch := primitives.Epoch(10)
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkVersion = []byte{'A', 'B', 'C', 'Z'}
	cfg.CapellaForkEpoch = fakeCapellaEpoch
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)
	emptyRoot, err := ssz.TransactionsRoot([][]byte{})
	require.NoError(t, err)
	ti, err := slots.StartTime(time.Now(), 0)
	require.NoError(t, err)

	sk, err := bls.RandKey()
	require.NoError(t, err)

	gasLimit := uint64(30000000)
	bid := &ethpb.BuilderBid{
		Header: &v1.ExecutionPayloadHeader{
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			ParentHash:       params.BeaconConfig().ZeroHash[:],
			Timestamp:        uint64(ti.Unix()),
			GasLimit:         gasLimit,
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	d := params.BeaconConfig().DomainApplicationBuilder
	domain, err := signing.ComputeDomain(d, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(bid, domain)
	require.NoError(t, err)
	sBid := &ethpb.SignedBuilderBid{
		Message:   bid,
		Signature: sk.Sign(sr[:]).Marshal(),
	}
	withdrawals := []*v1.Withdrawal{{
		Index:          1,
		ValidatorIndex: 2,
		Address:        make([]byte, fieldparams.FeeRecipientLength),
		Amount:         3,
	}}
	wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)

	tiCapella, err := slots.StartTime(genesis, primitives.Slot(fakeCapellaEpoch)*params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)
	bidCapella := &ethpb.BuilderBidCapella{
		Header: &v1.ExecutionPayloadHeaderCapella{
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			ParentHash:       params.BeaconConfig().ZeroHash[:],
			Timestamp:        uint64(tiCapella.Unix()),
			WithdrawalsRoot:  wr[:],
			GasLimit:         gasLimit,
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	srCapella, err := signing.ComputeSigningRoot(bidCapella, domain)
	require.NoError(t, err)
	sBidCapella := &ethpb.SignedBuilderBidCapella{
		Message:   bidCapella,
		Signature: sk.Sign(srCapella[:]).Marshal(),
	}

	incorrectGasLimitBid := &ethpb.BuilderBid{
		Header: &v1.ExecutionPayloadHeader{
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			ParentHash:       params.BeaconConfig().ZeroHash[:],
			Timestamp:        uint64(tiCapella.Unix()),
			GasLimit:         31000000,
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	signedIncorrectGasLimitBid :=
		&ethpb.SignedBuilderBid{
			Message:   incorrectGasLimitBid,
			Signature: sk.Sign(srCapella[:]).Marshal(),
		}

	tests := []struct {
		name                  string
		head                  interfaces.ReadOnlySignedBeaconBlock
		mock                  *builderTest.MockBuilderService
		fetcher               *blockchainTest.ChainService
		err                   string
		returnedHeader        *v1.ExecutionPayloadHeader
		returnedHeaderCapella *v1.ExecutionPayloadHeaderCapella
		forkVersion           int
	}{
		{
			name: "can't request before bellatrix epoch",
			mock: &builderTest.MockBuilderService{},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
					require.NoError(t, err)
					return wb
				}(),
			},
			err: "can't get payload header from builder before bellatrix epoch",
		},
		{
			name: "get header failed",
			mock: &builderTest.MockBuilderService{
				ErrGetHeader: errors.New("can't get header"),
				Bid:          sBid,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			err: "can't get header",
		},
		{
			name: "0 bid",
			mock: &builderTest.MockBuilderService{
				Bid: &ethpb.SignedBuilderBid{
					Message: &ethpb.BuilderBid{
						Header: &v1.ExecutionPayloadHeader{
							BlockNumber: 123,
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			err: "builder returned header with 0 bid amount",
		},
		{
			name: "invalid tx root",
			mock: &builderTest.MockBuilderService{
				Bid: &ethpb.SignedBuilderBid{
					Message: &ethpb.BuilderBid{
						Value: []byte{1},
						Header: &v1.ExecutionPayloadHeader{
							BlockNumber:      123,
							TransactionsRoot: emptyRoot[:],
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			err: "builder returned header with an empty tx root",
		},
		{
			name: "can get header",
			mock: &builderTest.MockBuilderService{
				Bid: sBid,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			returnedHeader: bid.Header,
		},
		{
			name: "wrong bid version",
			mock: &builderTest.MockBuilderService{
				BidCapella: sBidCapella,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			err: "builder bid response version: 3 is not compatible with expected version: 2 for epoch 1",
		},
		{
			name: "different bid version during hard fork",
			mock: &builderTest.MockBuilderService{
				BidCapella: sBidCapella,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(fakeCapellaEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			returnedHeaderCapella: bidCapella.Header,
		},
		{
			name: "incorrect gas limit",
			mock: &builderTest.MockBuilderService{
				Bid: signedIncorrectGasLimitBid,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			err: "incorrect header gas limit 30000000 != 31000000",
		},
		{
			name: "electra bid with fulu head block - compatible",
			mock: func() *builderTest.MockBuilderService {
				// Create Electra bid
				requests := &v1.ExecutionRequests{
					Deposits: []*v1.DepositRequest{
						{
							Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
							WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
							Amount:                params.BeaconConfig().MinActivationBalance,
							Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
							Index:                 0,
						},
					},
					Withdrawals: []*v1.WithdrawalRequest{
						{
							SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
							ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
							Amount:          params.BeaconConfig().MinActivationBalance,
						},
					},
					Consolidations: []*v1.ConsolidationRequest{
						{
							SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
							SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
							TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
						},
					},
				}

				electraBid := &ethpb.BuilderBidElectra{
					Header: &v1.ExecutionPayloadHeaderDeneb{
						FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
						StateRoot:        make([]byte, fieldparams.RootLength),
						ReceiptsRoot:     make([]byte, fieldparams.RootLength),
						LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
						PrevRandao:       make([]byte, fieldparams.RootLength),
						BaseFeePerGas:    make([]byte, fieldparams.RootLength),
						BlockHash:        make([]byte, fieldparams.RootLength),
						TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
						ParentHash:       params.BeaconConfig().ZeroHash[:],
						Timestamp:        uint64(ti.Unix()),
						BlockNumber:      2,
						WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
						BlobGasUsed:      123,
						ExcessBlobGas:    456,
						GasLimit:         gasLimit,
					},
					Pubkey:             sk.PublicKey().Marshal(),
					Value:              bytesutil.PadTo([]byte{1, 2, 3}, 32),
					BlobKzgCommitments: [][]byte{bytesutil.PadTo([]byte{2}, fieldparams.BLSPubkeyLength)},
					ExecutionRequests:  requests,
				}

				d := params.BeaconConfig().DomainApplicationBuilder
				domain, err := signing.ComputeDomain(d, nil, nil)
				require.NoError(t, err)
				sr, err := signing.ComputeSigningRoot(electraBid, domain)
				require.NoError(t, err)

				sBidElectra := &ethpb.SignedBuilderBidElectra{
					Message:   electraBid,
					Signature: sk.Sign(sr[:]).Marshal(),
				}

				return &builderTest.MockBuilderService{
					BidElectra: sBidElectra,
				}
			}(),
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					// Create Fulu head block
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockFulu())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			// Should succeed because Electra bids are compatible with Fulu head blocks
			forkVersion: version.Fulu,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.forkVersion != 0 {
				params.SetupTestConfigCleanup(t)
				cfg := params.BeaconConfig()
				if tc.forkVersion == version.Fulu {
					cfg.FuluForkEpoch = 0
					cfg.BellatrixForkEpoch = 0
					cfg.InitializeForkSchedule()
				}
				params.OverrideBeaconConfig(cfg)
			}
			vs := &Server{BeaconDB: dbTest.SetupDB(t), BlockBuilder: tc.mock, HeadFetcher: tc.fetcher, TimeFetcher: &blockchainTest.ChainService{
				Genesis: genesis,
			}}
			regCache := cache.NewRegistrationCache()
			regCache.UpdateIndexToRegisteredMap(t.Context(), map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1{
				0: {
					GasLimit:     gasLimit,
					FeeRecipient: make([]byte, 20),
					Pubkey:       make([]byte, 48),
				},
			})
			tc.mock.RegistrationCache = regCache
			hb, err := vs.HeadFetcher.HeadBlock(t.Context())
			require.NoError(t, err)
			bid, err := vs.getPayloadHeaderFromBuilder(t.Context(), hb.Block().Slot(), 0, 30000000)
			if tc.err != "" {
				require.ErrorContains(t, tc.err, err)
			} else {
				require.NoError(t, err)
				h, err := bid.Header()
				require.NoError(t, err)
				if tc.returnedHeader != nil {
					want, err := blocks.WrappedExecutionPayloadHeader(tc.returnedHeader)
					require.NoError(t, err)
					require.DeepEqual(t, want, h)
				}
				if tc.returnedHeaderCapella != nil {
					want, err := blocks.WrappedExecutionPayloadHeaderCapella(tc.returnedHeaderCapella) // value is a mock
					require.NoError(t, err)
					require.DeepEqual(t, want, h)
				}
			}
		})
	}
}

func TestServer_validateBuilderSignature(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	bid := &ethpb.BuilderBid{
		Header: &v1.ExecutionPayloadHeader{
			ParentHash:       make([]byte, fieldparams.RootLength),
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			BlockNumber:      1,
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	d := params.BeaconConfig().DomainApplicationBuilder
	domain, err := signing.ComputeDomain(d, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(bid, domain)
	require.NoError(t, err)
	pbBid := &ethpb.SignedBuilderBid{
		Message:   bid,
		Signature: sk.Sign(sr[:]).Marshal(),
	}
	sBid, err := builder.WrappedSignedBuilderBid(pbBid)
	require.NoError(t, err)
	require.NoError(t, validateBuilderSignature(sBid))

	pbBid.Message.Value = make([]byte, 32)
	sBid, err = builder.WrappedSignedBuilderBid(pbBid)
	require.NoError(t, err)
	require.ErrorIs(t, validateBuilderSignature(sBid), signing.ErrSigFailedToVerify)
}

func Test_matchingWithdrawalsRoot(t *testing.T) {
	t.Run("could not get local withdrawals", func(t *testing.T) {
		local := &v1.ExecutionPayload{}
		p, err := blocks.WrappedExecutionPayload(local)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(p, p)
		require.ErrorContains(t, "could not get local withdrawals", err)
	})
	t.Run("could not get builder withdrawals root", func(t *testing.T) {
		local := &v1.ExecutionPayloadCapella{}
		p, err := blocks.WrappedExecutionPayloadCapella(local)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeader{}
		h, err := blocks.WrappedExecutionPayloadHeader(header)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(p, h)
		require.ErrorContains(t, "could not get builder withdrawals root", err)
	})
	t.Run("withdrawals mismatch", func(t *testing.T) {
		local := &v1.ExecutionPayloadCapella{}
		p, err := blocks.WrappedExecutionPayloadCapella(local)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderCapella{}
		h, err := blocks.WrappedExecutionPayloadHeaderCapella(header)
		require.NoError(t, err)
		matched, err := matchingWithdrawalsRoot(p, h)
		require.NoError(t, err)
		require.Equal(t, false, matched)
	})
	t.Run("withdrawals match", func(t *testing.T) {
		wds := []*v1.Withdrawal{{
			Index:          1,
			ValidatorIndex: 2,
			Address:        make([]byte, fieldparams.FeeRecipientLength),
			Amount:         3,
		}}
		local := &v1.ExecutionPayloadCapella{Withdrawals: wds}
		p, err := blocks.WrappedExecutionPayloadCapella(local)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderCapella{}
		wr, err := ssz.WithdrawalSliceRoot(wds, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		header.WithdrawalsRoot = wr[:]
		h, err := blocks.WrappedExecutionPayloadHeaderCapella(header)
		require.NoError(t, err)
		matched, err := matchingWithdrawalsRoot(p, h)
		require.NoError(t, err)
		require.Equal(t, true, matched)
	})
}

func TestEmptyTransactionsRoot(t *testing.T) {
	r, err := ssz.TransactionsRoot([][]byte{})
	require.NoError(t, err)
	require.DeepEqual(t, r, emptyTransactionsRoot)
}

func Test_expectedGasLimit(t *testing.T) {
	type args struct {
		parentGasLimit uint64
		targetGasLimit uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "Increase within limit",
			args: args{
				parentGasLimit: 15000000,
				targetGasLimit: 15000100,
			},
			want: 15000100,
		},
		{
			name: "Increase exceeding limit",
			args: args{
				parentGasLimit: 15000000,
				targetGasLimit: 16000000,
			},
			want: 15014647, // maxGasLimitDiff = (15000000 / 1024) - 1 = 1464
		},
		{
			name: "Decrease within limit",
			args: args{
				parentGasLimit: 15000000,
				targetGasLimit: 14999990,
			},
			want: 14999990,
		},
		{
			name: "Decrease exceeding limit",
			args: args{
				parentGasLimit: 15000000,
				targetGasLimit: 14000000,
			},
			want: 14985353, // maxGasLimitDiff = (15000000 / 1024) - 1 = 1464
		},
		{
			name: "Target equals parent",
			args: args{
				parentGasLimit: 15000000,
				targetGasLimit: 15000000,
			},
			want: 15000000, // No change
		},
		{
			name: "Very small parent gas limit",
			args: args{
				parentGasLimit: 1025,
				targetGasLimit: 2000,
			},
			want: 1025 + ((1025 / 1024) - 1),
		},
		{
			name: "Target far below parent but limited",
			args: args{
				parentGasLimit: 20000000,
				targetGasLimit: 10000000,
			},
			want: 19980470, // maxGasLimitDiff = (20000000 / 1024) - 1
		},
		{
			name: "Parent gas limit under flows",
			args: args{
				parentGasLimit: 1023,
				targetGasLimit: 30000000,
			},
			want: 1023,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expectedGasLimit(tt.args.parentGasLimit, tt.args.targetGasLimit); got != tt.want {
				t.Errorf("expectedGasLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsVersionCompatible(t *testing.T) {
	tests := []struct {
		name               string
		bidVersion         int
		currentForkVersion int
		want               bool
	}{
		{
			name:               "Exact version match - Bellatrix",
			bidVersion:         version.Bellatrix,
			currentForkVersion: version.Bellatrix,
			want:               true,
		},
		{
			name:               "Exact version match - Capella",
			bidVersion:         version.Capella,
			currentForkVersion: version.Capella,
			want:               true,
		},
		{
			name:               "Exact version match - Deneb",
			bidVersion:         version.Deneb,
			currentForkVersion: version.Deneb,
			want:               true,
		},
		{
			name:               "Exact version match - Electra",
			bidVersion:         version.Electra,
			currentForkVersion: version.Electra,
			want:               true,
		},
		{
			name:               "Exact version match - Fulu",
			bidVersion:         version.Fulu,
			currentForkVersion: version.Fulu,
			want:               true,
		},
		{
			name:               "Electra bid with Fulu head block - Compatible",
			bidVersion:         version.Electra,
			currentForkVersion: version.Fulu,
			want:               true,
		},
		{
			name:               "Fulu bid with Electra head block - Not compatible",
			bidVersion:         version.Fulu,
			currentForkVersion: version.Electra,
			want:               false,
		},
		{
			name:               "Deneb bid with Electra head block - Not compatible",
			bidVersion:         version.Deneb,
			currentForkVersion: version.Electra,
			want:               false,
		},
		{
			name:               "Electra bid with Deneb head block - Not compatible",
			bidVersion:         version.Electra,
			currentForkVersion: version.Deneb,
			want:               false,
		},
		{
			name:               "Capella bid with Deneb head block - Not compatible",
			bidVersion:         version.Capella,
			currentForkVersion: version.Deneb,
			want:               false,
		},
		{
			name:               "Bellatrix bid with Capella head block - Not compatible",
			bidVersion:         version.Bellatrix,
			currentForkVersion: version.Capella,
			want:               false,
		},
		{
			name:               "Phase0 bid with Altair head block - Not compatible",
			bidVersion:         version.Phase0,
			currentForkVersion: version.Altair,
			want:               false,
		},
		{
			name:               "Deneb bid with Fulu head block - Not compatible",
			bidVersion:         version.Deneb,
			currentForkVersion: version.Fulu,
			want:               false,
		},
		{
			name:               "Capella bid with Fulu head block - Not compatible",
			bidVersion:         version.Capella,
			currentForkVersion: version.Fulu,
			want:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVersionCompatible(tt.bidVersion, tt.currentForkVersion)
			if got != tt.want {
				t.Errorf("isVersionCompatible(%d, %d) = %v, want %v", tt.bidVersion, tt.currentForkVersion, got, tt.want)
			}
		})
	}
}
