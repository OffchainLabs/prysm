package backfill

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func mockCurrentNeeds(begin, end primitives.Slot) das.CurrentNeeds {
	return das.CurrentNeeds{
		Block: das.NeedSpan{
			Begin: begin,
			End:   end,
		},
		Blob: das.NeedSpan{
			Begin: begin,
			End:   end,
		},
		Col: das.NeedSpan{
			Begin: begin,
			End:   end,
		},
	}
}

func mockCurrentSpecNeeds() das.CurrentNeeds {
	cfg := params.BeaconConfig()
	fuluSlot := slots.UnsafeEpochStart(cfg.FuluForkEpoch)
	denebSlot := slots.UnsafeEpochStart(cfg.DenebForkEpoch)
	return das.CurrentNeeds{
		Block: das.NeedSpan{
			Begin: 0,
			End:   primitives.Slot(math.MaxUint64),
		},
		Blob: das.NeedSpan{
			Begin: denebSlot,
			End:   fuluSlot,
		},
		Col: das.NeedSpan{
			Begin: fuluSlot,
			End:   primitives.Slot(math.MaxUint64),
		},
	}
}

func mockCurrentNeedsFunc(begin, end primitives.Slot) func() das.CurrentNeeds {
	return func() das.CurrentNeeds {
		return mockCurrentNeeds(begin, end)
	}
}

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
	for i := range nBlocks {
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
	vbs, err := v.verify(blks)
	require.NoError(t, err)
	require.Equal(t, len(blks), len(vbs))
}
