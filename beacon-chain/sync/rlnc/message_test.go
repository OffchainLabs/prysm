package rlnc

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
	ristretto "github.com/gtank/ristretto255"
)

func TestVerify(t *testing.T) {
	// chunks have length 3 and there are two chunks
	s11 := randomScalar()
	s12 := randomScalar()
	s13 := randomScalar()
	scalars1 := []*ristretto.Scalar{s11, s12, s13}

	s21 := randomScalar()
	s22 := randomScalar()
	s23 := randomScalar()
	scalars2 := []*ristretto.Scalar{s21, s22, s23}

	data := [][]*ristretto.Scalar{scalars1, scalars2}

	coeff1 := randomScalar()
	coeff2 := randomScalar()
	coeff3 := randomScalar()
	goodCoefficients := []*ristretto.Scalar{coeff1, coeff2}
	badCoefficients := []*ristretto.Scalar{coeff1, coeff3}

	committer := newCommitter(3)
	c1, err := committer.commit(scalars1)
	require.NoError(t, err)
	c2, err := committer.commit(scalars2)
	require.NoError(t, err)
	goodCommitments := []*ristretto.Element{c1, c2}
	badCommitments := []*ristretto.Element{c1, c1}

	goodData, err := scalarLC(goodCoefficients, data)
	require.NoError(t, err)
	badData, err := scalarLC(badCoefficients, data)
	require.NoError(t, err)

	tests := []struct {
		name     string
		message  *message
		expected bool
	}{
		{
			name: "valid message",
			message: &message{
				chunk: chunk{
					data:         goodData,
					coefficients: goodCoefficients,
				},
				commitments: goodCommitments,
			},
			expected: true,
		},
		{
			name: "invalid coefficients",
			message: &message{
				chunk: chunk{
					data:         goodData,
					coefficients: badCoefficients,
				},
				commitments: goodCommitments,
			},
			expected: false,
		},
		{
			name: "invalid data",
			message: &message{
				chunk: chunk{
					data:         badData,
					coefficients: goodCoefficients,
				},
				commitments: goodCommitments,
			},
			expected: false,
		},
		{
			name: "invalid commitments",
			message: &message{
				chunk: chunk{
					data:         goodData,
					coefficients: goodCoefficients,
				},
				commitments: badCommitments,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.message.Verify(committer)
			require.Equal(t, tt.expected, result)
		})
	}
}
