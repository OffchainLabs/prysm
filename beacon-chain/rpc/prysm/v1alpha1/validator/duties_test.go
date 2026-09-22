//go:build minimal

package validator

import (
	"encoding/binary"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// pubKey is a helper to generate a well-formed public key.
func pubKey(i uint64) []byte {
	pubKey := make([]byte, params.BeaconConfig().BLSPubkeyLength)
	binary.LittleEndian.PutUint64(pubKey, i)
	return pubKey
}

func TestGetDuties_OK(t *testing.T) {
	genesis := util.NewBeaconBlock()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))
	vs := &Server{
		BeaconDB:          db,
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	// Test the first validator in registry.
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// Test the last validator in registry.
	lastValidatorIndex := depChainStart - 1
	req = &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[lastValidatorIndex].Data.PublicKey},
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// We request for duties for all validators.
	req = &ethpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      0,
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}
}

func TestGetAltairDuties_SyncCommitteeOK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	params.OverrideBeaconConfig(cfg)

	genesis := util.NewBeaconBlock()
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err, "Could not setup genesis bs")
	h := &ethpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	syncCommittee, err := altair.NextSyncCommittee(t.Context(), bs)
	require.NoError(t, err)
	require.NoError(t, bs.SetCurrentSyncCommittee(syncCommittee))
	pubKeys := make([][]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}
	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	pubkeysAs48ByteType := make([][fieldparams.BLSPubkeyLength]byte, len(pubKeys))
	for i, pk := range pubKeys {
		pubkeysAs48ByteType[i] = bytesutil.ToBytes48(pk)
	}

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	vs := &Server{
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		Eth1InfoFetcher:   &mockExecution.Chain{},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	// Test the first validator in registry.
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// Test the last validator in registry.
	lastValidatorIndex := params.BeaconConfig().SyncCommitteeSize - 1
	req = &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[lastValidatorIndex].Data.PublicKey},
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// We request for duties for all validators.
	req = &ethpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      0,
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.Equal(t, true, res.CurrentEpochDuties[i].IsSyncCommittee)
		// Current epoch and next epoch duties should be equal before the sync period epoch boundary.
		require.Equal(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}

	// Current epoch and next epoch duties should not be equal at the sync period epoch boundary.
	req = &ethpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1,
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.NotEqual(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}
}

func TestGetBellatrixDuties_SyncCommitteeOK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	cfg.BellatrixForkEpoch = primitives.Epoch(1)
	params.OverrideBeaconConfig(cfg)

	genesis := util.NewBeaconBlock()
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	h := &ethpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	syncCommittee, err := altair.NextSyncCommittee(t.Context(), bs)
	require.NoError(t, err)
	require.NoError(t, bs.SetCurrentSyncCommittee(syncCommittee))
	pubKeys := make([][]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}
	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	bs, err = execution.UpgradeToBellatrix(bs)
	require.NoError(t, err)

	pubkeysAs48ByteType := make([][fieldparams.BLSPubkeyLength]byte, len(pubKeys))
	for i, pk := range pubKeys {
		pubkeysAs48ByteType[i] = bytesutil.ToBytes48(pk)
	}

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	vs := &Server{
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		Eth1InfoFetcher:   &mockExecution.Chain{},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	// Test the first validator in registry.
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// Test the last validator in registry.
	lastValidatorIndex := params.BeaconConfig().SyncCommitteeSize - 1
	req = &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[lastValidatorIndex].Data.PublicKey},
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// We request for duties for all validators.
	req = &ethpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      0,
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, true, res.CurrentEpochDuties[i].IsSyncCommittee)
		// Current epoch and next epoch duties should be equal before the sync period epoch boundary.
		assert.Equal(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}

	// Current epoch and next epoch duties should not be equal at the sync period epoch boundary.
	req = &ethpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1,
	}
	res, err = vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.NotEqual(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}
}

func TestGetAltairDuties_UnknownPubkey(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	params.OverrideBeaconConfig(cfg)

	genesis := util.NewBeaconBlock()
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)
	h := &ethpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)

	vs := &Server{
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		TimeFetcher:       chain,
		Eth1InfoFetcher:   &mockExecution.Chain{},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		DepositFetcher:    depositCache,
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	unknownPubkey := bytesutil.PadTo([]byte{'u'}, 48)
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{unknownPubkey},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, false, res.CurrentEpochDuties[0].IsSyncCommittee)
	assert.Equal(t, false, res.NextEpochDuties[0].IsSyncCommittee)
}

func TestGetDuties_SlotOutOfUpperBound(t *testing.T) {
	chain := &mockChain.ChainService{
		Genesis: time.Now(),
	}
	vs := &Server{
		ForkchoiceFetcher: chain,
		TimeFetcher:       chain,
	}
	req := &ethpb.DutiesRequest{
		Epoch: primitives.Epoch(chain.CurrentSlot()/params.BeaconConfig().SlotsPerEpoch + 2),
	}
	_, err := vs.duties(t.Context(), req)
	require.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetDuties_CurrentEpoch_ShouldNotFail(t *testing.T) {
	genesis := util.NewBeaconBlock()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bState, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err, "Could not setup genesis state")
	// Set state to non-epoch start slot.
	require.NoError(t, bState.SetSlot(5))

	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes48(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bState, Root: genesisRoot[:], Genesis: time.Now(),
	}
	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))
	vs := &Server{
		BeaconDB:          db,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		TimeFetcher:       chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	// Test the first validator in registry.
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(res.CurrentEpochDuties), "Expected 1 assignment")
}

func TestGetDuties_MultipleKeys_OK(t *testing.T) {
	genesis := util.NewBeaconBlock()
	depChainStart := uint64(64)

	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes48(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))
	vs := &Server{
		BeaconDB:          db,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		TimeFetcher:       chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		PayloadIDCache:    cache.NewPayloadIDCache(),
	}

	pubkey0 := deposits[0].Data.PublicKey
	pubkey1 := deposits[1].Data.PublicKey

	// Test the first validator in registry.
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{pubkey0, pubkey1},
	}
	res, err := vs.GetDuties(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	assert.Equal(t, 2, len(res.CurrentEpochDuties))
	assert.Equal(t, primitives.Slot(4), res.CurrentEpochDuties[0].AttesterSlot)
	assert.Equal(t, primitives.Slot(4), res.CurrentEpochDuties[1].AttesterSlot)
}

func TestGetDuties_SyncNotReady(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetDuties(t.Context(), &ethpb.DutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func BenchmarkCommitteeAssignment(b *testing.B) {

	genesis := util.NewBeaconBlock()
	depChainStart := uint64(8192 * 2)
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(b, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(b, err)
	bs, err := transition.GenesisBeaconState(b.Context(), deposits, 0, eth1Data)
	require.NoError(b, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(b, err, "Could not get signing root")

	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes48(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{State: bs, Root: genesisRoot[:], Genesis: time.Now()}
	db := dbutil.SetupDB(b)
	require.NoError(b, db.SaveGenesisBlockRoot(b.Context(), genesisRoot))
	vs := &Server{
		BeaconDB:    db,
		HeadFetcher: chain,
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	// Create request for all validators in the system.
	pks := make([][]byte, len(deposits))
	for i, deposit := range deposits {
		pks[i] = deposit.Data.PublicKey
	}
	req := &ethpb.DutiesRequest{
		PublicKeys: pks,
		Epoch:      0,
	}

	for b.Loop() {
		_, err := vs.GetDuties(b.Context(), req)
		assert.NoError(b, err)
	}
}

func TestGetDuties_DependentRootsFromState(t *testing.T) {
	spe := params.BeaconConfig().SlotsPerEpoch
	for _, tt := range []struct {
		name                           string
		headSlot, stateSlot, clockSlot primitives.Slot
		requestEpoch                   primitives.Epoch
		wantPrevSlot, wantCurrSlot     primitives.Slot
	}{
		{name: "genesis"},
		{name: "first epoch", headSlot: spe.Sub(1), stateSlot: spe, clockSlot: spe, requestEpoch: 1, wantCurrSlot: spe.Sub(1)},
		{name: "checkpoint boundary", headSlot: 2 * spe, stateSlot: 2 * spe, clockSlot: 2 * spe, requestEpoch: 2, wantPrevSlot: spe.Sub(1), wantCurrSlot: (2 * spe).Sub(1)},
		{name: "skipped checkpoint boundary", headSlot: (2 * spe).Sub(1), stateSlot: 2 * spe, clockSlot: 2 * spe, requestEpoch: 2, wantPrevSlot: spe.Sub(1), wantCurrSlot: (2 * spe).Sub(1)},
		{name: "next epoch request keeps clock epoch roots", headSlot: (3 * spe).Sub(1), stateSlot: (3 * spe).Sub(1), clockSlot: (3 * spe).Sub(1), requestEpoch: 3, wantPrevSlot: spe.Sub(1), wantCurrSlot: (2 * spe).Sub(1)},
		{name: "clock crossed epoch boundary", headSlot: (3 * spe).Sub(1), stateSlot: (3 * spe).Sub(1), clockSlot: 3 * spe, requestEpoch: 2, wantPrevSlot: (2 * spe).Sub(1), wantCurrSlot: (3 * spe).Sub(1)},
		{name: "both boundaries after head", headSlot: (3 * spe).Sub(1), stateSlot: (3 * spe).Sub(1), clockSlot: 4 * spe, requestEpoch: 2, wantPrevSlot: (3 * spe).Sub(1), wantCurrSlot: (3 * spe).Sub(1)},
		{name: "historical request keeps clock epoch roots", headSlot: 3 * spe, stateSlot: 3 * spe, clockSlot: 3 * spe, requestEpoch: 1, wantPrevSlot: (2 * spe).Sub(1), wantCurrSlot: (3 * spe).Sub(1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := dbutil.SetupDB(t)
			st, genesisRoot, keys := util.DeterministicGenesisStateWithGenesisBlock(t, ctx, db, 64)
			require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
			roots := map[primitives.Slot][32]byte{0: genesisRoot}
			for _, slot := range []primitives.Slot{spe.Sub(1), (2 * spe).Sub(1), 2 * spe, (3 * spe).Sub(1), 3 * spe} {
				if slot > tt.headSlot {
					break
				}
				signed, err := util.GenerateFullBlock(st, keys, &util.BlockGenConfig{}, slot)
				require.NoError(t, err)
				blk, err := blocks.NewSignedBeaconBlock(signed)
				require.NoError(t, err)
				st, err = transition.ExecuteStateTransition(ctx, st, blk)
				require.NoError(t, err)
				roots[slot], err = blk.Block().HashTreeRoot()
				require.NoError(t, err)
			}
			if st.Slot() < tt.stateSlot {
				var err error
				st, err = transition.ProcessSlots(ctx, st, tt.stateSlot)
				require.NoError(t, err)
			}
			before, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)
			headRoot := roots[tt.headSlot]
			chain := &mockChain.ChainService{State: st, Root: headRoot[:], Slot: &tt.clockSlot}
			// No forkchoice fetcher: roots must come from the duties state.
			vs := &Server{BeaconDB: db, HeadFetcher: chain, TimeFetcher: chain, SyncChecker: &mockSync.Sync{}}
			pubkey := st.PubkeyAtIndex(0)
			req := &ethpb.DutiesRequest{PublicKeys: [][]byte{pubkey[:]}, Epoch: tt.requestEpoch}
			prev, curr := roots[tt.wantPrevSlot], roots[tt.wantCurrSlot]
			res, err := vs.GetDuties(ctx, req)
			require.NoError(t, err)
			require.DeepEqual(t, prev[:], res.PreviousDutyDependentRoot)
			require.DeepEqual(t, curr[:], res.CurrentDutyDependentRoot)
			resV2, err := vs.GetDutiesV2(ctx, req)
			require.NoError(t, err)
			require.DeepEqual(t, prev[:], resV2.PreviousDutyDependentRoot)
			require.DeepEqual(t, curr[:], resV2.CurrentDutyDependentRoot)
			after, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
