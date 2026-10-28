package lightclient

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/async/event"
	blockchainTest "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtesting "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	lightclient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	p2ptesting "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestLightClientHandlers_Gloas(t *testing.T) {
	setupLightClientAPIConfig(t)
	for _, fork := range []int{version.Fulu, version.Gloas} {
		t.Run(version.String(fork), func(t *testing.T) {
			update := lightClientAPIUpdate(t, fork)
			finality, err := light_client.NewFinalityUpdateFromUpdate(update)
			require.NoError(t, err)
			optimistic, err := light_client.NewOptimisticUpdateFromUpdate(update)
			require.NoError(t, err)
			var bootstrap interfaces.LightClientBootstrap
			switch p := update.Proto().(type) {
			case *pb.LightClientUpdateGloas:
				bootstrap, err = light_client.NewWrappedBootstrap(&pb.LightClientBootstrapGloas{
					Header:                     p.AttestedHeader,
					CurrentSyncCommittee:       p.NextSyncCommittee,
					CurrentSyncCommitteeBranch: p.NextSyncCommitteeBranch,
				})
			case *pb.LightClientUpdateElectra:
				bootstrap, err = light_client.NewWrappedBootstrap(&pb.LightClientBootstrapElectra{
					Header:                     p.AttestedHeader,
					CurrentSyncCommittee:       p.NextSyncCommittee,
					CurrentSyncCommitteeBranch: p.NextSyncCommitteeBranch,
				})
			}
			require.NoError(t, err)
			db := dbtesting.SetupDB(t)
			store := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)
			root, err := bootstrap.Header().Beacon().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, db.SaveLightClientBootstrap(t.Context(), root[:], bootstrap))
			store.SetLastFinalityUpdate(finality, false)
			store.SetLastOptimisticUpdate(optimistic, false)
			server := &Server{LCStore: store}
			for _, endpoint := range []struct {
				name    string
				handler http.HandlerFunc
				marshal func() ([]byte, error)
			}{
				{"bootstrap", server.GetLightClientBootstrap, bootstrap.MarshalSSZ},
				{"finality", server.GetLightClientFinalityUpdate, finality.MarshalSSZ},
				{"optimistic", server.GetLightClientOptimisticUpdate, optimistic.MarshalSSZ},
			} {
				for _, format := range []string{"application/json", "application/octet-stream"} {
					t.Run(endpoint.name+"/"+format, func(t *testing.T) {
						request := httptest.NewRequest(http.MethodGet, "/", nil)
						request.SetPathValue("block_root", hexutil.Encode(root[:]))
						request.Header.Set("Accept", format)
						writer := httptest.NewRecorder()
						endpoint.handler(writer, request)
						require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
						require.Equal(t, version.String(fork), writer.Header().Get(api.VersionHeader))
						if format == "application/octet-stream" {
							want, err := endpoint.marshal()
							require.NoError(t, err)
							require.DeepEqual(t, want, writer.Body.Bytes())
							return
						}
						var response struct {
							Version string                     `json:"version"`
							Data    map[string]json.RawMessage `json:"data"`
						}
						require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &response))
						require.Equal(t, version.String(fork), response.Version)
						if endpoint.name == "bootstrap" {
							checkLightClientAPIHeader(t, response.Data["header"], update.AttestedHeader())
						} else {
							checkLightClientAPIHeader(t, response.Data["attested_header"], update.AttestedHeader())
						}
						if endpoint.name == "finality" {
							checkLightClientAPIHeader(t, response.Data["finalized_header"], update.FinalizedHeader())
						}
					})
				}
			}
		})
	}
}

func TestLightClientHandlers_GloasMixedRange(t *testing.T) {
	setupLightClientAPIConfig(t)
	updates := []interfaces.LightClientUpdate{lightClientAPIUpdate(t, version.Fulu), lightClientAPIUpdate(t, version.Gloas)}
	db := dbtesting.SetupDB(t)
	for i, update := range updates {
		require.NoError(t, db.SaveLightClientUpdate(t.Context(), uint64(5+i), update))
	}
	head, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)
	server := &Server{
		LCStore:     lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db),
		HeadFetcher: &blockchainTest.ChainService{Block: head},
	}
	for _, format := range []string{"application/json", "application/octet-stream"} {
		t.Run(format, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?start_period=5&count=2", nil)
			request.Header.Set("Accept", format)
			writer := httptest.NewRecorder()
			server.GetLightClientUpdatesByRange(writer, request)
			require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
			if format == "application/json" {
				var responses []structs.LightClientUpdateResponse
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &responses))
				require.Equal(t, 2, len(responses))
				for i, response := range responses {
					require.Equal(t, version.String(updates[i].Version()), response.Version)
					checkLightClientAPIHeader(t, response.Data.AttestedHeader, updates[i].AttestedHeader())
					checkLightClientAPIHeader(t, response.Data.FinalizedHeader, updates[i].FinalizedHeader())
				}
				return
			}
			reader := bytes.NewReader(writer.Body.Bytes())
			for i, update := range updates {
				var length uint64
				require.NoError(t, binary.Read(reader, binary.LittleEndian, &length))
				wantSSZ, err := update.MarshalSSZ()
				require.NoError(t, err)
				require.Equal(t, uint64(len(wantSSZ)+4), length)
				var digest [4]byte
				_, err = io.ReadFull(reader, digest[:])
				require.NoError(t, err)
				require.Equal(t, params.ForkDigest(primitives.Epoch(5+i)), digest)
				ssz := make([]byte, length-4)
				_, err = io.ReadFull(reader, ssz)
				require.NoError(t, err)
				require.DeepEqual(t, wantSSZ, ssz)
			}
			require.Equal(t, 0, reader.Len())
		})
	}
}

func setupLightClientAPIConfig(t *testing.T) {
	t.Helper()
	cfg := params.MainnetTestConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 0, 1, 2
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch = 3, 4, 5, 6
	cfg.EpochsPerSyncCommitteePeriod = 1
	params.SetActiveTestCleanup(t, cfg)
}

func lightClientAPIUpdate(t *testing.T, fork int) interfaces.LightClientUpdate {
	t.Helper()
	var block interfaces.SignedBeaconBlock
	var err error
	if fork == version.Gloas {
		p := util.NewBeaconBlockGloas()
		p.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash[0] = 42
		p.Block.Body.SignedExecutionPayloadBid.Message.BlockHash[0] = 99
		block, err = blocks.NewSignedBeaconBlock(p)
	} else {
		block, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockFulu())
	}
	require.NoError(t, err)
	if fork == version.Gloas {
		block.SetSlot(6 * fieldparams.SlotsPerEpoch)
	} else {
		block.SetSlot(6*fieldparams.SlotsPerEpoch - 1)
	}
	update, err := lightclient.CreateDefaultLightClientUpdate(block)
	require.NoError(t, err)
	header, err := lightclient.BlockToLightClientHeader(t.Context(), fork, block)
	require.NoError(t, err)
	require.NoError(t, update.SetAttestedHeader(header))
	finalizedBlock, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockFulu())
	require.NoError(t, err)
	finalizedBlock.SetSlot(5 * fieldparams.SlotsPerEpoch)
	finalizedHeader, err := lightclient.BlockToLightClientHeader(t.Context(), fork, finalizedBlock)
	require.NoError(t, err)
	require.NoError(t, update.SetFinalizedHeader(finalizedHeader))
	update.SetSignatureSlot(block.Block().Slot() + 1)
	update.SetSyncAggregate(&pb.SyncAggregate{SyncCommitteeBits: make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength), SyncCommitteeSignature: make([]byte, 96)})
	return update
}

func checkLightClientAPIHeader(t *testing.T, raw json.RawMessage, want interfaces.LightClientHeader) {
	t.Helper()
	var got structs.LightClientHeaderGloas
	require.NoError(t, json.Unmarshal(raw, &got))
	require.DeepEqual(t, structs.BeaconBlockHeaderFromConsensus(want.Beacon()), got.Beacon)
	if want.Version() == version.Gloas {
		hash, err := want.ExecutionBlockHash()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(hash[:]), got.ExecutionBlockHash)
		branch, err := want.ExecutionBranchGloas()
		require.NoError(t, err)
		require.Equal(t, 11, len(got.ExecutionBranch))
		for i := range branch {
			require.Equal(t, hexutil.Encode(branch[i][:]), got.ExecutionBranch[i])
		}
		require.Equal(t, false, bytes.Contains(raw, []byte(`"execution":`)))
	} else {
		require.Equal(t, "", got.ExecutionBlockHash)
		require.Equal(t, 4, len(got.ExecutionBranch))
		require.Equal(t, true, bytes.Contains(raw, []byte(`"execution":`)))
	}
}
