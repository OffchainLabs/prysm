package validator

import (
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestGetDutiesV2_OK(t *testing.T) {
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
	for i := 0; i < len(deposits); i++ {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
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
	res, err := vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}
}

func TestGetAltairDutiesV2_SyncCommitteeOK(t *testing.T) {
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
	for i := 0; i < len(deposits); i++ {
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
	res, err := vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.NotEqual(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}
}

func TestGetBellatrixDutiesV2_SyncCommitteeOK(t *testing.T) {
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
	for i := 0; i < len(deposits); i++ {
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
	res, err := vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
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
	res, err = vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.NotEqual(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}
}

func TestGetAltairDutiesV2_UnknownPubkey(t *testing.T) {
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
	res, err := vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, false, res.CurrentEpochDuties[0].IsSyncCommittee)
	assert.Equal(t, false, res.NextEpochDuties[0].IsSyncCommittee)
}

func TestGetDutiesV2_StateAdvancement(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = primitives.Epoch(0)
	params.OverrideBeaconConfig(cfg)

	epochStart, err := slots.EpochStart(1)
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisStateElectra(t, 1)
	require.NoError(t, st.SetSlot(epochStart-1))

	// Request epoch 1 which requires slot 32 processing
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{pubKey(0)},
		Epoch:      1,
	}
	b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockElectra(&ethpb.SignedBeaconBlockElectra{}))
	require.NoError(t, err)
	b.SetSlot(epochStart)
	currentSlot := epochStart - 1
	// Mock chain service with state at slot 0
	chain := &mockChain.ChainService{
		Root:  make([]byte, 32),
		State: st,
		Block: b,
		Slot:  &currentSlot,
	}

	vs := &Server{
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
	}

	// Verify state processing occurs
	res, err := vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestGetDutiesV2_SlotOutOfUpperBound(t *testing.T) {
	chain := &mockChain.ChainService{
		Genesis: time.Now(),
	}
	vs := &Server{
		ForkchoiceFetcher: chain,
		TimeFetcher:       chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
	}
	req := &ethpb.DutiesRequest{
		Epoch: primitives.Epoch(chain.CurrentSlot()/params.BeaconConfig().SlotsPerEpoch + 2),
	}
	_, err := vs.GetDutiesV2(t.Context(), req)
	require.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetDutiesV2_CurrentEpoch_ShouldNotFail(t *testing.T) {
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
	for i := 0; i < len(deposits); i++ {
		pubKeys[i] = bytesutil.ToBytes48(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bState, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
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
	res, err := vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(res.CurrentEpochDuties), "Expected 1 assignment")
}

func TestGetDutiesV2_MultipleKeys_OK(t *testing.T) {
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
	for i := 0; i < len(deposits); i++ {
		pubKeys[i] = bytesutil.ToBytes48(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
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
	res, err := vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	assert.Equal(t, 2, len(res.CurrentEpochDuties))
	assert.Equal(t, primitives.Slot(4), res.CurrentEpochDuties[0].AttesterSlot)
	assert.Equal(t, primitives.Slot(4), res.CurrentEpochDuties[1].AttesterSlot)
}

func TestGetDutiesV2_NextSyncCommitteePeriod(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	cfg.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(cfg)

	// Configure sync committee period boundary
	epochsPerPeriod := params.BeaconConfig().EpochsPerSyncCommitteePeriod
	boundaryEpoch := epochsPerPeriod - 1

	// Create state at last epoch of current period
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	st, err := util.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)

	syncCommittee, err := altair.NextSyncCommittee(t.Context(), st)
	require.NoError(t, err)
	require.NoError(t, st.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(boundaryEpoch)))

	validatorPubkey := deposits[0].Data.PublicKey

	// Request duties for boundary epoch + 1
	req := &ethpb.DutiesRequest{
		PublicKeys: [][]byte{validatorPubkey},
		Epoch:      boundaryEpoch + 1,
	}

	genesisRoot := [32]byte{}
	chain := &mockChain.ChainService{
		State: st,
		Root:  genesisRoot[:],
	}
	vs := &Server{
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
	}

	res, err := vs.GetDutiesV2(t.Context(), req)
	require.NoError(t, err)

	//Verify next epoch duties have updated sync committee status
	require.NotEqual(t,
		res.CurrentEpochDuties[0].IsSyncCommittee,
		res.NextEpochDuties[0].IsSyncCommittee,
	)
}

func TestGetDutiesV2_SyncNotReady(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetDutiesV2(t.Context(), &ethpb.DutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetValidatorAssignment(t *testing.T) {
	start := primitives.Slot(100)

	// Test using CommitteeAssignments
	committeeAssignments := map[primitives.ValidatorIndex]*helpers.CommitteeAssignment{
		5: {
			Committee:      []primitives.ValidatorIndex{4, 5, 6},
			AttesterSlot:   start + 1,
			CommitteeIndex: primitives.CommitteeIndex(0),
		},
	}

	meta := &metadata{
		committeeAssignments: committeeAssignments,
	}

	vs := &Server{}

	// Test existing validator
	assignment := vs.getValidatorAssignment(meta, primitives.ValidatorIndex(5))
	require.NotNil(t, assignment)
	assert.Equal(t, start+1, assignment.AttesterSlot)
	assert.Equal(t, primitives.CommitteeIndex(0), assignment.CommitteeIndex)
	assert.Equal(t, uint64(1), assignment.ValidatorCommitteeIndex)

	// Test non-existent validator should return empty assignment
	assignment = vs.getValidatorAssignment(meta, primitives.ValidatorIndex(99))
	require.NotNil(t, assignment)
	assert.Equal(t, primitives.Slot(0), assignment.AttesterSlot)
	assert.Equal(t, primitives.CommitteeIndex(0), assignment.CommitteeIndex)
}
