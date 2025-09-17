package lightclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/async/event"
	blockchainTest "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	dbtesting "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	p2ptesting "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/prysmaticlabs/fastssz"
	"google.golang.org/protobuf/proto"
)

func TestLightClientHandler_GetLightClientBootstrap(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 1
	cfg.CapellaForkEpoch = 2
	cfg.DenebForkEpoch = 3
	cfg.ElectraForkEpoch = 4
	cfg.FuluForkEpoch = 5
	params.OverrideBeaconConfig(cfg)

	for testVersion := version.Altair; testVersion <= version.Electra; testVersion++ {
		t.Run(version.String(testVersion), func(t *testing.T) {
			l := util.NewTestLightClient(t, testVersion)

			slot := primitives.Slot(params.BeaconConfig().VersionToForkEpochMap()[testVersion] * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
			blockRoot, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)

			bootstrap, err := lightclient.NewLightClientBootstrapFromBeaconState(l.Ctx, slot, l.State, l.Block)
			require.NoError(t, err)

			db := dbtesting.SetupDB(t)
			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)
			require.NoError(t, err)

			err = db.SaveLightClientBootstrap(l.Ctx, blockRoot[:], bootstrap)
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			request := httptest.NewRequest("GET", "http://foo.com/", nil)
			request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetLightClientBootstrap(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			var resp structs.LightClientBootstrapResponse
			err = json.Unmarshal(writer.Body.Bytes(), &resp)
			require.NoError(t, err)
			var respHeader structs.LightClientHeader
			err = json.Unmarshal(resp.Data.Header, &respHeader)
			require.NoError(t, err)
			require.Equal(t, version.String(testVersion), resp.Version)

			blockHeader, err := l.Block.Header()
			require.NoError(t, err)
			require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
			require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

			require.NotNil(t, resp.Data.CurrentSyncCommittee)
			require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
		})

		t.Run(version.String(testVersion)+"SSZ", func(t *testing.T) {
			l := util.NewTestLightClient(t, testVersion)

			slot := primitives.Slot(params.BeaconConfig().VersionToForkEpochMap()[testVersion] * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
			blockRoot, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)

			bootstrap, err := lightclient.NewLightClientBootstrapFromBeaconState(l.Ctx, slot, l.State, l.Block)
			require.NoError(t, err)

			db := dbtesting.SetupDB(t)
			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)
			require.NoError(t, err)

			err = db.SaveLightClientBootstrap(l.Ctx, blockRoot[:], bootstrap)
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			request := httptest.NewRequest("GET", "http://foo.com/", nil)
			request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
			request.Header.Add("Accept", "application/octet-stream")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetLightClientBootstrap(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			var resp proto.Message
			switch testVersion {
			case version.Altair:
				resp = &pb.LightClientBootstrapAltair{}
			case version.Bellatrix:
				resp = &pb.LightClientBootstrapAltair{}
			case version.Capella:
				resp = &pb.LightClientBootstrapCapella{}
			case version.Deneb:
				resp = &pb.LightClientBootstrapDeneb{}
			case version.Electra:
				resp = &pb.LightClientBootstrapElectra{}
			default:
				t.Fatalf("Unsupported version %s", version.String(testVersion))
			}
			obj := resp.(ssz.Unmarshaler)
			err = obj.UnmarshalSSZ(writer.Body.Bytes())
			require.NoError(t, err)

			bootstrapSSZ, err := bootstrap.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, bootstrapSSZ, writer.Body.Bytes())
		})
	}

	t.Run("no bootstrap found", func(t *testing.T) {
		lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
		s := &Server{
			LCStore: lcStore,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode([]byte{0x00, 0x01, 0x02}))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})
}

func TestLightClientHandler_GetLightClientByRange(t *testing.T) {
	helpers.ClearCache()
	ctx := t.Context()

	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.EpochsPerSyncCommitteePeriod = 1
	config.AltairForkEpoch = 0
	config.BellatrixForkEpoch = 1
	config.CapellaForkEpoch = 2
	config.DenebForkEpoch = 3
	config.ElectraForkEpoch = 4
	params.OverrideBeaconConfig(config)

	t.Run("can save retrieve", func(t *testing.T) {
		for testVersion := version.Altair; testVersion <= version.Electra; testVersion++ {
			t.Run(version.String(testVersion), func(t *testing.T) {

				slot := primitives.Slot(params.BeaconConfig().VersionToForkEpochMap()[testVersion] * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
				startPeriod := uint64(slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch)))

				db := dbtesting.SetupDB(t)

				updates := make([]interfaces.LightClientUpdate, 0)
				for i := 1; i <= 2; i++ {
					update, err := createUpdate(t, testVersion)
					require.NoError(t, err)
					updates = append(updates, update)
				}

				lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

				blk := util.NewBeaconBlock()
				signedBlk, err := blocks.NewSignedBeaconBlock(blk)
				require.NoError(t, err)

				s := &Server{
					LCStore: lcStore,
					HeadFetcher: &blockchainTest.ChainService{
						Block: signedBlk,
					},
				}

				saveHead(t, ctx, db)

				updatePeriod := startPeriod
				for _, update := range updates {
					err := db.SaveLightClientUpdate(ctx, updatePeriod, update)
					require.NoError(t, err)
					updatePeriod++
				}

				t.Run("single update", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=1&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)
					var resp structs.LightClientUpdatesByRangeResponse
					err := json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
					require.NoError(t, err)
					require.Equal(t, 1, len(resp.Updates))
					require.Equal(t, version.String(testVersion), resp.Updates[0].Version)
					updateJson, err := structs.LightClientUpdateFromConsensus(updates[0])
					require.NoError(t, err)
					require.DeepEqual(t, updateJson, resp.Updates[0].Data)
				})

				t.Run("single update ssz", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=1&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					request.Header.Add("Accept", "application/octet-stream")
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)
					var resp proto.Message
					switch testVersion {
					case version.Altair:
						resp = &pb.LightClientUpdateAltair{}
					case version.Bellatrix:
						resp = &pb.LightClientUpdateAltair{}
					case version.Capella:
						resp = &pb.LightClientUpdateCapella{}
					case version.Deneb:
						resp = &pb.LightClientUpdateDeneb{}
					case version.Electra:
						resp = &pb.LightClientUpdateElectra{}
					default:
						t.Fatalf("Unsupported version %s", version.String(testVersion))
					}
					obj := resp.(ssz.Unmarshaler)
					err := obj.UnmarshalSSZ(writer.Body.Bytes()[12:]) // skip the length and fork digest prefixes
					require.NoError(t, err)

					ussz, err := updates[0].MarshalSSZ()
					require.NoError(t, err)
					require.DeepSSZEqual(t, ussz, writer.Body.Bytes()[12:])
				})

				t.Run("multiple updates", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)
					var resp structs.LightClientUpdatesByRangeResponse
					err := json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
					require.NoError(t, err)
					require.Equal(t, 2, len(resp.Updates))
					for i, update := range updates {
						require.Equal(t, version.String(testVersion), resp.Updates[i].Version)
						updateJson, err := structs.LightClientUpdateFromConsensus(update)
						require.NoError(t, err)
						require.DeepEqual(t, updateJson, resp.Updates[i].Data)
					}
				})

				t.Run("multiple updates ssz", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					request.Header.Add("Accept", "application/octet-stream")
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)

					offset := 0
					for i := 0; offset < writer.Body.Len(); i++ {
						updateLen := int(ssz.UnmarshallUint64(writer.Body.Bytes()[offset:offset+8]) - 4)
						offset += 12

						var resp proto.Message
						switch testVersion {
						case version.Altair:
							resp = &pb.LightClientUpdateAltair{}
						case version.Bellatrix:
							resp = &pb.LightClientUpdateAltair{}
						case version.Capella:
							resp = &pb.LightClientUpdateCapella{}
						case version.Deneb:
							resp = &pb.LightClientUpdateDeneb{}
						case version.Electra:
							resp = &pb.LightClientUpdateElectra{}
						default:
							t.Fatalf("Unsupported version %s", version.String(testVersion))
						}
						obj := resp.(ssz.Unmarshaler)

						updateBytes := writer.Body.Bytes()[offset : offset+updateLen]

						err := obj.UnmarshalSSZ(updateBytes)
						require.NoError(t, err)

						ussz, err := updates[i].MarshalSSZ()
						require.NoError(t, err)
						require.DeepSSZEqual(t, ussz, updateBytes)

						offset += updateLen
					}
				})
			})
		}
	})

	t.Run("updates from multiple forks", func(t *testing.T) {
		for testVersion := version.Altair; testVersion < version.Electra; testVersion++ { // 1-2, 2-3, 3-4, 4-5
			t.Run(version.String(testVersion)+"-"+version.String(testVersion+1), func(t *testing.T) {
				firstForkSlot := primitives.Slot(params.BeaconConfig().VersionToForkEpochMap()[testVersion] * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
				secondForkSlot := primitives.Slot(params.BeaconConfig().VersionToForkEpochMap()[testVersion+1] * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

				db := dbtesting.SetupDB(t)
				lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

				blk := util.NewBeaconBlock()
				signedBlk, err := blocks.NewSignedBeaconBlock(blk)
				require.NoError(t, err)

				s := &Server{
					LCStore: lcStore,
					HeadFetcher: &blockchainTest.ChainService{
						Block: signedBlk,
					},
				}

				saveHead(t, ctx, db)

				updates := make([]interfaces.LightClientUpdate, 2)

				updatePeriod := firstForkSlot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
				startPeriod := updatePeriod

				updates[0], err = createUpdate(t, testVersion)
				require.NoError(t, err)

				err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[0])
				require.NoError(t, err)

				updatePeriod = secondForkSlot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

				updates[1], err = createUpdate(t, testVersion+1)
				require.NoError(t, err)

				err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[1])
				require.NoError(t, err)

				t.Run("json", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)
					var resp structs.LightClientUpdatesByRangeResponse
					err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
					require.NoError(t, err)
					require.Equal(t, 2, len(resp.Updates))
					for i, update := range updates {
						if i < 1 {
							require.Equal(t, version.String(testVersion), resp.Updates[i].Version)
						} else {
							require.Equal(t, version.String(testVersion+1), resp.Updates[i].Version)
						}
						updateJson, err := structs.LightClientUpdateFromConsensus(update)
						require.NoError(t, err)
						require.DeepEqual(t, updateJson, resp.Updates[i].Data)
					}
				})

				t.Run("ssz", func(t *testing.T) {
					url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
					request := httptest.NewRequest("GET", url, nil)
					request.Header.Add("Accept", "application/octet-stream")
					writer := httptest.NewRecorder()
					writer.Body = &bytes.Buffer{}

					s.GetLightClientUpdatesByRange(writer, request)

					require.Equal(t, http.StatusOK, writer.Code)

					offset := 0
					updateLen := int(ssz.UnmarshallUint64(writer.Body.Bytes()[offset:offset+8]) - 4)
					offset += 12
					var resp proto.Message
					switch testVersion {
					case version.Altair:
						resp = &pb.LightClientUpdateAltair{}
					case version.Bellatrix:
						resp = &pb.LightClientUpdateAltair{}
					case version.Capella:
						resp = &pb.LightClientUpdateCapella{}
					case version.Deneb:
						resp = &pb.LightClientUpdateDeneb{}
					case version.Electra:
						resp = &pb.LightClientUpdateElectra{}
					default:
						t.Fatalf("Unsupported version %s", version.String(testVersion))
					}
					obj := resp.(ssz.Unmarshaler)
					err = obj.UnmarshalSSZ(writer.Body.Bytes()[offset : offset+updateLen])
					require.NoError(t, err)
					u0ssz, err := updates[0].MarshalSSZ()
					require.NoError(t, err)
					require.DeepSSZEqual(t, u0ssz, writer.Body.Bytes()[offset:offset+updateLen])

					offset += updateLen
					updateLen = int(ssz.UnmarshallUint64(writer.Body.Bytes()[offset:offset+8]) - 4)
					offset += 12
					var resp1 proto.Message
					switch testVersion + 1 {
					case version.Altair:
						resp1 = &pb.LightClientUpdateAltair{}
					case version.Bellatrix:
						resp1 = &pb.LightClientUpdateAltair{}
					case version.Capella:
						resp1 = &pb.LightClientUpdateCapella{}
					case version.Deneb:
						resp1 = &pb.LightClientUpdateDeneb{}
					case version.Electra:
						resp1 = &pb.LightClientUpdateElectra{}
					default:
						t.Fatalf("Unsupported version %s", version.String(testVersion+1))
					}
					obj1 := resp1.(ssz.Unmarshaler)
					err = obj1.UnmarshalSSZ(writer.Body.Bytes()[offset : offset+updateLen])
					require.NoError(t, err)
					u1ssz, err := updates[1].MarshalSSZ()
					require.NoError(t, err)
					require.DeepSSZEqual(t, u1ssz, writer.Body.Bytes()[offset:offset+updateLen])
				})
			})
		}
	})

	t.Run("count bigger than limit", func(t *testing.T) {
		config.MaxRequestLightClientUpdates = 2
		params.OverrideBeaconConfig(config)
		slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

		db := dbtesting.SetupDB(t)
		lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

		blk := util.NewBeaconBlock()
		signedBlk, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		s := &Server{
			LCStore: lcStore,
			HeadFetcher: &blockchainTest.ChainService{
				Block: signedBlk,
			},
		}

		saveHead(t, ctx, db)

		updates := make([]interfaces.LightClientUpdate, 3)

		updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

		for i := 0; i < 3; i++ {
			updates[i], err = createUpdate(t, version.Altair)
			require.NoError(t, err)

			err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
			require.NoError(t, err)

			updatePeriod++
		}

		startPeriod := 0
		url := fmt.Sprintf("http://foo.com/?count=4&start_period=%d", startPeriod)
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientUpdatesByRange(writer, request)

		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientUpdatesByRangeResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
		require.NoError(t, err)
		require.Equal(t, 2, len(resp.Updates))
		for i, update := range updates {
			if i < 2 {
				require.Equal(t, "altair", resp.Updates[i].Version)
				updateJson, err := structs.LightClientUpdateFromConsensus(update)
				require.NoError(t, err)
				require.DeepEqual(t, updateJson, resp.Updates[i].Data)
			}
		}
	})

	t.Run("count bigger than max", func(t *testing.T) {
		config.MaxRequestLightClientUpdates = 2
		params.OverrideBeaconConfig(config)
		slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

		db := dbtesting.SetupDB(t)
		lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

		blk := util.NewBeaconBlock()
		signedBlk, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		s := &Server{
			LCStore: lcStore,
			HeadFetcher: &blockchainTest.ChainService{
				Block: signedBlk,
			},
		}

		saveHead(t, ctx, db)

		updates := make([]interfaces.LightClientUpdate, 3)

		updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

		for i := 0; i < 3; i++ {
			updates[i], err = createUpdate(t, version.Altair)
			require.NoError(t, err)

			err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
			require.NoError(t, err)

			updatePeriod++
		}

		startPeriod := 0
		url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientUpdatesByRange(writer, request)

		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientUpdatesByRangeResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
		require.NoError(t, err)
		require.Equal(t, 2, len(resp.Updates))
		for i, update := range updates {
			if i < 2 {
				require.Equal(t, "altair", resp.Updates[i].Version)
				updateJson, err := structs.LightClientUpdateFromConsensus(update)
				require.NoError(t, err)
				require.DeepEqual(t, updateJson, resp.Updates[i].Data)
			}
		}
	})

	t.Run("start period before altair", func(t *testing.T) {
		config.AltairForkEpoch = 1
		params.OverrideBeaconConfig(config)

		db := dbtesting.SetupDB(t)
		lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

		s := &Server{
			LCStore: lcStore,
		}

		startPeriod := 0
		url := fmt.Sprintf("http://foo.com/?count=128&start_period=%d", startPeriod)
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientUpdatesByRange(writer, request)

		require.Equal(t, http.StatusBadRequest, writer.Code)

		config.AltairForkEpoch = 0
		params.OverrideBeaconConfig(config)
	})

	t.Run("missing updates", func(t *testing.T) {
		slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

		t.Run("missing update in the middle", func(t *testing.T) {
			db := dbtesting.SetupDB(t)
			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

			blk := util.NewBeaconBlock()
			signedBlk, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
				HeadFetcher: &blockchainTest.ChainService{
					Block: signedBlk,
				},
			}

			saveHead(t, ctx, db)

			updates := make([]interfaces.LightClientUpdate, 3)

			updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

			for i := 0; i < 3; i++ {
				if i == 1 { // skip this update
					updatePeriod++
					continue
				}
				updates[i], err = createUpdate(t, version.Altair)
				require.NoError(t, err)

				err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
				require.NoError(t, err)

				updatePeriod++
			}

			startPeriod := 0
			url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
			request := httptest.NewRequest("GET", url, nil)
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetLightClientUpdatesByRange(writer, request)

			require.Equal(t, http.StatusOK, writer.Code)
			var resp structs.LightClientUpdatesByRangeResponse
			err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
			require.NoError(t, err)
			require.Equal(t, 1, len(resp.Updates))
			require.Equal(t, "altair", resp.Updates[0].Version)
			updateJson, err := structs.LightClientUpdateFromConsensus(updates[0])
			require.NoError(t, err)
			require.DeepEqual(t, updateJson, resp.Updates[0].Data)
		})

		t.Run("missing update at the beginning", func(t *testing.T) {
			db := dbtesting.SetupDB(t)
			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), db)

			blk := util.NewBeaconBlock()
			signedBlk, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
				HeadFetcher: &blockchainTest.ChainService{
					Block: signedBlk,
				},
			}

			saveHead(t, ctx, db)

			updates := make([]interfaces.LightClientUpdate, 3)

			updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

			for i := 0; i < 3; i++ {
				if i == 0 { // skip this update
					updatePeriod++
					continue
				}

				updates[i], err = createUpdate(t, version.Altair)
				require.NoError(t, err)

				err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
				require.NoError(t, err)

				updatePeriod++
			}

			startPeriod := 0
			url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
			request := httptest.NewRequest("GET", url, nil)
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetLightClientUpdatesByRange(writer, request)

			require.Equal(t, http.StatusOK, writer.Code)
			var resp structs.LightClientUpdatesByRangeResponse
			err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
			require.NoError(t, err)
			require.Equal(t, 0, len(resp.Updates))
		})
	})
}

func TestLightClientHandler_GetLightClientFinalityUpdate(t *testing.T) {
	helpers.ClearCache()

	t.Run("no update", func(t *testing.T) {
		s := &Server{LCStore: &lightclient.Store{}}
		request := httptest.NewRequest("GET", "http://foo.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.GetLightClientFinalityUpdate(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})

	for testVersion := 1; testVersion < 6; testVersion++ {
		t.Run(version.String(testVersion), func(t *testing.T) {
			ctx := t.Context()

			l := util.NewTestLightClient(t, testVersion)
			update, err := lightclient.NewLightClientFinalityUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)

			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			s.LCStore.SetLastFinalityUpdate(update, false)

			request := httptest.NewRequest("GET", "http://foo.com", nil)
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}
			s.GetLightClientFinalityUpdate(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			data, err := structs.LightClientFinalityUpdateFromConsensus(update)
			require.NoError(t, err)
			var resp structs.LightClientFinalityUpdateResponse
			err = json.Unmarshal(writer.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, version.String(testVersion), resp.Version)
			require.DeepEqual(t, data, resp.Data)
		})

		t.Run(version.String(testVersion)+" SSZ", func(t *testing.T) {
			ctx := t.Context()

			l := util.NewTestLightClient(t, testVersion)
			update, err := lightclient.NewLightClientFinalityUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)

			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			s.LCStore.SetLastFinalityUpdate(update, false)

			request := httptest.NewRequest("GET", "http://foo.com", nil)
			request.Header.Add("Accept", "application/octet-stream")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}
			s.GetLightClientFinalityUpdate(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			var resp proto.Message
			switch testVersion {
			case version.Altair:
				resp = &pb.LightClientFinalityUpdateAltair{}
			case version.Bellatrix:
				resp = &pb.LightClientFinalityUpdateAltair{}
			case version.Capella:
				resp = &pb.LightClientFinalityUpdateCapella{}
			case version.Deneb:
				resp = &pb.LightClientFinalityUpdateDeneb{}
			case version.Electra:
				resp = &pb.LightClientFinalityUpdateElectra{}
			default:
				t.Fatalf("Unsupported version %s", version.String(testVersion))
			}
			obj := resp.(ssz.Unmarshaler)
			err = obj.UnmarshalSSZ(writer.Body.Bytes())
			require.NoError(t, err)
			updateSSZ, err := update.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, updateSSZ, writer.Body.Bytes())
		})
	}
}

func TestLightClientHandler_GetLightClientOptimisticUpdate(t *testing.T) {
	helpers.ClearCache()

	t.Run("no update", func(t *testing.T) {
		lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))

		s := &Server{
			LCStore: lcStore,
		}

		request := httptest.NewRequest("GET", "http://foo.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.GetLightClientOptimisticUpdate(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})

	for testVersion := 1; testVersion < 6; testVersion++ {
		t.Run(version.String(testVersion), func(t *testing.T) {
			ctx := t.Context()
			l := util.NewTestLightClient(t, testVersion)
			update, err := lightclient.NewLightClientOptimisticUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock)
			require.NoError(t, err)

			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			s.LCStore.SetLastOptimisticUpdate(update, false)

			request := httptest.NewRequest("GET", "http://foo.com", nil)
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}
			s.GetLightClientOptimisticUpdate(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			data, err := structs.LightClientOptimisticUpdateFromConsensus(update)
			require.NoError(t, err)
			var resp structs.LightClientOptimisticUpdateResponse
			err = json.Unmarshal(writer.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, version.String(testVersion), resp.Version)
			require.DeepEqual(t, data, resp.Data)
		})

		t.Run(version.String(testVersion)+" SSZ", func(t *testing.T) {
			ctx := t.Context()
			l := util.NewTestLightClient(t, testVersion)
			update, err := lightclient.NewLightClientOptimisticUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock)
			require.NoError(t, err)

			lcStore := lightclient.NewLightClientStore(&p2ptesting.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
			require.NoError(t, err)

			s := &Server{
				LCStore: lcStore,
			}
			s.LCStore.SetLastOptimisticUpdate(update, false)

			request := httptest.NewRequest("GET", "http://foo.com", nil)
			request.Header.Add("Accept", "application/octet-stream")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}
			s.GetLightClientOptimisticUpdate(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			var resp proto.Message
			switch testVersion {
			case version.Altair:
				resp = &pb.LightClientOptimisticUpdateAltair{}
			case version.Bellatrix:
				resp = &pb.LightClientOptimisticUpdateAltair{}
			case version.Capella:
				resp = &pb.LightClientOptimisticUpdateCapella{}
			case version.Deneb:
				resp = &pb.LightClientOptimisticUpdateDeneb{}
			case version.Electra:
				resp = &pb.LightClientOptimisticUpdateDeneb{}
			default:
				t.Fatalf("Unsupported version %s", version.String(testVersion))
			}
			obj := resp.(ssz.Unmarshaler)
			err = obj.UnmarshalSSZ(writer.Body.Bytes())
			require.NoError(t, err)
			updateSSZ, err := update.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, updateSSZ, writer.Body.Bytes())
		})
	}
}

func createUpdate(t *testing.T, v int) (interfaces.LightClientUpdate, error) {
	config := params.BeaconConfig()
	var slot primitives.Slot
	var header interfaces.LightClientHeader
	var blk interfaces.ReadOnlySignedBeaconBlock
	var err error

	sampleRoot := make([]byte, 32)
	for i := 0; i < 32; i++ {
		sampleRoot[i] = byte(i)
	}

	sampleExecutionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < 4; i++ {
		sampleExecutionBranch[i] = make([]byte, 32)
		for j := 0; j < 32; j++ {
			sampleExecutionBranch[i][j] = byte(i + j)
		}
	}

	switch v {
	case version.Altair:
		slot = primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockAltair())
		require.NoError(t, err)
	case version.Bellatrix:
		slot = primitives.Slot(config.BellatrixForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockBellatrix())
		require.NoError(t, err)
	case version.Capella:
		slot = primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderCapella{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderCapella{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
	case version.Deneb:
		slot = primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
		require.NoError(t, err)
	case version.Electra:
		slot = primitives.Slot(config.ElectraForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
	case version.Fulu:
		slot = primitives.Slot(config.FuluForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          slot,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		blk, err = blocks.NewSignedBeaconBlock(util.NewBeaconBlockFulu())
		require.NoError(t, err)
	default:
		return nil, fmt.Errorf("unsupported version %s", version.String(v))
	}

	update, err := lightclient.CreateDefaultLightClientUpdate(blk)
	require.NoError(t, err)
	update.SetSignatureSlot(slot - 1)
	syncCommitteeBits := make([]byte, 64)
	syncCommitteeSignature := make([]byte, 96)
	update.SetSyncAggregate(&pb.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: syncCommitteeSignature,
	})

	require.NoError(t, update.SetAttestedHeader(header))
	require.NoError(t, update.SetFinalizedHeader(header))

	return update, nil
}

func saveHead(t *testing.T, ctx context.Context, d db.Database) {
	blk := util.NewBeaconBlock()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, d, blk)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, d.SaveState(ctx, st, blkRoot))
	require.NoError(t, d.SaveHeadBlockRoot(ctx, blkRoot))
}
