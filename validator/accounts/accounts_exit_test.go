package accounts

import (
	"encoding/json"
	"fmt"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/build/bazel"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestDisplayExitInfo(t *testing.T) {
	key := []byte("0x123456")

	t.Run("with URL", func(t *testing.T) {
		logHook := test.NewGlobal()
		otherKey := []byte("0x654321")
		displayExitInfo("https://hoodi.beaconcha.in", [][]byte{key, otherKey}, []string{string(key), string(otherKey)})
		assert.LogsContain(t, logHook, "https://hoodi.beaconcha.in/validator/3078313233343536")
		assert.LogsContain(t, logHook, "https://hoodi.beaconcha.in/validator/3078363534333231")
		assert.LogsContain(t, logHook, "count=2")
	})

	t.Run("without URL", func(t *testing.T) {
		logHook := test.NewGlobal()
		displayExitInfo("", [][]byte{key}, []string{string(key)})
		assert.LogsContain(t, logHook, "Voluntary exit was successful")
		assert.LogsDoNotContain(t, logHook, "beaconcha.in")
	})

	t.Run("no keys", func(t *testing.T) {
		logHook := test.NewGlobal()
		displayExitInfo("https://beaconcha.in", [][]byte{}, []string{})
		assert.LogsContain(t, logHook, "No successful voluntary exits")
	})
}

func TestBeaconChaBaseURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		root [32]byte
		want string
	}{
		{name: "mainnet", root: params.MainnetConfig().GenesisValidatorsRoot, want: "https://beaconcha.in"},
		{name: "sepolia", root: params.SepoliaConfig().GenesisValidatorsRoot, want: "https://sepolia.beaconcha.in"},
		{name: "hoodi", root: params.HoodiConfig().GenesisValidatorsRoot, want: "https://hoodi.beaconcha.in"},
		{name: "unknown network", root: [32]byte{'u'}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, beaconChaBaseURL(tt.root[:]))
		})
	}
}

func TestDisplayExitJSONInfo(t *testing.T) {
	logHook := test.NewGlobal()
	displayExitJSONInfo([]string{"0x123456", "0x654321"}, "/tmp/exits")
	assert.LogsContain(t, logHook, "pubkey=0x123456")
	assert.LogsContain(t, logHook, "pubkey=0x654321")
	assert.LogsContain(t, logHook, "count=2")
	assert.LogsContain(t, logHook, "No exits were broadcast to the network")
	assert.LogsContain(t, logHook, "/tmp/exits")
}

func TestDisplayExitJSONInfo_NoKeys(t *testing.T) {
	logHook := test.NewGlobal()
	displayExitJSONInfo([]string{}, "/tmp/exits")
	assert.LogsContain(t, logHook, "No signed voluntary exit files were written")
}

func TestPrepareAllKeys(t *testing.T) {
	key1 := bytesutil.ToBytes48([]byte("key1"))
	key2 := bytesutil.ToBytes48([]byte("key2"))
	raw, formatted := prepareAllKeys([][fieldparams.BLSPubkeyLength]byte{key1, key2})
	require.Equal(t, 2, len(raw))
	require.Equal(t, 2, len(formatted))
	assert.DeepEqual(t, bytesutil.ToBytes48([]byte{107, 101, 121, 49}), bytesutil.ToBytes48(raw[0]))
	assert.DeepEqual(t, bytesutil.ToBytes48([]byte{107, 101, 121, 50}), bytesutil.ToBytes48(raw[1]))
	assert.Equal(t, "0x6b6579310000", formatted[0])
	assert.Equal(t, "0x6b6579320000", formatted[1])
}

func TestWriteSignedVoluntaryExitJSON(t *testing.T) {
	sve := &eth.SignedVoluntaryExit{
		Exit: &eth.VoluntaryExit{
			Epoch:          5,
			ValidatorIndex: 300,
		},
		Signature: []byte{0x01, 0x02},
	}

	output := path.Join(bazel.TestTmpDir(), "TestWriteSignedVoluntaryExitJSON")
	require.NoError(t, writeSignedVoluntaryExitJSON(sve, output))

	b, err := file.ReadFileAsBytes(path.Join(output, "validator-exit-300.json"))
	require.NoError(t, err)

	svej := &structs.SignedVoluntaryExit{}
	require.NoError(t, json.Unmarshal(b, svej))

	require.Equal(t, fmt.Sprintf("%d", sve.Exit.Epoch), svej.Message.Epoch)
	require.Equal(t, fmt.Sprintf("%d", sve.Exit.ValidatorIndex), svej.Message.ValidatorIndex)
	require.Equal(t, "0x0102", svej.Signature)
}
