package slasherkv

import (
	"encoding/binary"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	slashertypes "github.com/OffchainLabs/prysm/v6/beacon-chain/slasher/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestStore_AttestationRecordForValidator_SaveRetrieve(t *testing.T) {
	const attestationsCount = 11_000
	ctx := t.Context()
	beaconDB := setupDB(t)
	phase0ValidatorIndex := primitives.ValidatorIndex(1)
	electraValidatorIndex := primitives.ValidatorIndex(2)

	// Defines attestations to save and retrieve.
	attWrappers := make([]*slashertypes.IndexedAttestationWrapper, attestationsCount)
	for i := 0; i < attestationsCount; i++ {
		var dataRoot [32]byte
		binary.LittleEndian.PutUint64(dataRoot[:], uint64(i))

		attWrapper := createAttestationWrapper(
			version.Phase0,
			primitives.Epoch(i),
			primitives.Epoch(i+1),
			[]uint64{uint64(phase0ValidatorIndex)},
			dataRoot[:],
		)

		attWrappers[i] = attWrapper
	}
	attWrappersElectra := make([]*slashertypes.IndexedAttestationWrapper, attestationsCount)
	for i := 0; i < attestationsCount; i++ {
		var dataRoot [32]byte
		binary.LittleEndian.PutUint64(dataRoot[:], uint64(i))

		attWrapper := createAttestationWrapper(
			version.Electra,
			primitives.Epoch(i),
			primitives.Epoch(i+1),
			[]uint64{uint64(electraValidatorIndex)},
			dataRoot[:],
		)

		attWrappersElectra[i] = attWrapper
	}

	type testCase struct {
		name string
		atts []*slashertypes.IndexedAttestationWrapper
		vi   primitives.ValidatorIndex
	}
	testCases := []testCase{
		{
			name: "phase0",
			atts: attWrappers,
			vi:   phase0ValidatorIndex,
		},
		{
			name: "electra",
			atts: attWrappersElectra,
			vi:   electraValidatorIndex,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Check on a sample of validators that no attestation records are available.
			for i := 0; i < attestationsCount; i += 100 {
				attRecord, err := beaconDB.AttestationRecordForValidator(ctx, tc.vi, primitives.Epoch(i+1))
				require.NoError(t, err)
				require.Equal(t, true, attRecord == nil)
			}

			// Save the attestation records to the database.
			err := beaconDB.SaveAttestationRecordsForValidators(ctx, tc.atts)
			require.NoError(t, err)

			// Check on a sample of validators that attestation records are available.
			for i := 0; i < attestationsCount; i += 100 {
				expected := attWrappers[i]
				actual, err := beaconDB.AttestationRecordForValidator(ctx, tc.vi, primitives.Epoch(i+1))
				require.NoError(t, err)

				require.DeepEqual(t, expected.IndexedAttestation.GetData().Source.Epoch, actual.IndexedAttestation.GetData().Source.Epoch)
			}
		})
	}
}

func TestStore_LastEpochWrittenForValidators(t *testing.T) {
	ctx := t.Context()
	beaconDB := setupDB(t)

	validatorsCount := 11000
	indices := make([]primitives.ValidatorIndex, validatorsCount)
	epochs := make([]primitives.Epoch, validatorsCount)

	for i := 0; i < validatorsCount; i++ {
		indices[i] = primitives.ValidatorIndex(i)
		epochs[i] = primitives.Epoch(i)
	}

	epochsByValidator := make(map[primitives.ValidatorIndex]primitives.Epoch, validatorsCount)
	for i := 0; i < validatorsCount; i++ {
		epochsByValidator[indices[i]] = epochs[i]
	}

	// No epochs written for any validators, should return empty list.
	attestedEpochs, err := beaconDB.LastEpochWrittenForValidators(ctx, indices)
	require.NoError(t, err)
	require.Equal(t, 0, len(attestedEpochs))

	err = beaconDB.SaveLastEpochWrittenForValidators(ctx, epochsByValidator)
	require.NoError(t, err)

	retrievedEpochs, err := beaconDB.LastEpochWrittenForValidators(ctx, indices)
	require.NoError(t, err)
	require.Equal(t, len(indices), len(retrievedEpochs))

	for i, retrievedEpoch := range retrievedEpochs {
		want := &slashertypes.AttestedEpochForValidator{
			Epoch:          epochs[i],
			ValidatorIndex: indices[i],
		}

		require.DeepEqual(t, want, retrievedEpoch)
	}
}

func TestStore_CheckAttesterDoubleVotes(t *testing.T) {
	ctx := t.Context()

	for _, ver := range []int{version.Phase0, version.Electra} {
		t.Run(version.String(ver), func(t *testing.T) {
			beaconDB := setupDB(t)
			err := beaconDB.SaveAttestationRecordsForValidators(ctx, []*slashertypes.IndexedAttestationWrapper{
				createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{1}),
				createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{3}),
			})
			require.NoError(t, err)

			slashableAtts := []*slashertypes.IndexedAttestationWrapper{
				createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{2}), // Different signing root.
				createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{4}), // Different signing root.
			}

			wanted := []*slashertypes.AttesterDoubleVote{
				{
					ValidatorIndex: 0,
					Target:         3,
					Wrapper_1:      createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{1}),
					Wrapper_2:      createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{2}),
				},
				{
					ValidatorIndex: 1,
					Target:         3,
					Wrapper_1:      createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{1}),
					Wrapper_2:      createAttestationWrapper(version.Phase0, 2, 3, []uint64{0, 1}, []byte{2}),
				},
				{
					ValidatorIndex: 2,
					Target:         4,
					Wrapper_1:      createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{3}),
					Wrapper_2:      createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{4}),
				},
				{
					ValidatorIndex: 3,
					Target:         4,
					Wrapper_1:      createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{3}),
					Wrapper_2:      createAttestationWrapper(version.Phase0, 3, 4, []uint64{2, 3}, []byte{4}),
				},
			}
			doubleVotes, err := beaconDB.CheckAttesterDoubleVotes(ctx, slashableAtts)
			require.NoError(t, err)
			sort.SliceStable(doubleVotes, func(i, j int) bool {
				return uint64(doubleVotes[i].ValidatorIndex) < uint64(doubleVotes[j].ValidatorIndex)
			})
			require.Equal(t, len(wanted), len(doubleVotes))
			for i, double := range doubleVotes {
				require.DeepEqual(t, wanted[i].ValidatorIndex, double.ValidatorIndex)
				require.DeepEqual(t, wanted[i].Target, double.Target)
				require.DeepEqual(t, wanted[i].Wrapper_1, double.Wrapper_1)
				require.DeepEqual(t, wanted[i].Wrapper_2, double.Wrapper_2)
			}
		})
	}
}

func TestStore_SlasherChunk_SaveRetrieve(t *testing.T) {
	// Define test parameters.
	const (
		elemsPerChunk = 16
		totalChunks   = 11_000
	)

	// Create context.
	ctx := t.Context()

	// Create database.
	beaconDB := setupDB(t)

	// Create min chunk keys and chunks.
	minChunkKeys := make([][]byte, totalChunks)
	minChunks := make([][]uint16, totalChunks)

	for i := 0; i < totalChunks; i++ {
		// Create chunk key.
		chunkKey := ssz.MarshalUint64(make([]byte, 0), uint64(i))
		minChunkKeys[i] = chunkKey

		// Create chunk.
		chunk := make([]uint16, elemsPerChunk)

		for j := 0; j < len(chunk); j++ {
			chunk[j] = uint16(i + j)
		}

		minChunks[i] = chunk
	}

	// Create max chunk keys and chunks.
	maxChunkKeys := make([][]byte, totalChunks)
	maxChunks := make([][]uint16, totalChunks)

	for i := 0; i < totalChunks; i++ {
		// Create chunk key.
		chunkKey := ssz.MarshalUint64(make([]byte, 0), uint64(i+1))
		maxChunkKeys[i] = chunkKey

		// Create chunk.
		chunk := make([]uint16, elemsPerChunk)

		for j := 0; j < len(chunk); j++ {
			chunk[j] = uint16(i + j + 1)
		}

		maxChunks[i] = chunk
	}

	// Save chunks for min spans.
	err := beaconDB.SaveSlasherChunks(ctx, slashertypes.MinSpan, minChunkKeys, minChunks)
	require.NoError(t, err)

	// Expect no chunks to be stored for max spans.
	_, chunksExist, err := beaconDB.LoadSlasherChunks(
		ctx, slashertypes.MaxSpan, minChunkKeys,
	)
	require.NoError(t, err)
	require.Equal(t, len(minChunks), len(chunksExist))

	for _, exists := range chunksExist {
		require.Equal(t, false, exists)
	}

	// Check the right chunks are saved.
	retrievedChunks, chunksExist, err := beaconDB.LoadSlasherChunks(
		ctx, slashertypes.MinSpan, minChunkKeys,
	)
	require.NoError(t, err)
	require.Equal(t, len(minChunks), len(retrievedChunks))
	require.Equal(t, len(minChunks), len(chunksExist))

	for i, exists := range chunksExist {
		require.Equal(t, true, exists)
		require.DeepEqual(t, minChunks[i], retrievedChunks[i])
	}

	// Save chunks for max spans.
	err = beaconDB.SaveSlasherChunks(ctx, slashertypes.MaxSpan, maxChunkKeys, maxChunks)
	require.NoError(t, err)

	// Check right chunks are saved.
	retrievedChunks, chunksExist, err = beaconDB.LoadSlasherChunks(
		ctx, slashertypes.MaxSpan, maxChunkKeys,
	)
	require.NoError(t, err)

	require.Equal(t, len(maxChunks), len(retrievedChunks))
	require.Equal(t, len(maxChunks), len(chunksExist))

	for i, exists := range chunksExist {
		require.Equal(t, true, exists)
		require.DeepEqual(t, maxChunks[i], retrievedChunks[i])
	}

	// Check the right chunks are still saved for min span.
	retrievedChunks, chunksExist, err = beaconDB.LoadSlasherChunks(
		ctx, slashertypes.MinSpan, minChunkKeys,
	)
	require.NoError(t, err)
	require.Equal(t, len(minChunks), len(retrievedChunks))
	require.Equal(t, len(minChunks), len(chunksExist))

	for i, exists := range chunksExist {
		require.Equal(t, true, exists)
		require.DeepEqual(t, minChunks[i], retrievedChunks[i])
	}
}

func TestStore_SlasherChunk_PreventsSavingWrongLength(t *testing.T) {
	ctx := t.Context()
	beaconDB := setupDB(t)
	totalChunks := 64
	chunkKeys := make([][]byte, totalChunks)
	chunks := make([][]uint16, totalChunks)
	for i := 0; i < totalChunks; i++ {
		chunks[i] = []uint16{}
		chunkKeys[i] = ssz.MarshalUint64(make([]byte, 0), uint64(i))
	}
	// We should get an error if saving empty chunks.
	err := beaconDB.SaveSlasherChunks(ctx, slashertypes.MinSpan, chunkKeys, chunks)
	require.ErrorContains(t, "cannot encode empty chunk", err)
}

func TestStore_ExistingBlockProposals(t *testing.T) {
	ctx := t.Context()
	beaconDB := setupDB(t)
	proposals := []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 1, 1, []byte{1}),
		createProposalWrapper(t, 2, 1, []byte{1}),
		createProposalWrapper(t, 3, 1, []byte{1}),
	}
	// First time checking should return empty existing proposals.
	doubleProposals, err := beaconDB.CheckDoubleBlockProposals(ctx, proposals)
	require.NoError(t, err)
	require.Equal(t, 0, len(doubleProposals))

	// We then save the block proposals to disk.
	err = beaconDB.SaveBlockProposals(ctx, proposals)
	require.NoError(t, err)

	// Second time checking same proposals but all with different signing root should
	// return all double proposals.
	proposals[0].HeaderRoot = bytesutil.ToBytes32([]byte{2})
	proposals[1].HeaderRoot = bytesutil.ToBytes32([]byte{2})
	proposals[2].HeaderRoot = bytesutil.ToBytes32([]byte{2})

	doubleProposals, err = beaconDB.CheckDoubleBlockProposals(ctx, proposals)
	require.NoError(t, err)
	require.Equal(t, len(proposals), len(doubleProposals))
	for i, existing := range doubleProposals {
		require.DeepEqual(t, doubleProposals[i].Header_1, existing.Header_1)
	}
}

func Test_encodeDecodeProposalRecord(t *testing.T) {
	tests := []struct {
		name    string
		blkHdr  *slashertypes.SignedBlockHeaderWrapper
		wantErr bool
	}{
		{
			name:   "empty standard encode/decode",
			blkHdr: createProposalWrapper(t, 0, 0, nil),
		},
		{
			name:   "standard encode/decode",
			blkHdr: createProposalWrapper(t, 15, 6, []byte("1")),
		},
		{
			name: "failing encode/decode",
			blkHdr: &slashertypes.SignedBlockHeaderWrapper{
				SignedBeaconBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{},
				},
			},
			wantErr: true,
		},
		{
			name:    "failing empty encode/decode",
			blkHdr:  &slashertypes.SignedBlockHeaderWrapper{},
			wantErr: true,
		},
		{
			name:    "failing nil",
			blkHdr:  nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeProposalRecord(tt.blkHdr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("encodeProposalRecord() error = %v, wantErr %v", err, tt.wantErr)
			}
			decoded, err := decodeProposalRecord(got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeProposalRecord() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !reflect.DeepEqual(tt.blkHdr, decoded) {
				t.Errorf("Did not match got = %v, want %v", tt.blkHdr, decoded)
			}
		})
	}
}

func Test_encodeDecodeAttestationRecord(t *testing.T) {
	tests := []struct {
		name       string
		attWrapper *slashertypes.IndexedAttestationWrapper
		wantErr    bool
	}{
		{
			name:       "phase0 empty standard encode/decode",
			attWrapper: createAttestationWrapper(version.Phase0, 0, 0, nil /* indices */, nil /* signingRoot */),
		},
		{
			name:       "phase0 standard encode/decode",
			attWrapper: createAttestationWrapper(version.Phase0, 15, 6, []uint64{2, 4}, []byte("1") /* signingRoot */),
		},
		{
			name:       "electra empty standard encode/decode",
			attWrapper: createAttestationWrapper(version.Electra, 0, 0, nil /* indices */, nil /* signingRoot */),
		},
		{
			name:       "electra standard encode/decode",
			attWrapper: createAttestationWrapper(version.Electra, 15, 6, []uint64{2, 4}, []byte("1") /* signingRoot */),
		},
		{
			name: "failing encode/decode",
			attWrapper: &slashertypes.IndexedAttestationWrapper{
				IndexedAttestation: &ethpb.IndexedAttestation{
					Data: &ethpb.AttestationData{},
				},
			},
			wantErr: true,
		},
		{
			name:       "failing empty encode/decode",
			attWrapper: &slashertypes.IndexedAttestationWrapper{},
			wantErr:    true,
		},
		{
			name:       "failing nil",
			attWrapper: nil,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeAttestationRecord(tt.attWrapper)
			if (err != nil) != tt.wantErr {
				t.Fatalf("encodeAttestationRecord() error = %v, wantErr %v", err, tt.wantErr)
			}
			decoded, err := decodeAttestationRecord(got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeAttestationRecord() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !reflect.DeepEqual(tt.attWrapper, decoded) {
				t.Errorf("Did not match got = %v, want %v", tt.attWrapper, decoded)
			}
		})
	}
}

func TestStore_HighestAttestations(t *testing.T) {
	ctx := t.Context()
	tests := []struct {
		name             string
		attestationsInDB []*slashertypes.IndexedAttestationWrapper
		expected         []*ethpb.HighestAttestation
		indices          []primitives.ValidatorIndex
		wantErr          bool
	}{
		{
			name: "should get highest att if single att in db",
			attestationsInDB: []*slashertypes.IndexedAttestationWrapper{
				createAttestationWrapper(version.Phase0, 0, 3, []uint64{1}, []byte{1}),
			},
			indices: []primitives.ValidatorIndex{1},
			expected: []*ethpb.HighestAttestation{
				{
					ValidatorIndex:     1,
					HighestSourceEpoch: 0,
					HighestTargetEpoch: 3,
				},
			},
		},
		{
			name: "should get highest att for multiple with diff histories",
			attestationsInDB: []*slashertypes.IndexedAttestationWrapper{
				createAttestationWrapper(version.Phase0, 0, 3, []uint64{2}, []byte{1}),
				createAttestationWrapper(version.Phase0, 1, 4, []uint64{3}, []byte{2}),
				createAttestationWrapper(version.Phase0, 2, 3, []uint64{4}, []byte{3}),
				createAttestationWrapper(version.Phase0, 5, 6, []uint64{5}, []byte{4}),
			},
			indices: []primitives.ValidatorIndex{2, 3, 4, 5},
			expected: []*ethpb.HighestAttestation{
				{
					ValidatorIndex:     2,
					HighestSourceEpoch: 0,
					HighestTargetEpoch: 3,
				},
				{
					ValidatorIndex:     3,
					HighestSourceEpoch: 1,
					HighestTargetEpoch: 4,
				},
				{
					ValidatorIndex:     4,
					HighestSourceEpoch: 2,
					HighestTargetEpoch: 3,
				},
				{
					ValidatorIndex:     5,
					HighestSourceEpoch: 5,
					HighestTargetEpoch: 6,
				},
			},
		},
		{
			name: "should get correct highest att for multiple shared atts with diff histories",
			attestationsInDB: []*slashertypes.IndexedAttestationWrapper{
				createAttestationWrapper(version.Phase0, 1, 4, []uint64{2, 3}, []byte{1}),
				createAttestationWrapper(version.Phase0, 2, 5, []uint64{3, 5}, []byte{2}),
				createAttestationWrapper(version.Phase0, 4, 5, []uint64{1, 2}, []byte{3}),
				createAttestationWrapper(version.Phase0, 6, 7, []uint64{5}, []byte{4}),
			},
			indices: []primitives.ValidatorIndex{2, 3, 4, 5},
			expected: []*ethpb.HighestAttestation{
				{
					ValidatorIndex:     2,
					HighestSourceEpoch: 4,
					HighestTargetEpoch: 5,
				},
				{
					ValidatorIndex:     3,
					HighestSourceEpoch: 2,
					HighestTargetEpoch: 5,
				},
				{
					ValidatorIndex:     5,
					HighestSourceEpoch: 6,
					HighestTargetEpoch: 7,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beaconDB := setupDB(t)
			require.NoError(t, beaconDB.SaveAttestationRecordsForValidators(ctx, tt.attestationsInDB))

			highestAttestations, err := beaconDB.HighestAttestations(ctx, tt.indices)
			require.NoError(t, err)
			require.Equal(t, len(tt.expected), len(highestAttestations))
			for i, existing := range highestAttestations {
				require.DeepEqual(t, existing, tt.expected[i])
			}
		})
	}
}

func BenchmarkHighestAttestations(b *testing.B) {
	b.StopTimer()
	count := 10000
	valsPerAtt := 100
	indicesPerAtt := make([][]uint64, count)
	for i := 0; i < count; i++ {
		indicesForAtt := make([]uint64, valsPerAtt)
		for r := i * count; r < valsPerAtt*(i+1); r++ {
			indicesForAtt[i] = uint64(r)
		}
		indicesPerAtt[i] = indicesForAtt
	}
	atts := make([]*slashertypes.IndexedAttestationWrapper, count)
	for i := 0; i < count; i++ {
		atts[i] = createAttestationWrapper(version.Phase0, primitives.Epoch(i), primitives.Epoch(i+2), indicesPerAtt[i], []byte{})
	}

	ctx := b.Context()
	beaconDB := setupDB(b)
	require.NoError(b, beaconDB.SaveAttestationRecordsForValidators(ctx, atts))

	allIndices := make([]primitives.ValidatorIndex, 0, valsPerAtt*count)
	for i := 0; i < count; i++ {
		indicesForAtt := make([]primitives.ValidatorIndex, valsPerAtt)
		for r := 0; r < valsPerAtt; r++ {
			indicesForAtt[r] = primitives.ValidatorIndex(atts[i].IndexedAttestation.GetAttestingIndices()[r])
		}
		allIndices = append(allIndices, indicesForAtt...)
	}
	b.ReportAllocs()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := beaconDB.HighestAttestations(ctx, allIndices)
		require.NoError(b, err)
	}
}

func BenchmarkStore_CheckDoubleBlockProposals(b *testing.B) {
	b.StopTimer()
	count := 10000
	valsPerAtt := 100
	indicesPerAtt := make([][]uint64, count)
	for i := 0; i < count; i++ {
		indicesForAtt := make([]uint64, valsPerAtt)
		for r := i * count; r < valsPerAtt*(i+1); r++ {
			indicesForAtt[i] = uint64(r)
		}
		indicesPerAtt[i] = indicesForAtt
	}
	atts := make([]*slashertypes.IndexedAttestationWrapper, count)
	for i := 0; i < count; i++ {
		atts[i] = createAttestationWrapper(version.Phase0, primitives.Epoch(i), primitives.Epoch(i+2), indicesPerAtt[i], []byte{})
	}

	ctx := b.Context()
	beaconDB := setupDB(b)
	require.NoError(b, beaconDB.SaveAttestationRecordsForValidators(ctx, atts))

	// shuffle attestations
	rand.Shuffle(count, func(i, j int) { atts[i], atts[j] = atts[j], atts[i] })

	b.ReportAllocs()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := beaconDB.CheckAttesterDoubleVotes(ctx, atts)
		require.NoError(b, err)
	}
}

func createProposalWrapper(t *testing.T, slot primitives.Slot, proposerIndex primitives.ValidatorIndex, signingRoot []byte) *slashertypes.SignedBlockHeaderWrapper {
	header := &ethpb.BeaconBlockHeader{
		Slot:          slot,
		ProposerIndex: proposerIndex,
		ParentRoot:    params.BeaconConfig().ZeroHash[:],
		StateRoot:     bytesutil.PadTo(signingRoot, 32),
		BodyRoot:      params.BeaconConfig().ZeroHash[:],
	}
	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	return &slashertypes.SignedBlockHeaderWrapper{
		SignedBeaconBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header:    header,
			Signature: params.BeaconConfig().EmptySignature[:],
		},
		HeaderRoot: headerRoot,
	}
}

func createAttestationWrapper(ver int, source, target primitives.Epoch, indices []uint64, dataRootBytes []byte) *slashertypes.IndexedAttestationWrapper {
	dataRoot := bytesutil.ToBytes32(dataRootBytes)
	if dataRootBytes == nil {
		dataRoot = params.BeaconConfig().ZeroHash
	}

	data := &ethpb.AttestationData{
		BeaconBlockRoot: params.BeaconConfig().ZeroHash[:],
		Source: &ethpb.Checkpoint{
			Epoch: source,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: target,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
	}

	if ver >= version.Electra {
		return &slashertypes.IndexedAttestationWrapper{
			IndexedAttestation: &ethpb.IndexedAttestationElectra{
				AttestingIndices: indices,
				Data:             data,
				Signature:        params.BeaconConfig().EmptySignature[:],
			},
			DataRoot: dataRoot,
		}
	}
	return &slashertypes.IndexedAttestationWrapper{
		IndexedAttestation: &ethpb.IndexedAttestation{
			AttestingIndices: indices,
			Data:             data,
			Signature:        params.BeaconConfig().EmptySignature[:],
		},
		DataRoot: dataRoot,
	}
}

func Test_encodeValidatorIndex(t *testing.T) {
	tests := []struct {
		name  string
		index primitives.ValidatorIndex
	}{
		{
			name:  "0",
			index: primitives.ValidatorIndex(0),
		},
		{
			name:  "genesis_validator_count",
			index: primitives.ValidatorIndex(params.BeaconConfig().MinGenesisActiveValidatorCount),
		},
		{
			name:  "max_possible_value",
			index: primitives.ValidatorIndex(params.BeaconConfig().ValidatorRegistryLimit - 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeValidatorIndex(tt.index)
			encodedIndex := append(got[:5], 0, 0, 0)
			decoded := binary.LittleEndian.Uint64(encodedIndex)
			require.DeepEqual(t, tt.index, primitives.ValidatorIndex(decoded))
		})
	}
}
