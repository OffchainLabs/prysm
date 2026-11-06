package structs

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestLightClientConversions_Gloas(t *testing.T) {
	p := &pb.LightClientUpdateGloas{
		AttestedHeader: &pb.LightClientHeaderGloas{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 700, ProposerIndex: 42,
				ParentRoot: bytes.Repeat([]byte{1}, 32),
				StateRoot:  bytes.Repeat([]byte{2}, 32),
				BodyRoot:   bytes.Repeat([]byte{3}, 32),
			},
			ExecutionBlockHash: bytes.Repeat([]byte{4}, 32),
			ExecutionBranch:    lightClientJSONRoots(11),
		},
		NextSyncCommittee: &pb.SyncCommittee{
			Pubkeys:         [][]byte{bytes.Repeat([]byte{5}, 48)},
			AggregatePubkey: bytes.Repeat([]byte{6}, 48),
		},
		NextSyncCommitteeBranch: lightClientJSONRoots(11),
		FinalizedHeader: &pb.LightClientHeaderGloas{
			Beacon:             &pb.BeaconBlockHeader{Slot: 100},
			ExecutionBlockHash: bytes.Repeat([]byte{7}, 32),
			ExecutionBranch:    lightClientJSONRoots(11),
		},
		FinalityBranch: lightClientJSONRoots(9),
		SyncAggregate: &pb.SyncAggregate{
			SyncCommitteeBits:      bytes.Repeat([]byte{8}, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: bytes.Repeat([]byte{9}, 96),
		},
		SignatureSlot: 701,
	}
	checkHeader := func(t *testing.T, raw json.RawMessage, want *pb.LightClientHeaderGloas) {
		t.Helper()
		var got LightClientHeaderGloas
		require.NoError(t, json.Unmarshal(raw, &got))
		require.DeepEqual(t, BeaconBlockHeaderFromConsensus(want.Beacon), got.Beacon)
		require.Equal(t, hexutil.Encode(want.ExecutionBlockHash), got.ExecutionBlockHash)
		checkLightClientJSONBranch(t, want.ExecutionBranch, got.ExecutionBranch)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		require.Equal(t, 3, len(fields))
		require.Equal(t, false, bytes.Contains(raw, []byte(`"execution":`)))
	}
	for _, name := range []string{"finalized header", "empty finalized header"} {
		t.Run(name, func(t *testing.T) {
			if name == "empty finalized header" {
				p.FinalizedHeader = &pb.LightClientHeaderGloas{Beacon: &pb.BeaconBlockHeader{}, ExecutionBlockHash: make([]byte, 32), ExecutionBranch: make([][]byte, 11)}
				for i := range p.FinalizedHeader.ExecutionBranch {
					p.FinalizedHeader.ExecutionBranch[i] = make([]byte, 32)
				}
				for i := range p.FinalityBranch {
					p.FinalityBranch[i] = make([]byte, 32)
				}
			}
			update, err := light_client.NewWrappedUpdate(p)
			require.NoError(t, err)
			got, err := LightClientUpdateFromConsensus(update)
			require.NoError(t, err)
			checkHeader(t, got.AttestedHeader, p.AttestedHeader)
			checkHeader(t, got.FinalizedHeader, p.FinalizedHeader)
			checkLightClientJSONBranch(t, p.NextSyncCommitteeBranch, got.NextSyncCommitteeBranch)
			checkLightClientJSONBranch(t, p.FinalityBranch, got.FinalityBranch)
			require.DeepEqual(t, &SyncCommittee{Pubkeys: []string{hexutil.Encode(p.NextSyncCommittee.Pubkeys[0])}, AggregatePubkey: hexutil.Encode(p.NextSyncCommittee.AggregatePubkey)}, got.NextSyncCommittee)
			require.DeepEqual(t, &SyncAggregate{SyncCommitteeBits: hexutil.Encode(p.SyncAggregate.SyncCommitteeBits), SyncCommitteeSignature: hexutil.Encode(p.SyncAggregate.SyncCommitteeSignature)}, got.SyncAggregate)
			require.Equal(t, "701", got.SignatureSlot)

			finality, err := light_client.NewFinalityUpdateFromUpdate(update)
			require.NoError(t, err)
			gotFinality, err := LightClientFinalityUpdateFromConsensus(finality)
			require.NoError(t, err)
			checkHeader(t, gotFinality.AttestedHeader, p.AttestedHeader)
			checkHeader(t, gotFinality.FinalizedHeader, p.FinalizedHeader)
			checkLightClientJSONBranch(t, p.FinalityBranch, gotFinality.FinalityBranch)
			require.DeepEqual(t, got.SyncAggregate, gotFinality.SyncAggregate)
			require.Equal(t, got.SignatureSlot, gotFinality.SignatureSlot)

			optimistic, err := light_client.NewOptimisticUpdateFromUpdate(update)
			require.NoError(t, err)
			gotOptimistic, err := LightClientOptimisticUpdateFromConsensus(optimistic)
			require.NoError(t, err)
			checkHeader(t, gotOptimistic.AttestedHeader, p.AttestedHeader)
			require.DeepEqual(t, got.SyncAggregate, gotOptimistic.SyncAggregate)
			require.Equal(t, got.SignatureSlot, gotOptimistic.SignatureSlot)

			bootstrap, err := light_client.NewWrappedBootstrap(&pb.LightClientBootstrapGloas{
				Header:                     p.AttestedHeader,
				CurrentSyncCommittee:       p.NextSyncCommittee,
				CurrentSyncCommitteeBranch: p.NextSyncCommitteeBranch,
			})
			require.NoError(t, err)
			gotBootstrap, err := LightClientBootstrapFromConsensus(bootstrap)
			require.NoError(t, err)
			checkHeader(t, gotBootstrap.Header, p.AttestedHeader)
			require.DeepEqual(t, got.NextSyncCommittee, gotBootstrap.CurrentSyncCommittee)
			checkLightClientJSONBranch(t, p.NextSyncCommitteeBranch, gotBootstrap.CurrentSyncCommitteeBranch)
		})
	}
}

func TestLightClientHeaderToJSON(t *testing.T) {
	t.Run("nil finalized header", func(t *testing.T) {
		got, err := lightClientHeaderToJSON(nil)
		require.NoError(t, err)
		require.Equal(t, 0, len(got))
	})
	t.Run("unsupported version", func(t *testing.T) {
		_, err := lightClientHeaderToJSON(&lightClientJSONHeader{fork: version.Phase0})
		require.ErrorContains(t, "unsupported header version", err)
	})
	for _, fork := range []int{version.Altair, version.Capella, version.Deneb, version.Electra, version.Gloas} {
		t.Run(version.String(fork), func(t *testing.T) {
			header := &lightClientJSONHeader{fork: fork}
			switch fork {
			case version.Capella:
				header.execution, _ = blocks.WrappedExecutionPayloadHeaderCapella(&enginev1.ExecutionPayloadHeaderCapella{BlockHash: bytes.Repeat([]byte{42}, 32)})
			case version.Deneb, version.Electra:
				header.execution, _ = blocks.WrappedExecutionPayloadHeaderDeneb(&enginev1.ExecutionPayloadHeaderDeneb{BlockHash: bytes.Repeat([]byte{42}, 32)})
			}
			raw, err := lightClientHeaderToJSON(header)
			require.NoError(t, err)
			var got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &got))
			require.NotNil(t, got["beacon"])
			_, hasExecution := got["execution"]
			_, hasHash := got["execution_block_hash"]
			require.Equal(t, fork >= version.Capella && fork < version.Gloas, hasExecution)
			require.Equal(t, fork == version.Gloas, hasHash)
			if hasExecution {
				var execution struct {
					BlockHash string `json:"block_hash"`
				}
				require.NoError(t, json.Unmarshal(got["execution"], &execution))
				require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{42}, 32)), execution.BlockHash)
			}
			if fork == version.Altair {
				require.Equal(t, 1, len(got))
				return
			}
			for _, accessor := range []string{"execution", "branch"} {
				t.Run(accessor+" error", func(t *testing.T) {
					header.failure = accessor
					_, err := lightClientHeaderToJSON(header)
					require.ErrorContains(t, accessor+" unavailable", err)
					header.failure = ""
				})
			}
			if fork == version.Gloas {
				return
			}
			t.Run("wrong execution type", func(t *testing.T) {
				original := header.execution
				if fork == version.Capella {
					header.execution, _ = blocks.WrappedExecutionPayloadHeaderDeneb(&enginev1.ExecutionPayloadHeaderDeneb{})
				} else {
					header.execution, _ = blocks.WrappedExecutionPayloadHeaderCapella(&enginev1.ExecutionPayloadHeaderCapella{})
				}
				_, err := lightClientHeaderToJSON(header)
				require.ErrorContains(t, "execution data is not", err)
				header.execution = original
			})
			t.Run("invalid base fee", func(t *testing.T) {
				fee := bytes.Repeat([]byte{255}, 33)
				if fork == version.Capella {
					header.execution, _ = blocks.WrappedExecutionPayloadHeaderCapella(&enginev1.ExecutionPayloadHeaderCapella{BaseFeePerGas: fee})
				} else {
					header.execution, _ = blocks.WrappedExecutionPayloadHeaderDeneb(&enginev1.ExecutionPayloadHeaderDeneb{BaseFeePerGas: fee})
				}
				_, err := lightClientHeaderToJSON(header)
				require.ErrorContains(t, "not a valid Uint256", err)
			})
		})
	}
}

type lightClientJSONHeader struct {
	interfaces.LightClientHeader
	fork      int
	execution interfaces.ExecutionData
	failure   string
}

func (h *lightClientJSONHeader) Version() int                { return h.fork }
func (*lightClientJSONHeader) Beacon() *pb.BeaconBlockHeader { return &pb.BeaconBlockHeader{Slot: 123} }
func (h *lightClientJSONHeader) Execution() (interfaces.ExecutionData, error) {
	if h.failure == "execution" {
		return nil, errors.New("execution unavailable")
	}
	return h.execution, nil
}
func (h *lightClientJSONHeader) ExecutionBranch() (interfaces.LightClientExecutionBranch, error) {
	if h.failure == "branch" {
		return interfaces.LightClientExecutionBranch{}, errors.New("branch unavailable")
	}
	return interfaces.LightClientExecutionBranch{}, nil
}
func (h *lightClientJSONHeader) ExecutionBlockHash() ([32]byte, error) {
	if h.failure == "execution" {
		return [32]byte{}, errors.New("execution unavailable")
	}
	return [32]byte{42}, nil
}
func (h *lightClientJSONHeader) ExecutionBranchGloas() (interfaces.LightClientExecutionBranchGloas, error) {
	if h.failure == "branch" {
		return interfaces.LightClientExecutionBranchGloas{}, errors.New("branch unavailable")
	}
	return interfaces.LightClientExecutionBranchGloas{}, nil
}

func lightClientJSONRoots(depth int) [][]byte {
	roots := make([][]byte, depth)
	for i := range roots {
		roots[i] = bytes.Repeat([]byte{byte(i + 1)}, 32)
	}
	return roots
}

func checkLightClientJSONBranch(t *testing.T, want [][]byte, got []string) {
	t.Helper()
	require.Equal(t, len(want), len(got))
	for i := range want {
		require.Equal(t, hexutil.Encode(want[i]), got[i])
	}
}
