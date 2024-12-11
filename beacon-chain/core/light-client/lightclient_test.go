package light_client_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"

	"github.com/pkg/errors"
	lightClient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	v11 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestLightClient_NewLightClientOptimisticUpdateFromBeaconState(t *testing.T) {
	t.Run("Altair", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		update, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock)
		require.NoError(t, err)
		require.NotNil(t, update, "update is nil")
		require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

		l.CheckSyncAggregate(update.SyncAggregate())
		l.CheckAttestedHeader(update.AttestedHeader())
	})

	t.Run("Capella", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestCapella(false)

		update, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock)
		require.NoError(t, err)
		require.NotNil(t, update, "update is nil")

		require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

		l.CheckSyncAggregate(update.SyncAggregate())
		l.CheckAttestedHeader(update.AttestedHeader())
	})

	t.Run("Deneb", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestDeneb(false)

		update, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock)
		require.NoError(t, err)
		require.NotNil(t, update, "update is nil")

		require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

		l.CheckSyncAggregate(update.SyncAggregate())
		l.CheckAttestedHeader(update.AttestedHeader())
	})
}

func TestLightClient_NewLightClientFinalityUpdateFromBeaconState(t *testing.T) {
	t.Run("Altair", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		t.Run("FinalizedBlock Not Nil", func(t *testing.T) {
			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			require.NotNil(t, update, "update is nil")

			require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

			l.CheckSyncAggregate(update.SyncAggregate())
			l.CheckAttestedHeader(update.AttestedHeader())

			finalizedBlockHeader, err := l.FinalizedBlock.Header()
			require.NoError(t, err)

			//zeroHash := params.BeaconConfig().ZeroHash[:]
			require.NotNil(t, update.FinalizedHeader(), "Finalized header is nil")
			require.Equal(t, reflect.TypeOf(update.FinalizedHeader().Proto()), reflect.TypeOf(&pb.LightClientHeaderAltair{}), "Finalized header is not Altair")
			updateFinalizedHeaderBeacon := update.FinalizedHeader().Beacon()
			require.Equal(t, finalizedBlockHeader.Header.Slot, updateFinalizedHeaderBeacon.Slot, "Finalized header slot is not equal")
			require.Equal(t, finalizedBlockHeader.Header.ProposerIndex, updateFinalizedHeaderBeacon.ProposerIndex, "Finalized header proposer index is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.ParentRoot, updateFinalizedHeaderBeacon.ParentRoot, "Finalized header parent root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.StateRoot, updateFinalizedHeaderBeacon.StateRoot, "Finalized header state root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.BodyRoot, updateFinalizedHeaderBeacon.BodyRoot, "Finalized header body root is not equal")
			fb, err := update.FinalityBranch()
			require.NoError(t, err)
			proof, err := l.AttestedState.FinalizedRootProof(l.Ctx)
			require.NoError(t, err)
			for i, leaf := range fb {
				require.DeepSSZEqual(t, proof[i], leaf[:], "Leaf is not equal")
			}
		})
	})

	t.Run("Capella", func(t *testing.T) {

		t.Run("FinalizedBlock Not Nil", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapella(false)
			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			require.NotNil(t, update, "update is nil")

			require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

			l.CheckSyncAggregate(update.SyncAggregate())
			l.CheckAttestedHeader(update.AttestedHeader())

			finalizedBlockHeader, err := l.FinalizedBlock.Header()
			require.NoError(t, err)
			require.NotNil(t, update.FinalizedHeader(), "Finalized header is nil")
			require.Equal(t, reflect.TypeOf(update.FinalizedHeader().Proto()), reflect.TypeOf(&pb.LightClientHeaderCapella{}), "Finalized header is not Capella")
			updateFinalizedHeaderBeacon := update.FinalizedHeader().Beacon()
			require.Equal(t, finalizedBlockHeader.Header.Slot, updateFinalizedHeaderBeacon.Slot, "Finalized header slot is not equal")
			require.Equal(t, finalizedBlockHeader.Header.ProposerIndex, updateFinalizedHeaderBeacon.ProposerIndex, "Finalized header proposer index is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.ParentRoot, updateFinalizedHeaderBeacon.ParentRoot, "Finalized header parent root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.StateRoot, updateFinalizedHeaderBeacon.StateRoot, "Finalized header state root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.BodyRoot, updateFinalizedHeaderBeacon.BodyRoot, "Finalized header body root is not equal")
			fb, err := update.FinalityBranch()
			require.NoError(t, err)
			proof, err := l.AttestedState.FinalizedRootProof(l.Ctx)
			require.NoError(t, err)
			for i, leaf := range fb {
				require.DeepSSZEqual(t, proof[i], leaf[:], "Leaf is not equal")
			}

			// Check Execution BlockHash
			payloadInterface, err := l.FinalizedBlock.Block().Body().Execution()
			require.NoError(t, err)
			transactionsRoot, err := payloadInterface.TransactionsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				transactions, err := payloadInterface.Transactions()
				require.NoError(t, err)
				transactionsRootArray, err := ssz.TransactionsRoot(transactions)
				require.NoError(t, err)
				transactionsRoot = transactionsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			withdrawalsRoot, err := payloadInterface.WithdrawalsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				withdrawals, err := payloadInterface.Withdrawals()
				require.NoError(t, err)
				withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				withdrawalsRoot = withdrawalsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			execution := &v11.ExecutionPayloadHeaderCapella{
				ParentHash:       payloadInterface.ParentHash(),
				FeeRecipient:     payloadInterface.FeeRecipient(),
				StateRoot:        payloadInterface.StateRoot(),
				ReceiptsRoot:     payloadInterface.ReceiptsRoot(),
				LogsBloom:        payloadInterface.LogsBloom(),
				PrevRandao:       payloadInterface.PrevRandao(),
				BlockNumber:      payloadInterface.BlockNumber(),
				GasLimit:         payloadInterface.GasLimit(),
				GasUsed:          payloadInterface.GasUsed(),
				Timestamp:        payloadInterface.Timestamp(),
				ExtraData:        payloadInterface.ExtraData(),
				BaseFeePerGas:    payloadInterface.BaseFeePerGas(),
				BlockHash:        payloadInterface.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}
			updateExecution, err := update.FinalizedHeader().Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, execution, updateExecution.Proto(), "Finalized Block Execution is not equal")
		})

		t.Run("FinalizedBlock In Previous Fork", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapellaFinalizedBlockAltair(false)
			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			require.NotNil(t, update, "update is nil")

			require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

			l.CheckSyncAggregate(update.SyncAggregate())
			l.CheckAttestedHeader(update.AttestedHeader())

			finalizedBlockHeader, err := l.FinalizedBlock.Header()
			require.NoError(t, err)
			require.NotNil(t, update.FinalizedHeader(), "Finalized header is nil")
			require.Equal(t, reflect.TypeOf(update.FinalizedHeader().Proto()), reflect.TypeOf(&pb.LightClientHeaderCapella{}), "Finalized header is not Capella")
			updateFinalizedHeaderBeacon := update.FinalizedHeader().Beacon()
			require.Equal(t, finalizedBlockHeader.Header.Slot, updateFinalizedHeaderBeacon.Slot, "Finalized header slot is not equal")
			require.Equal(t, finalizedBlockHeader.Header.ProposerIndex, updateFinalizedHeaderBeacon.ProposerIndex, "Finalized header proposer index is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.ParentRoot, updateFinalizedHeaderBeacon.ParentRoot, "Finalized header parent root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.StateRoot, updateFinalizedHeaderBeacon.StateRoot, "Finalized header state root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.BodyRoot, updateFinalizedHeaderBeacon.BodyRoot, "Finalized header body root is not equal")
			fb, err := update.FinalityBranch()
			require.NoError(t, err)
			proof, err := l.AttestedState.FinalizedRootProof(l.Ctx)
			require.NoError(t, err)
			for i, leaf := range fb {
				require.DeepSSZEqual(t, proof[i], leaf[:], "Leaf is not equal")
			}
		})
	})

	t.Run("Deneb", func(t *testing.T) {

		t.Run("FinalizedBlock Not Nil", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestDeneb(false)

			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			require.NotNil(t, update, "update is nil")

			require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

			l.CheckSyncAggregate(update.SyncAggregate())
			l.CheckAttestedHeader(update.AttestedHeader())

			//zeroHash := params.BeaconConfig().ZeroHash[:]
			finalizedBlockHeader, err := l.FinalizedBlock.Header()
			require.NoError(t, err)
			require.NotNil(t, update.FinalizedHeader(), "Finalized header is nil")
			updateFinalizedHeaderBeacon := update.FinalizedHeader().Beacon()
			require.Equal(t, finalizedBlockHeader.Header.Slot, updateFinalizedHeaderBeacon.Slot, "Finalized header slot is not equal")
			require.Equal(t, finalizedBlockHeader.Header.ProposerIndex, updateFinalizedHeaderBeacon.ProposerIndex, "Finalized header proposer index is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.ParentRoot, updateFinalizedHeaderBeacon.ParentRoot, "Finalized header parent root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.StateRoot, updateFinalizedHeaderBeacon.StateRoot, "Finalized header state root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.BodyRoot, updateFinalizedHeaderBeacon.BodyRoot, "Finalized header body root is not equal")
			fb, err := update.FinalityBranch()
			require.NoError(t, err)
			proof, err := l.AttestedState.FinalizedRootProof(l.Ctx)
			require.NoError(t, err)
			for i, leaf := range fb {
				require.DeepSSZEqual(t, proof[i], leaf[:], "Leaf is not equal")
			}

			// Check Execution BlockHash
			payloadInterface, err := l.FinalizedBlock.Block().Body().Execution()
			require.NoError(t, err)
			transactionsRoot, err := payloadInterface.TransactionsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				transactions, err := payloadInterface.Transactions()
				require.NoError(t, err)
				transactionsRootArray, err := ssz.TransactionsRoot(transactions)
				require.NoError(t, err)
				transactionsRoot = transactionsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			withdrawalsRoot, err := payloadInterface.WithdrawalsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				withdrawals, err := payloadInterface.Withdrawals()
				require.NoError(t, err)
				withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				withdrawalsRoot = withdrawalsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			execution := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payloadInterface.ParentHash(),
				FeeRecipient:     payloadInterface.FeeRecipient(),
				StateRoot:        payloadInterface.StateRoot(),
				ReceiptsRoot:     payloadInterface.ReceiptsRoot(),
				LogsBloom:        payloadInterface.LogsBloom(),
				PrevRandao:       payloadInterface.PrevRandao(),
				BlockNumber:      payloadInterface.BlockNumber(),
				GasLimit:         payloadInterface.GasLimit(),
				GasUsed:          payloadInterface.GasUsed(),
				Timestamp:        payloadInterface.Timestamp(),
				ExtraData:        payloadInterface.ExtraData(),
				BaseFeePerGas:    payloadInterface.BaseFeePerGas(),
				BlockHash:        payloadInterface.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}
			updateExecution, err := update.FinalizedHeader().Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, execution, updateExecution.Proto(), "Finalized Block Execution is not equal")
		})

		t.Run("FinalizedBlock In Previous Fork", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestDenebFinalizedBlockCapella(false)

			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			require.NotNil(t, update, "update is nil")

			require.Equal(t, l.Block.Block().Slot(), update.SignatureSlot(), "Signature slot is not equal")

			l.CheckSyncAggregate(update.SyncAggregate())
			l.CheckAttestedHeader(update.AttestedHeader())

			finalizedBlockHeader, err := l.FinalizedBlock.Header()
			require.NoError(t, err)
			require.NotNil(t, update.FinalizedHeader(), "Finalized header is nil")
			updateFinalizedHeaderBeacon := update.FinalizedHeader().Beacon()
			require.Equal(t, reflect.TypeOf(update.FinalizedHeader().Proto()), reflect.TypeOf(&pb.LightClientHeaderDeneb{}), "Finalized header is not Deneb")
			require.Equal(t, finalizedBlockHeader.Header.Slot, updateFinalizedHeaderBeacon.Slot, "Finalized header slot is not equal")
			require.Equal(t, finalizedBlockHeader.Header.ProposerIndex, updateFinalizedHeaderBeacon.ProposerIndex, "Finalized header proposer index is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.ParentRoot, updateFinalizedHeaderBeacon.ParentRoot, "Finalized header parent root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.StateRoot, updateFinalizedHeaderBeacon.StateRoot, "Finalized header state root is not equal")
			require.DeepSSZEqual(t, finalizedBlockHeader.Header.BodyRoot, updateFinalizedHeaderBeacon.BodyRoot, "Finalized header body root is not equal")
			fb, err := update.FinalityBranch()
			require.NoError(t, err)
			proof, err := l.AttestedState.FinalizedRootProof(l.Ctx)
			require.NoError(t, err)
			for i, leaf := range fb {
				require.DeepSSZEqual(t, proof[i], leaf[:], "Leaf is not equal")
			}

			// Check Execution BlockHash
			payloadInterface, err := l.FinalizedBlock.Block().Body().Execution()
			require.NoError(t, err)
			transactionsRoot, err := payloadInterface.TransactionsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				transactions, err := payloadInterface.Transactions()
				require.NoError(t, err)
				transactionsRootArray, err := ssz.TransactionsRoot(transactions)
				require.NoError(t, err)
				transactionsRoot = transactionsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			withdrawalsRoot, err := payloadInterface.WithdrawalsRoot()
			if errors.Is(err, consensustypes.ErrUnsupportedField) {
				withdrawals, err := payloadInterface.Withdrawals()
				require.NoError(t, err)
				withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				withdrawalsRoot = withdrawalsRootArray[:]
			} else {
				require.NoError(t, err)
			}
			execution := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payloadInterface.ParentHash(),
				FeeRecipient:     payloadInterface.FeeRecipient(),
				StateRoot:        payloadInterface.StateRoot(),
				ReceiptsRoot:     payloadInterface.ReceiptsRoot(),
				LogsBloom:        payloadInterface.LogsBloom(),
				PrevRandao:       payloadInterface.PrevRandao(),
				BlockNumber:      payloadInterface.BlockNumber(),
				GasLimit:         payloadInterface.GasLimit(),
				GasUsed:          payloadInterface.GasUsed(),
				Timestamp:        payloadInterface.Timestamp(),
				ExtraData:        payloadInterface.ExtraData(),
				BaseFeePerGas:    payloadInterface.BaseFeePerGas(),
				BlockHash:        payloadInterface.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}
			updateExecution, err := update.FinalizedHeader().Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, execution, updateExecution.Proto(), "Finalized Block Execution is not equal")
		})
	})
}

func TestLightClient_BlockToLightClientHeader(t *testing.T) {
	t.Run("Altair", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		header, err := lightClient.BlockToLightClientHeader(
			l.Ctx,
			primitives.Slot(params.BeaconConfig().AltairForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
			l.Block,
		)
		require.NoError(t, err)
		require.NotNil(t, header, "header is nil")

		parentRoot := l.Block.Block().ParentRoot()
		stateRoot := l.Block.Block().StateRoot()
		bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
		require.NoError(t, err)

		require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
		require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
		require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
		require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
		require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")
	})

	t.Run("Bellatrix", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestBellatrix()

		header, err := lightClient.BlockToLightClientHeader(
			l.Ctx,
			primitives.Slot(params.BeaconConfig().BellatrixForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
			l.Block,
		)
		require.NoError(t, err)
		require.NotNil(t, header, "header is nil")

		parentRoot := l.Block.Block().ParentRoot()
		stateRoot := l.Block.Block().StateRoot()
		bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
		require.NoError(t, err)

		require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
		require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
		require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
		require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
		require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")
	})

	t.Run("Capella", func(t *testing.T) {
		t.Run("Non-Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapella(false)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().CapellaForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block,
			)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := lightClient.ComputeTransactionsRoot(payload)
			require.NoError(t, err)

			withdrawalsRoot, err := lightClient.ComputeWithdrawalsRoot(payload)
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderCapella{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})

		t.Run("Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapella(true)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().CapellaForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block,
			)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := payload.TransactionsRoot()
			require.NoError(t, err)

			withdrawalsRoot, err := payload.WithdrawalsRoot()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderCapella{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})
	})

	t.Run("Deneb", func(t *testing.T) {
		t.Run("Non-Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestDeneb(false)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block,
			)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := lightClient.ComputeTransactionsRoot(payload)
			require.NoError(t, err)

			withdrawalsRoot, err := lightClient.ComputeWithdrawalsRoot(payload)
			require.NoError(t, err)

			blobGasUsed, err := payload.BlobGasUsed()
			require.NoError(t, err)

			excessBlobGas, err := payload.ExcessBlobGas()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
				BlobGasUsed:      blobGasUsed,
				ExcessBlobGas:    excessBlobGas,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})

		t.Run("Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestDeneb(true)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block,
			)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := payload.TransactionsRoot()
			require.NoError(t, err)

			withdrawalsRoot, err := payload.WithdrawalsRoot()
			require.NoError(t, err)

			blobGasUsed, err := payload.BlobGasUsed()
			require.NoError(t, err)

			excessBlobGas, err := payload.ExcessBlobGas()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
				BlobGasUsed:      blobGasUsed,
				ExcessBlobGas:    excessBlobGas,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})
	})

	t.Run("Electra", func(t *testing.T) {
		t.Run("Non-Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestElectra(false)

			header, err := lightClient.BlockToLightClientHeader(l.Ctx, l.State.Slot(), l.Block)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := lightClient.ComputeTransactionsRoot(payload)
			require.NoError(t, err)

			withdrawalsRoot, err := lightClient.ComputeWithdrawalsRoot(payload)
			require.NoError(t, err)

			blobGasUsed, err := payload.BlobGasUsed()
			require.NoError(t, err)

			excessBlobGas, err := payload.ExcessBlobGas()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderElectra{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
				BlobGasUsed:      blobGasUsed,
				ExcessBlobGas:    excessBlobGas,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})

		t.Run("Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestElectra(true)

			header, err := lightClient.BlockToLightClientHeader(l.Ctx, l.State.Slot(), l.Block)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := payload.TransactionsRoot()
			require.NoError(t, err)

			withdrawalsRoot, err := payload.WithdrawalsRoot()
			require.NoError(t, err)

			blobGasUsed, err := payload.BlobGasUsed()
			require.NoError(t, err)

			excessBlobGas, err := payload.ExcessBlobGas()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderElectra{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
				BlobGasUsed:      blobGasUsed,
				ExcessBlobGas:    excessBlobGas,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})
	})

	t.Run("Capella fork with Altair block", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		header, err := lightClient.BlockToLightClientHeader(
			l.Ctx,
			primitives.Slot(params.BeaconConfig().CapellaForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
			l.Block)
		require.NoError(t, err)
		require.NotNil(t, header, "header is nil")

		parentRoot := l.Block.Block().ParentRoot()
		stateRoot := l.Block.Block().StateRoot()
		bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
		require.NoError(t, err)

		require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
		require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
		require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
		require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
		require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")
	})

	t.Run("Deneb fork with Altair block", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		header, err := lightClient.BlockToLightClientHeader(
			l.Ctx,
			primitives.Slot(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
			l.Block)
		require.NoError(t, err)
		require.NotNil(t, header, "header is nil")

		parentRoot := l.Block.Block().ParentRoot()
		stateRoot := l.Block.Block().StateRoot()
		bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
		require.NoError(t, err)

		require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
		require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
		require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
		require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
		require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")
	})

	t.Run("Deneb fork with Capella block", func(t *testing.T) {
		t.Run("Non-Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapella(false)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := lightClient.ComputeTransactionsRoot(payload)
			require.NoError(t, err)

			withdrawalsRoot, err := lightClient.ComputeWithdrawalsRoot(payload)
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})

		t.Run("Blinded Beacon Block", func(t *testing.T) {
			l := util.NewTestLightClient(t).SetupTestCapella(true)

			header, err := lightClient.BlockToLightClientHeader(
				l.Ctx,
				primitives.Slot(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SlotsPerEpoch,
				l.Block)
			require.NoError(t, err)
			require.NotNil(t, header, "header is nil")

			parentRoot := l.Block.Block().ParentRoot()
			stateRoot := l.Block.Block().StateRoot()
			bodyRoot, err := l.Block.Block().Body().HashTreeRoot()
			require.NoError(t, err)

			payload, err := l.Block.Block().Body().Execution()
			require.NoError(t, err)

			transactionsRoot, err := payload.TransactionsRoot()
			require.NoError(t, err)

			withdrawalsRoot, err := payload.WithdrawalsRoot()
			require.NoError(t, err)

			executionHeader := &v11.ExecutionPayloadHeaderDeneb{
				ParentHash:       payload.ParentHash(),
				FeeRecipient:     payload.FeeRecipient(),
				StateRoot:        payload.StateRoot(),
				ReceiptsRoot:     payload.ReceiptsRoot(),
				LogsBloom:        payload.LogsBloom(),
				PrevRandao:       payload.PrevRandao(),
				BlockNumber:      payload.BlockNumber(),
				GasLimit:         payload.GasLimit(),
				GasUsed:          payload.GasUsed(),
				Timestamp:        payload.Timestamp(),
				ExtraData:        payload.ExtraData(),
				BaseFeePerGas:    payload.BaseFeePerGas(),
				BlockHash:        payload.BlockHash(),
				TransactionsRoot: transactionsRoot,
				WithdrawalsRoot:  withdrawalsRoot,
			}

			executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.Block.Block())
			require.NoError(t, err)

			require.Equal(t, l.Block.Block().Slot(), header.Beacon().Slot, "Slot is not equal")
			require.Equal(t, l.Block.Block().ProposerIndex(), header.Beacon().ProposerIndex, "Proposer index is not equal")
			require.DeepSSZEqual(t, parentRoot[:], header.Beacon().ParentRoot, "Parent root is not equal")
			require.DeepSSZEqual(t, stateRoot[:], header.Beacon().StateRoot, "State root is not equal")
			require.DeepSSZEqual(t, bodyRoot[:], header.Beacon().BodyRoot, "Body root is not equal")

			headerExecution, err := header.Execution()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionHeader, headerExecution.Proto(), "Execution headers are not equal")

			headerExecutionBranch, err := header.ExecutionBranch()
			require.NoError(t, err)
			require.DeepSSZEqual(t, executionPayloadProof, convertArrayToSlice(headerExecutionBranch), "Execution payload proofs are not equal")
		})
	})
}

func convertArrayToSlice(arr [4][32]uint8) [][]uint8 {
	slice := make([][]uint8, len(arr))
	for i := range arr {
		slice[i] = arr[i][:]
	}
	return slice
}

// When the update has relevant sync committee
func createNonEmptySyncCommitteeBranch() [][]byte {
	res := make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	res[0] = []byte(strings.Repeat("x", 32))
	for i := 1; i < len(res); i++ {
		res[i] = make([]byte, fieldparams.RootLength)
	}
	return res
}

// When the update has finality
func createNonEmptyFinalityBranch() [][]byte {
	res := make([][]byte, fieldparams.FinalityBranchDepth)
	res[0] = []byte(strings.Repeat("x", 32))
	for i := 1; i < fieldparams.FinalityBranchDepth; i++ {
		res[i] = make([]byte, 32)
	}
	return res
}

func TestIsBetterUpdate(t *testing.T) {
	config := params.BeaconConfig()
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)

	t.Run("new has supermajority but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b11111100, 0b1}, // [0,0,1,1,1,1,1,1]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("old has supermajority but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b11111100, 0b1}, // [0,0,1,1,1,1,1,1]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new doesn't have supermajority and newNumActiveParticipants is greater than oldNumActiveParticipants", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("new doesn't have supermajority and newNumActiveParticipants is lesser than oldNumActiveParticipants", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new has relevant sync committee but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000001,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(1000000)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("old has relevant sync committee but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000001,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(1000000)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new has finality but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("old has finality but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new has finality and sync committee finality both but old doesn't have sync committee finality", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(999999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("new has finality but doesn't have sync committee finality and old has sync committee finality", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(999999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new has more active participants than old", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,1,1,1,1,1,0,0]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("new has less active participants than old", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,1,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("new's attested header's slot is lesser than old's attested header's slot", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("new's attested header's slot is greater than old's attested header's slot", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})

	t.Run("none of the above conditions are met and new signature's slot is less than old signature's slot", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9998)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("none of the above conditions are met and new signature's slot is greater than old signature's slot", func(t *testing.T) {
		oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		require.NoError(t, err)
		newUpdate, err := lightClient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		require.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		require.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		oldUpdate.SetSignatureSlot(9998)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		require.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		require.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		require.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		require.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		require.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		require.NoError(t, err)

		result, err := lightClient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result)
	})
}
