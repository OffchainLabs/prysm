package detect

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestSlotFromBlock(t *testing.T) {
	b := util.NewBeaconBlock()
	var slot primitives.Slot = 3
	b.Block.Slot = slot
	bb, err := b.MarshalSSZ()
	require.NoError(t, err)
	sfb, err := slotFromBlock(bb)
	require.NoError(t, err)
	require.Equal(t, slot, sfb)

	ba := util.NewBeaconBlockAltair()
	ba.Block.Slot = slot
	bab, err := ba.MarshalSSZ()
	require.NoError(t, err)
	sfba, err := slotFromBlock(bab)
	require.NoError(t, err)
	require.Equal(t, slot, sfba)

	bm := util.NewBeaconBlockBellatrix()
	bm.Block.Slot = slot
	bmb, err := ba.MarshalSSZ()
	require.NoError(t, err)
	sfbm, err := slotFromBlock(bmb)
	require.NoError(t, err)
	require.Equal(t, slot, sfbm)
}

func TestByState(t *testing.T) {
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	bc := params.BeaconConfig()
	altairSlot, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	bellaSlot, err := slots.EpochStart(bc.BellatrixForkEpoch)
	require.NoError(t, err)
	capellaSlot, err := slots.EpochStart(bc.CapellaForkEpoch)
	require.NoError(t, err)
	denebSlot, err := slots.EpochStart(bc.DenebForkEpoch)
	require.NoError(t, err)
	electraSlot, err := slots.EpochStart(bc.ElectraForkEpoch)
	require.NoError(t, err)
	fuluSlot, err := slots.EpochStart(bc.FuluForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name        string
		version     int
		slot        primitives.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Phase0,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
		{
			name:        "altair",
			version:     version.Altair,
			slot:        altairSlot,
			forkversion: bytesutil.ToBytes4(bc.AltairForkVersion),
		},
		{
			name:        "bellatrix",
			version:     version.Bellatrix,
			slot:        bellaSlot,
			forkversion: bytesutil.ToBytes4(bc.BellatrixForkVersion),
		},
		{
			name:        "capella",
			version:     version.Capella,
			slot:        capellaSlot,
			forkversion: bytesutil.ToBytes4(bc.CapellaForkVersion),
		},
		{
			name:        "deneb",
			version:     version.Deneb,
			slot:        denebSlot,
			forkversion: bytesutil.ToBytes4(bc.DenebForkVersion),
		},
		{
			name:        "electra",
			version:     version.Electra,
			slot:        electraSlot,
			forkversion: bytesutil.ToBytes4(bc.ElectraForkVersion),
		},
		{
			name:        "fulu",
			version:     version.Fulu,
			slot:        fuluSlot,
			forkversion: bytesutil.ToBytes4(bc.FuluForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		fork, err := params.Fork(slots.ToEpoch(c.slot))
		require.NoError(t, err)
		require.NoError(t, st.SetFork(fork))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		require.Equal(t, c.version, cf.Fork)
		require.Equal(t, c.forkversion, cf.Version)
		require.Equal(t, bc.ConfigName, cf.Config.ConfigName)
	}
}

func stateForVersion(v int) (state.BeaconState, error) {
	switch v {
	case version.Phase0:
		return util.NewBeaconState()
	case version.Altair:
		return util.NewBeaconStateAltair()
	case version.Bellatrix:
		return util.NewBeaconStateBellatrix()
	case version.Capella:
		return util.NewBeaconStateCapella()
	case version.Deneb:
		return util.NewBeaconStateDeneb()
	case version.Electra:
		return util.NewBeaconStateElectra()
	case version.Fulu:
		return util.NewBeaconStateFulu()
	default:
		return nil, fmt.Errorf("unrecognized version %d", v)
	}
}

func TestUnmarshalState(t *testing.T) {
	ctx := t.Context()
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	bc := params.BeaconConfig()
	altairSlot, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	bellaSlot, err := slots.EpochStart(bc.BellatrixForkEpoch)
	require.NoError(t, err)
	capellaSlot, err := slots.EpochStart(bc.CapellaForkEpoch)
	require.NoError(t, err)
	denebSlot, err := slots.EpochStart(bc.DenebForkEpoch)
	require.NoError(t, err)
	electraSlot, err := slots.EpochStart(bc.ElectraForkEpoch)
	require.NoError(t, err)
	fuluSlot, err := slots.EpochStart(bc.FuluForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name        string
		version     int
		slot        primitives.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Phase0,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
		{
			name:        "altair",
			version:     version.Altair,
			slot:        altairSlot,
			forkversion: bytesutil.ToBytes4(bc.AltairForkVersion),
		},
		{
			name:        "bellatrix",
			version:     version.Bellatrix,
			slot:        bellaSlot,
			forkversion: bytesutil.ToBytes4(bc.BellatrixForkVersion),
		},
		{
			name:        "capella",
			version:     version.Capella,
			slot:        capellaSlot,
			forkversion: bytesutil.ToBytes4(bc.CapellaForkVersion),
		},
		{
			name:        "deneb",
			version:     version.Deneb,
			slot:        denebSlot,
			forkversion: bytesutil.ToBytes4(bc.DenebForkVersion),
		},
		{
			name:        "electra",
			version:     version.Electra,
			slot:        electraSlot,
			forkversion: bytesutil.ToBytes4(bc.ElectraForkVersion),
		},
		{
			name:        "fulu",
			version:     version.Fulu,
			slot:        fuluSlot,
			forkversion: bytesutil.ToBytes4(bc.FuluForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		require.NoError(t, st.SetFork(&ethpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  c.forkversion[:],
			Epoch:           0,
		}))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		s, err := cf.UnmarshalBeaconState(m)
		require.NoError(t, err)
		expected, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		actual, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, expected, actual)
	}
}

func TestDetectAndUnmarshalBlock(t *testing.T) {
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	altairS, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
	require.NoError(t, err)
	bellaS, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
	require.NoError(t, err)
	capellaS, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	require.NoError(t, err)
	denebS, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	electraS, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	fuluS, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		b         func(*testing.T, primitives.Slot) interfaces.ReadOnlySignedBeaconBlock
		name      string
		slot      primitives.Slot
		errExists bool
	}{
		{
			name: "genesis - slot 0",
			b:    signedTestBlockGenesis,
		},
		{
			name: "last slot of phase 0",
			b:    signedTestBlockGenesis,
			slot: altairS - 1,
		},
		{
			name: "first slot of altair",
			b:    signedTestBlockAltair,
			slot: altairS,
		},
		{
			name: "last slot of altair",
			b:    signedTestBlockAltair,
			slot: bellaS - 1,
		},
		{
			name: "first slot of bellatrix",
			b:    signedTestBlockBellatrix,
			slot: bellaS,
		},
		{
			name: "first slot of capella",
			b:    signedTestBlockCapella,
			slot: capellaS,
		},
		{
			name: "last slot of capella",
			b:    signedTestBlockCapella,
			slot: denebS - 1,
		},
		{
			name: "first slot of deneb",
			b:    signedTestBlockDeneb,
			slot: denebS,
		},
		{
			name: "first slot of electra",
			b:    signedTestBlockElectra,
			slot: electraS,
		},
		{
			name: "first slot of fulu",
			b:    signedTestBlockFulu,
			slot: fuluS,
		},
		{
			name:      "bellatrix block in altair slot",
			b:         signedTestBlockBellatrix,
			slot:      bellaS - 1,
			errExists: true,
		},
		{
			name:      "genesis block in altair slot",
			b:         signedTestBlockGenesis,
			slot:      bellaS - 1,
			errExists: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromBlock(marshaled)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBeaconBlock(marshaled)
			if c.errExists {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestUnmarshalBlock(t *testing.T) {
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	genv := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)
	altairv := bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion)
	bellav := bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion)
	capellaV := bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion)
	denebV := bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion)
	electraV := bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion)
	fuluV := bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion)
	altairS, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
	require.NoError(t, err)
	bellaS, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
	require.NoError(t, err)
	capellaS, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	require.NoError(t, err)
	denebS, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	electraS, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	fuluS, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		b       func(*testing.T, primitives.Slot) interfaces.ReadOnlySignedBeaconBlock
		name    string
		version [4]byte
		slot    primitives.Slot
		err     error
	}{
		{
			name:    "genesis - slot 0",
			b:       signedTestBlockGenesis,
			version: genv,
		},
		{
			name:    "last slot of phase 0",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    altairS - 1,
		},
		{
			name:    "first slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    altairS,
		},
		{
			name:    "last slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    bellaS - 1,
		},
		{
			name:    "first slot of bellatrix",
			b:       signedTestBlockBellatrix,
			version: bellav,
			slot:    bellaS,
		},
		{
			name:    "first slot of capella",
			b:       signedTestBlockCapella,
			version: capellaV,
			slot:    capellaS,
		},
		{
			name:    "last slot of capella",
			b:       signedTestBlockCapella,
			version: capellaV,
			slot:    denebS - 1,
		},
		{
			name:    "first slot of deneb",
			b:       signedTestBlockDeneb,
			version: denebV,
			slot:    denebS,
		},
		{
			name:    "first slot of electra",
			b:       signedTestBlockElectra,
			version: electraV,
			slot:    electraS,
		},
		{
			name:    "first slot of fulu",
			b:       signedTestBlockFulu,
			version: fuluV,
			slot:    fuluS,
		},
		{
			name:    "bellatrix block in altair slot",
			b:       signedTestBlockBellatrix,
			version: bellav,
			slot:    bellaS - 1,
			err:     errBlockForkMismatch,
		},
		{
			name:    "genesis block in altair slot",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    bellaS - 1,
			err:     errBlockForkMismatch,
		},
		{
			name:    "altair block in genesis slot",
			b:       signedTestBlockAltair,
			version: altairv,
			err:     errBlockForkMismatch,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromForkVersion(c.version)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBeaconBlock(marshaled)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestUnmarshalBlindedBlock(t *testing.T) {
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	genv := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)
	altairv := bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion)
	bellav := bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion)
	capellaV := bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion)
	denebV := bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion)
	electraV := bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion)
	fuluV := bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion)
	altairS, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
	require.NoError(t, err)
	bellaS, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
	require.NoError(t, err)
	capellaS, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	require.NoError(t, err)
	denebS, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	electraS, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	fuluS, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		b       func(*testing.T, primitives.Slot) interfaces.ReadOnlySignedBeaconBlock
		name    string
		version [4]byte
		slot    primitives.Slot
		err     error
	}{
		{
			name:    "genesis - slot 0",
			b:       signedTestBlockGenesis,
			version: genv,
		},
		{
			name:    "last slot of phase 0",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    altairS - 1,
		},
		{
			name:    "first slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    altairS,
		},
		{
			name:    "last slot of altair",
			b:       signedTestBlockAltair,
			version: altairv,
			slot:    bellaS - 1,
		},
		{
			name:    "first slot of bellatrix",
			b:       signedTestBlindedBlockBellatrix,
			version: bellav,
			slot:    bellaS,
		},
		{
			name:    "bellatrix block in altair slot",
			b:       signedTestBlindedBlockBellatrix,
			version: bellav,
			slot:    bellaS - 1,
			err:     errBlockForkMismatch,
		},
		{
			name:    "first slot of capella",
			b:       signedTestBlindedBlockCapella,
			version: capellaV,
			slot:    capellaS,
		},
		{
			name:    "last slot of capella",
			b:       signedTestBlindedBlockCapella,
			version: capellaV,
			slot:    denebS - 1,
		},
		{
			name:    "first slot of deneb",
			b:       signedTestBlindedBlockDeneb,
			version: denebV,
			slot:    denebS,
		},
		{
			name:    "first slot of electra",
			b:       signedTestBlindedBlockElectra,
			version: electraV,
			slot:    electraS,
		},
		{
			name:    "first slot of fulu",
			b:       signedTestBlindedBlockFulu,
			version: fuluV,
			slot:    fuluS,
		},
		{
			name:    "genesis block in altair slot",
			b:       signedTestBlockGenesis,
			version: genv,
			slot:    bellaS - 1,
			err:     errBlockForkMismatch,
		},
		{
			name:    "altair block in genesis slot",
			b:       signedTestBlockAltair,
			version: altairv,
			err:     errBlockForkMismatch,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromForkVersion(c.version)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBlindedBeaconBlock(marshaled)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

// ----------------------------------------------------------------------------
// Phase 0
// ----------------------------------------------------------------------------

func signedTestBlockGenesis(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlock()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Altair
// ----------------------------------------------------------------------------

func signedTestBlockAltair(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockAltair()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Bellatrix
// ----------------------------------------------------------------------------

func signedTestBlockBellatrix(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockBellatrix()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockBellatrix(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockBellatrix()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Capella
// ----------------------------------------------------------------------------

func signedTestBlockCapella(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockCapella()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockCapella(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockCapella()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Deneb
// ----------------------------------------------------------------------------

func signedTestBlockDeneb(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockDeneb()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockDeneb(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockDeneb()
	b.Message.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Electra
// ----------------------------------------------------------------------------

func signedTestBlockElectra(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockElectra()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockElectra(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockElectra()
	b.Message.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

// ----------------------------------------------------------------------------
// Fulu
// ----------------------------------------------------------------------------

func signedTestBlockFulu(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockFulu()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockFulu(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockFulu()
	b.Message.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func TestUnmarshalStateStandalone(t *testing.T) {
	ctx := t.Context()
	defer util.HackForksMaxuint(t, []int{version.Electra, version.Fulu})()

	bc := params.BeaconConfig()
	altairSlot, err := slots.EpochStart(bc.AltairForkEpoch)
	require.NoError(t, err)
	bellaSlot, err := slots.EpochStart(bc.BellatrixForkEpoch)
	require.NoError(t, err)
	capellaSlot, err := slots.EpochStart(bc.CapellaForkEpoch)
	require.NoError(t, err)
	denebSlot, err := slots.EpochStart(bc.DenebForkEpoch)
	require.NoError(t, err)
	electraSlot, err := slots.EpochStart(bc.ElectraForkEpoch)
	require.NoError(t, err)
	fuluSlot, err := slots.EpochStart(bc.FuluForkEpoch)
	require.NoError(t, err)

	cases := []struct {
		name        string
		version     int
		slot        primitives.Slot
		forkversion [4]byte
	}{
		{
			name:        "phase0",
			version:     version.Phase0,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
		{
			name:        "altair",
			version:     version.Altair,
			slot:        altairSlot,
			forkversion: bytesutil.ToBytes4(bc.AltairForkVersion),
		},
		{
			name:        "bellatrix",
			version:     version.Bellatrix,
			slot:        bellaSlot,
			forkversion: bytesutil.ToBytes4(bc.BellatrixForkVersion),
		},
		{
			name:        "capella",
			version:     version.Capella,
			slot:        capellaSlot,
			forkversion: bytesutil.ToBytes4(bc.CapellaForkVersion),
		},
		{
			name:        "deneb",
			version:     version.Deneb,
			slot:        denebSlot,
			forkversion: bytesutil.ToBytes4(bc.DenebForkVersion),
		},
		{
			name:        "electra",
			version:     version.Electra,
			slot:        electraSlot,
			forkversion: bytesutil.ToBytes4(bc.ElectraForkVersion),
		},
		{
			name:        "fulu",
			version:     version.Fulu,
			slot:        fuluSlot,
			forkversion: bytesutil.ToBytes4(bc.FuluForkVersion),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Create a state for the specific version
			originalState, err := stateForVersion(c.version)
			require.NoError(t, err)
			require.NoError(t, originalState.SetFork(&ethpb.Fork{
				PreviousVersion: make([]byte, 4),
				CurrentVersion:  c.forkversion[:],
				Epoch:           0,
			}))
			require.NoError(t, originalState.SetSlot(c.slot))

			marshaled, err := originalState.MarshalSSZ()
			require.NoError(t, err)

			unmarshaledState, err := UnmarshalState(marshaled)
			require.NoError(t, err)
			require.NotNil(t, unmarshaledState)

			// Verify the unmarshaled state matches the original
			expectedRoot, err := originalState.HashTreeRoot(ctx)
			require.NoError(t, err)
			actualRoot, err := unmarshaledState.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, expectedRoot, actualRoot)

			// Verify basic state properties
			require.Equal(t, c.slot, unmarshaledState.Slot())
			fork := unmarshaledState.Fork()
			require.DeepEqual(t, c.forkversion[:], fork.CurrentVersion)
		})
	}

	t.Run("invalid state data", func(t *testing.T) {
		invalidData := []byte("bad")
		_, err := UnmarshalState(invalidData)
		require.ErrorContains(t, "failed to detect version from state", err)
	})
}
