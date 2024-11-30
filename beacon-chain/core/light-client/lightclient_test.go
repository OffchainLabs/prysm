package light_client_test

import (
	"reflect"
	"testing"

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

		header, err := lightClient.BlockToLightClientHeader(l.Ctx, l.State.Slot(), l.Block)
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

		header, err := lightClient.BlockToLightClientHeader(l.Ctx, l.State.Slot(), l.Block)
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
}

func convertArrayToSlice(arr [4][32]uint8) [][]uint8 {
	slice := make([][]uint8, len(arr))
	for i := range arr {
		slice[i] = arr[i][:]
	}
	return slice
}
