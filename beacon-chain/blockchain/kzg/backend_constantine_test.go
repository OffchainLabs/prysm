package kzg

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/crypto/random"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func embeddedSetup(t *testing.T) *TrustedSetup {
	t.Helper()

	setup := &TrustedSetup{}
	require.NoError(t, json.Unmarshal(embeddedTrustedSetup, setup))

	return setup
}

// newConstantineBackend returns a Constantine backend with the trusted setup
// loaded, independently of whichever backend is currently active.
func newConstantineBackend(t *testing.T) *constantineBackend {
	t.Helper()

	b := &constantineBackend{}
	require.NoError(t, b.LoadTrustedSetup(embeddedSetup(t)))
	t.Cleanup(b.ctx.Close)

	return b
}

func randomBlob(t *testing.T, seed int64) Blob {
	t.Helper()

	return Blob(random.GetRandBlob(seed))
}

// TestConstantineMatchesCKZG is the important test for the Constantine backend:
// the two libraries must agree bit for bit on every operation, since they are
// interchangeable implementations of the same specification.
func TestConstantineMatchesCKZG(t *testing.T) {
	require.NoError(t, Start())

	ckzg := ckzgBackend{}
	ctt := newConstantineBackend(t)

	const blobCount = 3
	for seed := int64(0); seed < blobCount; seed++ {
		blob := randomBlob(t, seed)

		t.Run("ComputeCells", func(t *testing.T) {
			want, err := ckzg.ComputeCells(&blob)
			require.NoError(t, err)
			got, err := ctt.ComputeCells(&blob)
			require.NoError(t, err)
			requireCellsEqual(t, want, got)
		})

		t.Run("ComputeCellsAndKZGProofs", func(t *testing.T) {
			wantCells, wantProofs, err := ckzg.ComputeCellsAndKZGProofs(&blob)
			require.NoError(t, err)
			gotCells, gotProofs, err := ctt.ComputeCellsAndKZGProofs(&blob)
			require.NoError(t, err)
			requireCellsEqual(t, wantCells, gotCells)
			requireProofsEqual(t, wantProofs, gotProofs)
		})
	}

	// Recovery needs at least half of the cells; drop every other one.
	blob := randomBlob(t, 42)
	allCells, allProofs, err := ckzg.ComputeCellsAndKZGProofs(&blob)
	require.NoError(t, err)

	var (
		partialIndices []uint64
		partialCells   []Cell
	)
	for i := 0; i < len(allCells); i += 2 {
		partialIndices = append(partialIndices, uint64(i))
		partialCells = append(partialCells, allCells[i])
	}

	t.Run("RecoverCells", func(t *testing.T) {
		want, err := ckzg.RecoverCells(partialIndices, partialCells)
		require.NoError(t, err)
		got, err := ctt.RecoverCells(partialIndices, partialCells)
		require.NoError(t, err)
		requireCellsEqual(t, want, got)
		// Recovery has to reproduce the cells it was not given.
		requireCellsEqual(t, allCells, got)
	})

	t.Run("RecoverCellsAndKZGProofs", func(t *testing.T) {
		wantCells, wantProofs, err := ckzg.RecoverCellsAndKZGProofs(partialIndices, partialCells)
		require.NoError(t, err)
		gotCells, gotProofs, err := ctt.RecoverCellsAndKZGProofs(partialIndices, partialCells)
		require.NoError(t, err)
		requireCellsEqual(t, wantCells, gotCells)
		requireProofsEqual(t, wantProofs, gotProofs)
		requireProofsEqual(t, allProofs, gotProofs)
	})

	t.Run("VerifyCellKZGProofBatch", func(t *testing.T) {
		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		commitments := make([]Bytes48, len(allCells))
		indices := make([]uint64, len(allCells))
		proofs := make([]Bytes48, len(allCells))
		for i := range allCells {
			commitments[i] = Bytes48(commitment)
			indices[i] = uint64(i)
			proofs[i] = Bytes48(allProofs[i])
		}

		valid, err := ctt.VerifyCellKZGProofBatch(commitments, indices, allCells, proofs)
		require.NoError(t, err)
		require.Equal(t, true, valid)

		// Two valid proofs in the wrong order stay well formed, so this has to be
		// reported as a failed verification rather than as an error.
		swapped := make([]Bytes48, len(proofs))
		copy(swapped, proofs)
		swapped[0], swapped[1] = swapped[1], swapped[0]

		valid, err = ctt.VerifyCellKZGProofBatch(commitments, indices, allCells, swapped)
		require.NoError(t, err)
		require.Equal(t, false, valid)

		// A proof that is not a valid curve point encoding is a different matter,
		// and is reported as an error.
		corrupted := make([]Bytes48, len(proofs))
		copy(corrupted, proofs)
		corrupted[0][0] ^= 0xff

		_, err = ctt.VerifyCellKZGProofBatch(commitments, indices, allCells, corrupted)
		require.ErrorContains(t, "EccInvalidEncoding", err)
	})

	t.Run("VerifyBlobKZGProofBatch", func(t *testing.T) {
		blobs := make([]Blob, blobCount)
		commitments := make([]Bytes48, blobCount)
		proofs := make([]Bytes48, blobCount)

		for i := range blobs {
			blobs[i] = randomBlob(t, int64(100+i))

			commitment, err := BlobToKZGCommitment(&blobs[i])
			require.NoError(t, err)
			commitments[i] = Bytes48(commitment)

			proof, err := ComputeBlobKZGProof(&blobs[i], commitment)
			require.NoError(t, err)
			proofs[i] = Bytes48(proof)
		}

		valid, err := ctt.VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.NoError(t, err)
		require.Equal(t, true, valid)

		// As above: well formed but mismatched proofs fail verification.
		swapped := make([]Bytes48, len(proofs))
		copy(swapped, proofs)
		swapped[0], swapped[1] = swapped[1], swapped[0]

		valid, err = ctt.VerifyBlobKZGProofBatch(blobs, commitments, swapped)
		require.NoError(t, err)
		require.Equal(t, false, valid)
	})
}

func requireCellsEqual(t *testing.T, want, got []Cell) {
	t.Helper()

	require.Equal(t, len(want), len(got))
	for i := range want {
		require.DeepEqual(t, want[i][:], got[i][:])
	}
}

func requireProofsEqual(t *testing.T, want, got []Proof) {
	t.Helper()

	require.Equal(t, len(want), len(got))
	for i := range want {
		require.DeepEqual(t, want[i][:], got[i][:])
	}
}

// TestWriteTrustedSetupFile checks the file Constantine is handed against the
// c-kzg text format it parses: a count of G1 and of G2 points, then the G1
// Lagrange, G2 monomial and G1 monomial points as bare hex, one per line.
func TestWriteTrustedSetupFile(t *testing.T) {
	setup := embeddedSetup(t)

	path, err := writeTrustedSetupFile(setup)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Remove(path))
	}()

	file, err := os.Open(path) // #nosec G304 -- path from writeTrustedSetupFile
	require.NoError(t, err)
	defer func() {
		require.NoError(t, file.Close())
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024), 1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())

	const (
		g1Count = 4096
		g2Count = 65
	)
	require.Equal(t, 2+g1Count+g2Count+g1Count, len(lines))
	require.Equal(t, "4096", lines[0])
	require.Equal(t, "65", lines[1])

	// Sections in order, hex without the 0x prefix, decodable to 48 or 96 bytes.
	requireSection := func(offset int, points []string, bytesPerPoint int) {
		for i, point := range points {
			line := lines[offset+i]
			require.Equal(t, false, strings.HasPrefix(line, "0x"))
			require.Equal(t, strings.TrimPrefix(point, "0x"), line)

			raw, err := hex.DecodeString(line)
			require.NoError(t, err)
			require.Equal(t, bytesPerPoint, len(raw))
		}
	}

	g1Lagrange := make([]string, g1Count)
	g1Monomial := make([]string, g1Count)
	for i := range setup.G1Lagrange {
		g1Lagrange[i] = string(setup.G1Lagrange[i])
		g1Monomial[i] = string(setup.G1Monomial[i])
	}
	g2Monomial := make([]string, g2Count)
	for i := range setup.G2Monomial {
		g2Monomial[i] = string(setup.G2Monomial[i])
	}

	requireSection(2, g1Lagrange, 48)
	requireSection(2+g1Count, g2Monomial, 96)
	requireSection(2+g1Count+g2Count, g1Monomial, 48)
}

// TestSelectBackend covers the wiring of the feature flag, since it is the only
// thing that decides which library the node uses.
func TestSelectBackend(t *testing.T) {
	t.Cleanup(func() { activeBackend = ckzgBackend{} })

	reset := features.InitWithReset(&features.Flags{ConstantineKZG: false})
	selectBackend()
	_, isCKZG := activeBackend.(ckzgBackend)
	require.Equal(t, true, isCKZG)
	reset()

	reset = features.InitWithReset(&features.Flags{ConstantineKZG: true})
	selectBackend()
	_, isConstantine := activeBackend.(*constantineBackend)
	require.Equal(t, true, isConstantine)
	reset()
}
