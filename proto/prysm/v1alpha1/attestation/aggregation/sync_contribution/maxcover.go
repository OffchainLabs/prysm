package sync_contribution

import (
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	v2 "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation/aggregation"
)

// maxCoverSyncContributionAggregation implements max cover aggregation strategy for sync contributions.
// This implementation provides better performance than naive aggregation by using a greedy approach
// to maximize the coverage of aggregation bits.
func maxCoverSyncContributionAggregation(contributions []*v2.SyncCommitteeContribution) ([]*v2.SyncCommitteeContribution, error) {
	if len(contributions) <= 1 {
		return contributions, nil
	}

	// Create candidates for max cover problem
	candidates := make([]*aggregation.MaxCoverCandidate, len(contributions))
	for i, c := range contributions {
		candidates[i] = aggregation.NewMaxCoverCandidate(i, c.AggregationBits)
	}

	// Create and solve max cover problem
	problem := &aggregation.MaxCoverProblem{
		Candidates: candidates,
	}

	// We want to find the minimum number of contributions that cover all bits
	// So we start with k = 1 and increase it until we get full coverage or reach len(contributions)
	var solution *aggregation.Aggregation
	var err error
	for k := 1; k <= len(contributions); k++ {
		solution, err = problem.Cover(k, false) // false = do not allow overlaps
		if err != nil {
			return nil, err
		}

		// Check if we have full coverage
		if solution.Coverage.Count() == solution.Coverage.Len() {
			break
		}
	}

	// Aggregate the selected contributions
	result := make([]*v2.SyncCommitteeContribution, 0, len(solution.Keys))
	for _, idx := range solution.Keys {
		if len(result) == 0 {
			result = append(result, v2.CopySyncCommitteeContribution(contributions[idx]))
			continue
		}

		// Aggregate with the first contribution in result
		aggregated, err := aggregate(result[0], contributions[idx])
		if err != nil {
			return nil, err
		}
		result[0] = aggregated
	}

	return result, nil
} 
