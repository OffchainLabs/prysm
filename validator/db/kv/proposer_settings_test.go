package kv

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_ProposerSettings_ReadAndWrite(t *testing.T) {
	t.Run("save to db in full", func(t *testing.T) {
		ctx := context.Background()
		db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})
		key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
		require.NoError(t, err)
		settings := &proposer.Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				bytesutil.ToBytes48(key1): {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
					},
				},
			},
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  false,
					GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				},
			},
		}
		err = db.SaveProposerSettings(ctx, settings)
		require.NoError(t, err)

		dbSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, settings, dbSettings)
	})
	t.Run("update default settings then update at specific key", func(t *testing.T) {
		ctx := context.Background()
		db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})
		key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
		require.NoError(t, err)
		settings := &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  false,
					GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				},
			},
		}
		err = db.SaveProposerSettings(ctx, settings)
		require.NoError(t, err)
		updatedDefault := &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x9995733c5af9B61374A128e6F85f553aF09ff89B"),
			},
			BuilderConfig: &proposer.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
			},
		}
		settings.DefaultConfig = updatedDefault
		err = db.SaveProposerSettings(ctx, settings)
		require.NoError(t, err)

		dbSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbSettings)
		require.DeepEqual(t, dbSettings.DefaultConfig, updatedDefault)
		option := &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
			},
			BuilderConfig: &proposer.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(40000000),
			},
		}

		dbSettings.ProposeConfig = map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{bytesutil.ToBytes48(key1): option}
		err = db.SaveProposerSettings(ctx, dbSettings)
		require.NoError(t, err)

		newSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, newSettings)
		require.DeepEqual(t, newSettings.DefaultConfig, updatedDefault)
		op, ok := newSettings.ProposeConfig[bytesutil.ToBytes48(key1)]
		require.Equal(t, ok, true)
		require.DeepEqual(t, op, option)
	})
}
