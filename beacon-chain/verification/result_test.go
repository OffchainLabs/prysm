package verification

import (
	"math"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestResultList(t *testing.T) {
	const (
		a Requirement = iota
		b
		c
		d
		e
		f
		g
		h
	)
	// leave out h to test excluding non-existent item
	all := []Requirement{a, b, c, d, e, f, g}
	alsoAll := requirementList(all).excluding()
	require.DeepEqual(t, all, alsoAll)
	missingFirst := requirementList(all).excluding(a)
	require.Equal(t, len(all)-1, len(missingFirst))
	require.DeepEqual(t, all[1:], missingFirst)
	missingLast := requirementList(all).excluding(g)
	require.Equal(t, len(all)-1, len(missingLast))
	require.DeepEqual(t, all[0:len(all)-1], missingLast)
	missingEnds := requirementList(missingLast).excluding(a)
	require.Equal(t, len(missingLast)-1, len(missingEnds))
	require.DeepEqual(t, all[1:len(all)-1], missingEnds)
	excludeNonexist := requirementList(missingEnds).excluding(h)
	require.Equal(t, len(missingEnds), len(excludeNonexist))
	require.DeepEqual(t, missingEnds, excludeNonexist)
}

func TestExportedBlobSanityCheck(t *testing.T) {
	// make sure all requirement lists contain the bare minimum checks
	sanity := []Requirement{RequireValidProposerSignature, RequireSidecarKzgProofVerified, RequireBlobIndexInBounds, RequireSidecarInclusionProven}
	reqs := [][]Requirement{GossipBlobSidecarRequirements, SpectestBlobSidecarRequirements, InitsyncBlobSidecarRequirements, BackfillBlobSidecarRequirements, PendingQueueBlobSidecarRequirements}
	for i := range reqs {
		r := reqs[i]
		reqMap := make(map[Requirement]struct{})
		for ii := range r {
			reqMap[r[ii]] = struct{}{}
		}
		for ii := range sanity {
			_, ok := reqMap[sanity[ii]]
			require.Equal(t, true, ok)
		}
	}
	require.DeepEqual(t, allBlobSidecarRequirements, GossipBlobSidecarRequirements)
}

func TestAllBlobRequirementsHaveStrings(t *testing.T) {
	var derp Requirement = math.MaxInt
	require.Equal(t, unknownRequirementName, derp.String())
	for i := range allBlobSidecarRequirements {
		require.NotEqual(t, unknownRequirementName, allBlobSidecarRequirements[i].String())
	}
}
