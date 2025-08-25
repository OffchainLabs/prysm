package backfill

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/interop"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestDomainCache(t *testing.T) {
	cfg := params.MainnetConfig()
	// This hack is needed not to have both Electra and Fulu fork epoch both set to the future max epoch.
	// It can be removed once the Electra fork version has been set to a real value.
	for version := range cfg.ForkVersionSchedule {
		if cfg.ForkVersionNames[version] == "electra" {
			cfg.ForkVersionSchedule[version] = math.MaxUint64 - 1
		}
	}

	vRoot, err := hexutil.Decode("0x0011223344556677889900112233445566778899001122334455667788990011")
	require.NoError(t, err)
	dType := cfg.DomainBeaconProposer
	require.Equal(t, 32, len(vRoot))
	dc, err := newDomainCache(vRoot, dType)
	require.NoError(t, err)
	schedule := params.SortedForkSchedule()
	require.Equal(t, len(schedule), len(dc.forkDomains))
	for _, entry := range schedule {
		ad, err := dc.forEpoch(entry.Epoch)
		require.NoError(t, err)
		ed, err := signing.ComputeDomain(dType, entry.ForkVersion[:], vRoot)
		require.NoError(t, err)
		require.DeepEqual(t, ed, ad)
	}
}

func testBlocksWithKeys(t *testing.T, nBlocks uint64, nBlobs int, vr []byte) ([]blocks.ROBlock, [][]blocks.ROBlob, []bls.SecretKey, []bls.PublicKey) {
	blks := make([]blocks.ROBlock, nBlocks)
	blbs := make([][]blocks.ROBlob, nBlocks)
	sks, pks, err := interop.DeterministicallyGenerateKeys(0, nBlocks)
	require.NoError(t, err)
	prevRoot := [32]byte{}
	for i := uint64(0); i < nBlocks; i++ {
		block, blobs := util.GenerateTestDenebBlockWithSidecar(t, prevRoot, primitives.Slot(i), nBlobs, util.WithProposerSigning(primitives.ValidatorIndex(i), sks[i], vr))
		prevRoot = block.Root()
		blks[i] = block
		blbs[i] = blobs
	}
	return blks, blbs, sks, pks
}

func TestVerify(t *testing.T) {
	vr := make([]byte, 32)
	copy(vr, "yooooo")
	blks, _, _, pks := testBlocksWithKeys(t, 2, 0, vr)
	pubkeys := make([][fieldparams.BLSPubkeyLength]byte, len(pks))
	for i := range pks {
		pubkeys[i] = bytesutil.ToBytes48(pks[i].Marshal())
	}
	v, err := newBackfillVerifier(vr, pubkeys)
	require.NoError(t, err)
	notrob := make([]interfaces.ReadOnlySignedBeaconBlock, len(blks))
	// We have to unwrap the ROBlocks for this code because that's what it expects (for now).
	for i := range blks {
		notrob[i] = blks[i].ReadOnlySignedBeaconBlock
	}
	vbs, err := v.verify(notrob)
	require.NoError(t, err)
	require.Equal(t, len(blks), len(vbs))
}
