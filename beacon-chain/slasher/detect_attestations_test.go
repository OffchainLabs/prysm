package slasher

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	dbtest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	slashingsmock "github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/slashings/mock"
	slashertypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls/common"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func Test_processAttestations(t *testing.T) {
	type (
		attestationInfo struct {
			ver             int
			source          primitives.Epoch
			target          primitives.Epoch
			indices         []uint64
			beaconBlockRoot []byte
		}

		slashingInfo struct {
			ver               int
			attestationInfo_1 *attestationInfo
			attestationInfo_2 *attestationInfo
		}

		step struct {
			currentEpoch          primitives.Epoch
			attestationsInfo      []*attestationInfo
			expectedSlashingsInfo []*slashingInfo
		}

		test struct {
			ver   int
			name  string
			steps []*step
		}
	)

	var tests []test

	for _, v := range []int{version.Phase0, version.Electra} {
		tests = append(tests, []test{
			{
				ver:  v,
				name: "Same target with different signing roots - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
								attestationInfo_2: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Same target with different signing roots - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
								attestationInfo_2: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Same target with same signing roots - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Same target with same signing roots - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounding vote (source 1, target 2), (source 0, target 3) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounding vote (source 1, target 2), (source 0, target 3) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounding vote (source 50, target 51), (source 0, target 1000) - single step",
				steps: []*step{
					{
						currentEpoch: 1000,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 50, target: 51, indices: []uint64{0}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 1000, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 0, target: 1000, indices: []uint64{0}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 50, target: 51, indices: []uint64{0}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounding vote (source 50, target 51), (source 0, target 1000) - two steps",
				steps: []*step{
					{
						currentEpoch: 1000,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 50, target: 51, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 1000,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 1000, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 0, target: 1000, indices: []uint64{0}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 50, target: 51, indices: []uint64{0}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounded vote (source 0, target 3), (source 1, target 2) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects surrounded vote (source 0, target 3), (source 1, target 2) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects double vote, (source 1, target 2), (source 0, target 2) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Detects double vote, (source 1, target 2), (source 0, target 2) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: []*slashingInfo{
							{
								ver:               v,
								attestationInfo_1: &attestationInfo{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
								attestationInfo_2: &attestationInfo{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							},
						},
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounding but non-overlapping attesting indices within same validator chunk index - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 3, indices: []uint64{1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounding but non-overlapping attesting indices within same validator chunk index - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounded but non-overlapping attesting indices within same validator chunk index - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 1, target: 2, indices: []uint64{2, 3}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounded but non-overlapping attesting indices within same validator chunk index - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{2, 3}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounding but non-overlapping attesting indices in different validator chunk index - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0}, beaconBlockRoot: nil},
							{ver: v, source: 1, target: 2, indices: []uint64{params.BeaconConfig().MinGenesisActiveValidatorCount - 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounding but non-overlapping attesting indices in different validator chunk index - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{params.BeaconConfig().MinGenesisActiveValidatorCount - 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounded but non-overlapping attesting indices in different validator chunk index - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0}, beaconBlockRoot: nil},
							{ver: v, source: 1, target: 2, indices: []uint64{params.BeaconConfig().MinGenesisActiveValidatorCount - 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, surrounded but non-overlapping attesting indices in different validator chunk index - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{params.BeaconConfig().MinGenesisActiveValidatorCount - 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 1, target 2), (source 2, target 3) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 2, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 1, target 2), (source 2, target 3) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 2, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 3), (source 2, target 4) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 2, target: 4, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 3), (source 2, target 4) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 2, target: 4, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 2), (source 0, target 3) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 2), (source 0, target 3) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 3), (source 0, target 2) - single step",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
			{
				ver:  v,
				name: "Not slashable, (source 0, target 3), (source 0, target 2) - two steps",
				steps: []*step{
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 3, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
					{
						currentEpoch: 4,
						attestationsInfo: []*attestationInfo{
							{ver: v, source: 0, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: nil},
						},
						expectedSlashingsInfo: nil,
					},
				},
			},
		}...)

		tests = append(tests, test{
			name: "Different versions, same target with different signing roots - single step",
			steps: []*step{
				{
					currentEpoch: 4,
					attestationsInfo: []*attestationInfo{
						{ver: version.Phase0, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
						{ver: version.Electra, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
					},
					expectedSlashingsInfo: []*slashingInfo{
						{
							ver:               version.Electra,
							attestationInfo_1: &attestationInfo{ver: version.Electra, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{1}},
							attestationInfo_2: &attestationInfo{ver: version.Electra, source: 1, target: 2, indices: []uint64{0, 1}, beaconBlockRoot: []byte{2}},
						},
					},
				},
			},
		})

		for _, tt := range tests {
			name := version.String(tt.ver) + ": " + tt.name
			t.Run(name, func(t *testing.T) {
				// Create context.
				ctx := context.Background()

				// Configure logging.
				hook := logTest.NewGlobal()
				defer hook.Reset()

				// Configure the slasher database.
				slasherDB := dbtest.SetupSlasherDB(t)

				// Configure the beacon state.
				beaconState, err := util.NewBeaconState()
				require.NoError(t, err)

				// Create the mock chain service.
				mockChain := &mock.ChainService{State: beaconState}

				// Create the mock slashing pool inserter.
				mockSlashingPoolInserter := &slashingsmock.PoolMock{}

				// Create the service configuration.
				serviceConfig := &ServiceConfig{
					Database:                slasherDB,
					HeadStateFetcher:        mockChain,
					AttestationStateFetcher: mockChain,
					SlashingPoolInserter:    mockSlashingPoolInserter,
				}

				// Create the slasher service.
				slasherService, err := New(context.Background(), serviceConfig)
				require.NoError(t, err)

				// Initialize validators in the state.
				numVals := params.BeaconConfig().MinGenesisActiveValidatorCount
				validators := make([]*ethpb.Validator, numVals)
				privateKeys := make([]bls.SecretKey, numVals)

				for i := uint64(0); i < numVals; i++ {
					// Create a random private key.
					privateKey, err := bls.RandKey()
					require.NoError(t, err)

					// Add the private key to the list.
					privateKeys[i] = privateKey

					// Derive the public key from the private key.
					publicKey := privateKey.PublicKey().Marshal()

					// Initialize the validator.
					validator := &ethpb.Validator{PublicKey: publicKey}

					// Add the validator to the list.
					validators[i] = validator
				}

				// Set the validators into the state.
				err = beaconState.SetValidators(validators)
				require.NoError(t, err)

				// Compute the signing domain.
				domain, err := signing.Domain(
					beaconState.Fork(),
					0,
					params.BeaconConfig().DomainBeaconAttester,
					beaconState.GenesisValidatorsRoot(),
				)
				require.NoError(t, err)

				for _, step := range tt.steps {
					// Build attestation wrappers.
					attestationsCount := len(step.attestationsInfo)
					attestationWrappers := make([]*slashertypes.IndexedAttestationWrapper, 0, attestationsCount)
					for _, attestationInfo := range step.attestationsInfo {
						// Create a wrapped attestation.
						attestationWrapper := createAttestationWrapper(
							t,
							attestationInfo.ver,
							domain,
							privateKeys,
							attestationInfo.source,
							attestationInfo.target,
							attestationInfo.indices,
							attestationInfo.beaconBlockRoot,
						)

						// Add the wrapped attestation to the list.
						attestationWrappers = append(attestationWrappers, attestationWrapper)
					}

					// Build expected attester slashings.
					expectedSlashings := make(map[[fieldparams.RootLength]byte]ethpb.AttSlashing, len(step.expectedSlashingsInfo))

					for _, slashingInfo := range step.expectedSlashingsInfo {
						// Create attestations.
						wrapper_1 := createAttestationWrapper(
							t,
							slashingInfo.attestationInfo_1.ver,
							domain,
							privateKeys,
							slashingInfo.attestationInfo_1.source,
							slashingInfo.attestationInfo_1.target,
							slashingInfo.attestationInfo_1.indices,
							slashingInfo.attestationInfo_1.beaconBlockRoot,
						)

						wrapper_2 := createAttestationWrapper(
							t,
							slashingInfo.attestationInfo_2.ver,
							domain,
							privateKeys,
							slashingInfo.attestationInfo_2.source,
							slashingInfo.attestationInfo_2.target,
							slashingInfo.attestationInfo_2.indices,
							slashingInfo.attestationInfo_2.beaconBlockRoot,
						)

						// Create the attester slashing.
						var expectedSlashing ethpb.AttSlashing

						if slashingInfo.ver >= version.Electra {
							expectedSlashing = &ethpb.AttesterSlashingElectra{
								Attestation_1: wrapper_1.IndexedAttestation.(*ethpb.IndexedAttestationElectra),
								Attestation_2: wrapper_2.IndexedAttestation.(*ethpb.IndexedAttestationElectra),
							}
						} else {
							expectedSlashing = &ethpb.AttesterSlashing{
								Attestation_1: wrapper_1.IndexedAttestation.(*ethpb.IndexedAttestation),
								Attestation_2: wrapper_2.IndexedAttestation.(*ethpb.IndexedAttestation),
							}
						}

						root, err := expectedSlashing.HashTreeRoot()
						require.NoError(t, err, "failed to hash tree root")

						// Add the attester slashing to the map.
						expectedSlashings[root] = expectedSlashing
					}

					// Get the currentSlot for the current epoch.
					currentSlot, err := slots.EpochStart(step.currentEpoch)
					require.NoError(t, err)

					// Process the attestations.
					processedSlashings := slasherService.processAttestations(ctx, attestationWrappers, currentSlot)

					// Check the processed slashings correspond to the expected slashings.
					require.Equal(t, len(expectedSlashings), len(processedSlashings), "processed slashings count not equal to expected")

					for root := range expectedSlashings {
						// Check the expected slashing is in the processed slashings.
						processedSlashing, ok := processedSlashings[root]
						require.Equal(t, true, ok, "processed slashing not found")

						// Check the root matches
						controlRoot, err := processedSlashing.HashTreeRoot()
						require.NoError(t, err, "failed to hash tree root")
						require.Equal(t, root, controlRoot, "root not equal")
					}
				}
			})
		}
	}
}

func Test_processQueuedAttestations_MultipleChunkIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	defer hook.Reset()

	slasherDB := dbtest.SetupSlasherDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	slasherParams := DefaultParams()

	// We process submit attestations from chunk index 0 to chunk index 1.
	// What we want to test here is if we can proceed
	// with processing queued attestations once the chunk index changes.
	// For example, epochs 0 - 15 are chunk 0, epochs 16 - 31 are chunk 1, etc.
	startEpoch := primitives.Epoch(slasherParams.chunkSize)
	endEpoch := primitives.Epoch(slasherParams.chunkSize + 1)

	currentTime := time.Now()
	totalSlots := uint64(startEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)
	secondsSinceGenesis := time.Duration(totalSlots * params.BeaconConfig().SecondsPerSlot)
	genesisTime := currentTime.Add(-secondsSinceGenesis * time.Second)

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	mockChain := &mock.ChainService{
		State: beaconState,
	}

	s, err := New(context.Background(),
		&ServiceConfig{
			Database:                slasherDB,
			StateNotifier:           &mock.MockStateNotifier{},
			HeadStateFetcher:        mockChain,
			AttestationStateFetcher: mockChain,
			SlashingPoolInserter:    &slashingsmock.PoolMock{},
			ClockWaiter:             startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)
	s.genesisTime = genesisTime

	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedAttestations(ctx, currentSlotChan)
	}()

	for i := startEpoch; i <= endEpoch; i++ {
		source := primitives.Epoch(0)
		target := primitives.Epoch(0)
		if i != 0 {
			source = i - 1
			target = i
		}
		var sr [32]byte
		copy(sr[:], fmt.Sprintf("%d", i))
		att := createAttestationWrapperEmptySig(t, version.Phase0, source, target, []uint64{0}, sr[:])
		s.attsQueue = newAttestationsQueue()
		s.attsQueue.push(att)
		slot, err := slots.EpochStart(i)
		require.NoError(t, err)
		require.NoError(t, mockChain.State.SetSlot(slot))
		s.serviceCfg.HeadStateFetcher = mockChain
		currentSlotChan <- slot
	}

	time.Sleep(time.Millisecond * 200)
	cancel()
	s.wg.Wait()
	require.LogsDoNotContain(t, hook, "Slashable offenses found")
	require.LogsDoNotContain(t, hook, "Could not detect")
}

func Test_processQueuedAttestations_OverlappingChunkIndices(t *testing.T) {
	hook := logTest.NewGlobal()
	defer hook.Reset()

	slasherDB := dbtest.SetupSlasherDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	slasherParams := DefaultParams()

	startEpoch := primitives.Epoch(slasherParams.chunkSize)

	currentTime := time.Now()
	totalSlots := uint64(startEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)
	secondsSinceGenesis := time.Duration(totalSlots * params.BeaconConfig().SecondsPerSlot)
	genesisTime := currentTime.Add(-secondsSinceGenesis * time.Second)

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	mockChain := &mock.ChainService{
		State: beaconState,
	}

	s, err := New(context.Background(),
		&ServiceConfig{
			Database:                slasherDB,
			StateNotifier:           &mock.MockStateNotifier{},
			HeadStateFetcher:        mockChain,
			AttestationStateFetcher: mockChain,
			SlashingPoolInserter:    &slashingsmock.PoolMock{},
			ClockWaiter:             startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)
	s.genesisTime = genesisTime

	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedAttestations(ctx, currentSlotChan)
	}()

	// We create two attestations fully spanning chunk indices 0 and chunk 1
	att1 := createAttestationWrapperEmptySig(t, version.Phase0, primitives.Epoch(slasherParams.chunkSize-2), primitives.Epoch(slasherParams.chunkSize), []uint64{0, 1}, nil)
	att2 := createAttestationWrapperEmptySig(t, version.Phase0, primitives.Epoch(slasherParams.chunkSize-1), primitives.Epoch(slasherParams.chunkSize+1), []uint64{0, 1}, nil)

	// We attempt to process the batch.
	s.attsQueue = newAttestationsQueue()
	s.attsQueue.push(att1)
	s.attsQueue.push(att2)
	slot, err := slots.EpochStart(att2.IndexedAttestation.GetData().Target.Epoch)
	require.NoError(t, err)
	mockChain.Slot = &slot
	s.serviceCfg.HeadStateFetcher = mockChain
	currentSlotChan <- slot

	time.Sleep(time.Millisecond * 200)
	cancel()
	s.wg.Wait()
	require.LogsDoNotContain(t, hook, "Slashable offenses found")
	require.LogsDoNotContain(t, hook, "Could not detect")
}

func Test_updatedChunkByChunkIndex(t *testing.T) {
	neutralMin, neutralMax := uint16(65535), uint16(0)

	testCases := []struct {
		name                               string
		chunkSize                          uint64
		validatorChunkSize                 uint64
		historyLength                      primitives.Epoch
		currentEpoch                       primitives.Epoch
		validatorChunkIndex                uint64
		latestUpdatedEpochByValidatorIndex map[primitives.ValidatorIndex]primitives.Epoch
		initialMinChunkByChunkIndex        map[uint64][]uint16
		expectedMinChunkByChunkIndex       map[uint64][]uint16
		initialMaxChunkByChunkIndex        map[uint64][]uint16
		expectedMaxChunkByChunkIndex       map[uint64][]uint16
	}{
		{
			name:                               "start with no data - first chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      8,
			currentEpoch:                       2,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: nil,
			initialMinChunkByChunkIndex:        nil,
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                   validator 43                |
				0: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
			},
			initialMaxChunkByChunkIndex: nil,
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                   validator 43                |
				0: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
			},
		},
		{
			name:                               "start with no data - second chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      8,
			currentEpoch:                       5,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: nil,
			initialMinChunkByChunkIndex:        nil,
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                   validator 43                |
				0: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
				1: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
			},
			initialMaxChunkByChunkIndex: nil,
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                   validator 43                |
				0: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
				1: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
			},
		},
		{
			name:                               "start with some data - first chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      8,
			currentEpoch:                       2,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: map[primitives.ValidatorIndex]primitives.Epoch{42: 0, 43: 1},
			initialMinChunkByChunkIndex: map[uint64][]uint16{
				// |    validator 42    |   validator 43    |
				0: {14, 9999, 9999, 9999, 15, 16, 9999, 9999},
			},
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |           validator 42         |      validator 43       |
				0: {14, neutralMin, neutralMin, 9999, 15, 16, neutralMin, 9999},
			},
			initialMaxChunkByChunkIndex: map[uint64][]uint16{
				// |    validator 42    |  validator 43     |
				0: {70, 9999, 9999, 9999, 71, 72, 9999, 9999},
			},
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |          validator 42          |      validator 43        |
				0: {70, neutralMax, neutralMax, 9999, 71, 72, neutralMax, 9999},
			},
		},
		{
			name:                               "start with some data - second chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      8,
			currentEpoch:                       5,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: map[primitives.ValidatorIndex]primitives.Epoch{42: 1, 43: 2},
			initialMinChunkByChunkIndex: map[uint64][]uint16{
				// |   validator 42   |  validator 43   |
				0: {14, 13, 9999, 9999, 15, 16, 17, 9999},
			},
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |         validator 42         |     validator 43      |
				0: {14, 13, neutralMin, neutralMin, 15, 16, 17, neutralMin},

				// |                  validator 42                |                   validator 43                |
				1: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
			},
			initialMaxChunkByChunkIndex: map[uint64][]uint16{
				// |   validator 42   |   validator 43  |
				0: {70, 69, 9999, 9999, 71, 72, 73, 9999},
			},
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |         validator 42         |      validator 43     |
				0: {70, 69, neutralMax, neutralMax, 71, 72, 73, neutralMax},

				// |                  validator 42                |                   validator 43                |
				1: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
			},
		},
		{
			name:                               "start with some data - third chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      12,
			currentEpoch:                       9,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: map[primitives.ValidatorIndex]primitives.Epoch{42: 5, 43: 6},
			initialMinChunkByChunkIndex: map[uint64][]uint16{
				// |   validator 42   |  validator 43   |
				1: {14, 13, 9999, 9999, 15, 16, 17, 9999},
			},
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |         validator 42         |     validator 43      |
				1: {14, 13, neutralMin, neutralMin, 15, 16, 17, neutralMin},

				// |                  validator 42                |                   validator 43                |
				2: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
			},
			initialMaxChunkByChunkIndex: map[uint64][]uint16{
				// |   validator 42   |   validator 43  |
				1: {70, 69, 9999, 9999, 71, 72, 73, 9999},
			},
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |         validator 42         |      validator 43     |
				1: {70, 69, neutralMax, neutralMax, 71, 72, 73, neutralMax},

				// |                  validator 42                |                   validator 43                |
				2: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
			},
		},
		{
			name:                               "start with some data - third chunk - wrap to first chunk",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      12,
			currentEpoch:                       14,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: map[primitives.ValidatorIndex]primitives.Epoch{42: 9, 43: 10},
			initialMinChunkByChunkIndex: map[uint64][]uint16{
				// | validator 42 |  validator 43 |
				0: {55, 55, 55, 55, 55, 55, 55, 55},
				1: {66, 66, 66, 66, 66, 66, 66, 66},

				// |   validator 42   |   validator 43  |
				2: {77, 77, 9999, 9999, 77, 77, 77, 9999},
			},
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |        validator 42          |      validator 43     |
				2: {77, 77, neutralMin, neutralMin, 77, 77, 77, neutralMin},

				// |             validator 42             |             validator 43              |
				0: {neutralMin, neutralMin, neutralMin, 55, neutralMin, neutralMin, neutralMin, 55},
			},
			initialMaxChunkByChunkIndex: map[uint64][]uint16{
				// | validator 42 |  validator 43 |
				0: {55, 55, 55, 55, 55, 55, 55, 55},
				1: {66, 66, 66, 66, 66, 66, 66, 66},

				// |   validator 42   |   validator 43  |
				2: {77, 77, 9999, 9999, 77, 77, 77, 9999},
			},
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |        validator 42          |      validator 43     |
				2: {77, 77, neutralMax, neutralMax, 77, 77, 77, neutralMax},

				// |             validator 42             |             validator 43              |
				0: {neutralMax, neutralMax, neutralMax, 55, neutralMax, neutralMax, neutralMax, 55},
			},
		},
		{
			name:                               "start with some data - high latest updated epoch",
			chunkSize:                          4,
			validatorChunkSize:                 2,
			historyLength:                      12,
			currentEpoch:                       16,
			validatorChunkIndex:                21,
			latestUpdatedEpochByValidatorIndex: map[primitives.ValidatorIndex]primitives.Epoch{42: 2, 43: 3},
			initialMinChunkByChunkIndex: map[uint64][]uint16{
				// | validator 42 |  validator 43 |
				0: {55, 55, 55, 55, 55, 55, 55, 55},
				1: {66, 66, 66, 66, 66, 66, 66, 66},
				2: {77, 77, 77, 77, 77, 77, 77, 77},
			},
			expectedMinChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                  validator 43                 |
				0: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
				1: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
				2: {neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin, neutralMin},
			},
			initialMaxChunkByChunkIndex: map[uint64][]uint16{
				// | validator 42 |  validator 43 |
				0: {55, 55, 55, 55, 55, 55, 55, 55},
				1: {66, 66, 66, 66, 66, 66, 66, 66},
				2: {77, 77, 77, 77, 77, 77, 77, 77},
			},
			expectedMaxChunkByChunkIndex: map[uint64][]uint16{
				// |                  validator 42                |                  validator 43                 |
				0: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
				1: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
				2: {neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax, neutralMax},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Create context.
			ctx := context.Background()

			// Initialize the slasher database.
			slasherDB := dbtest.SetupSlasherDB(t)

			// Initialize the slasher service.
			service := &Service{
				params: &Parameters{
					chunkSize:          tt.chunkSize,
					validatorChunkSize: tt.validatorChunkSize,
					historyLength:      tt.historyLength,
				},
				serviceCfg:                     &ServiceConfig{Database: slasherDB},
				latestEpochUpdatedForValidator: tt.latestUpdatedEpochByValidatorIndex,
			}

			// Save min initial chunks if they exist.
			if tt.initialMinChunkByChunkIndex != nil {
				minChunkerByChunkerIndex := map[uint64]Chunker{}
				for chunkIndex, minChunk := range tt.initialMinChunkByChunkIndex {
					minChunkerByChunkerIndex[chunkIndex] = &MinSpanChunksSlice{data: minChunk}
				}

				minChunkerByChunkerIndexByValidatorChunkerIndex := map[uint64]map[uint64]Chunker{
					tt.validatorChunkIndex: minChunkerByChunkerIndex,
				}

				err := service.saveChunksToDisk(ctx, slashertypes.MinSpan, minChunkerByChunkerIndexByValidatorChunkerIndex)
				require.NoError(t, err)
			}

			// Save max initial chunks if they exist.
			if tt.initialMaxChunkByChunkIndex != nil {
				maxChunkerByChunkerIndex := map[uint64]Chunker{}
				for chunkIndex, maxChunk := range tt.initialMaxChunkByChunkIndex {
					maxChunkerByChunkerIndex[chunkIndex] = &MaxSpanChunksSlice{data: maxChunk}
				}

				maxChunkerByChunkerIndexByValidatorChunkerIndex := map[uint64]map[uint64]Chunker{
					tt.validatorChunkIndex: maxChunkerByChunkerIndex,
				}

				err := service.saveChunksToDisk(ctx, slashertypes.MaxSpan, maxChunkerByChunkerIndexByValidatorChunkerIndex)
				require.NoError(t, err)
			}

			// Get chunks.
			actualMinChunkByChunkIndex, err := service.updatedChunkByChunkIndex(
				ctx, slashertypes.MinSpan, tt.currentEpoch, tt.validatorChunkIndex,
			)

			// Compare the actual and expected chunks.
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedMinChunkByChunkIndex), len(actualMinChunkByChunkIndex))
			for chunkIndex, expectedMinChunk := range tt.expectedMinChunkByChunkIndex {
				actualMinChunk, ok := actualMinChunkByChunkIndex[chunkIndex]
				require.Equal(t, true, ok)
				require.Equal(t, len(expectedMinChunk), len(actualMinChunk.Chunk()))
				require.DeepSSZEqual(t, expectedMinChunk, actualMinChunk.Chunk())
			}

			actualMaxChunkByChunkIndex, err := service.updatedChunkByChunkIndex(
				ctx, slashertypes.MaxSpan, tt.currentEpoch, tt.validatorChunkIndex,
			)

			require.NoError(t, err)
			require.Equal(t, len(tt.expectedMaxChunkByChunkIndex), len(actualMaxChunkByChunkIndex))
			for chunkIndex, expectedMaxChunk := range tt.expectedMaxChunkByChunkIndex {
				actualMaxChunk, ok := actualMaxChunkByChunkIndex[chunkIndex]
				require.Equal(t, true, ok)
				require.Equal(t, len(expectedMaxChunk), len(actualMaxChunk.Chunk()))
				require.DeepSSZEqual(t, expectedMaxChunk, actualMaxChunk.Chunk())
			}

		})
	}
}

func Test_applyAttestationForValidator_MinSpanChunk(t *testing.T) {
	ctx := context.Background()
	slasherDB := dbtest.SetupSlasherDB(t)
	srv, err := New(context.Background(),
		&ServiceConfig{
			Database:      slasherDB,
			StateNotifier: &mock.MockStateNotifier{},
			ClockWaiter:   startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)

	// We initialize an empty chunks slice.
	currentEpoch := primitives.Epoch(3)
	validatorChunkIndex := uint64(0)
	validatorIdx := primitives.ValidatorIndex(0)
	chunksByChunkIdx := map[uint64]Chunker{}

	// We apply attestation with (source 1, target 2) for our validator.
	source := primitives.Epoch(1)
	target := primitives.Epoch(2)
	att := createAttestationWrapperEmptySig(t, version.Phase0, source, target, nil, nil)
	slashing, err := srv.applyAttestationForValidator(
		ctx,
		chunksByChunkIdx,
		att,
		slashertypes.MinSpan,
		validatorChunkIndex,
		validatorIdx,
		currentEpoch,
	)
	require.NoError(t, err)
	require.IsNil(t, slashing)
	att.IndexedAttestation.(*ethpb.IndexedAttestation).AttestingIndices = []uint64{uint64(validatorIdx)}
	err = slasherDB.SaveAttestationRecordsForValidators(
		ctx,
		[]*slashertypes.IndexedAttestationWrapper{att},
	)
	require.NoError(t, err)

	// Next, we apply an attestation with (source 0, target 3) and
	// expect a slashable offense to be returned.
	source = primitives.Epoch(0)
	target = primitives.Epoch(3)
	slashableAtt := createAttestationWrapperEmptySig(t, version.Phase0, source, target, nil, nil)
	slashing, err = srv.applyAttestationForValidator(
		ctx,
		chunksByChunkIdx,
		slashableAtt,
		slashertypes.MinSpan,
		validatorChunkIndex,
		validatorIdx,
		currentEpoch,
	)
	require.NoError(t, err)
	require.NotNil(t, slashing)
}

func Test_applyAttestationForValidator_MaxSpanChunk(t *testing.T) {
	ctx := context.Background()
	slasherDB := dbtest.SetupSlasherDB(t)
	srv, err := New(context.Background(),
		&ServiceConfig{
			Database:      slasherDB,
			StateNotifier: &mock.MockStateNotifier{},
			ClockWaiter:   startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)

	// We initialize an empty chunks slice.
	currentEpoch := primitives.Epoch(3)
	validatorChunkIndex := uint64(0)
	validatorIdx := primitives.ValidatorIndex(0)
	chunksByChunkIdx := map[uint64]Chunker{}

	// We apply attestation with (source 0, target 3) for our validator.
	source := primitives.Epoch(0)
	target := primitives.Epoch(3)
	att := createAttestationWrapperEmptySig(t, version.Phase0, source, target, nil, nil)
	slashing, err := srv.applyAttestationForValidator(
		ctx,
		chunksByChunkIdx,
		att,
		slashertypes.MaxSpan,
		validatorChunkIndex,
		validatorIdx,
		currentEpoch,
	)
	require.NoError(t, err)
	require.Equal(t, true, slashing == nil)
	att.IndexedAttestation.(*ethpb.IndexedAttestation).AttestingIndices = []uint64{uint64(validatorIdx)}
	err = slasherDB.SaveAttestationRecordsForValidators(
		ctx,
		[]*slashertypes.IndexedAttestationWrapper{att},
	)
	require.NoError(t, err)

	// Next, we apply an attestation with (source 1, target 2) and
	// expect a slashable offense to be returned.
	source = primitives.Epoch(1)
	target = primitives.Epoch(2)
	slashableAtt := createAttestationWrapperEmptySig(t, version.Phase0, source, target, nil, nil)
	slashing, err = srv.applyAttestationForValidator(
		ctx,
		chunksByChunkIdx,
		slashableAtt,
		slashertypes.MaxSpan,
		validatorChunkIndex,
		validatorIdx,
		currentEpoch,
	)
	require.NoError(t, err)
	require.NotNil(t, slashing)
}

func Test_loadChunks_MinSpans(t *testing.T) {
	testLoadChunks(t, slashertypes.MinSpan)
}

func Test_loadChunks_MaxSpans(t *testing.T) {
	testLoadChunks(t, slashertypes.MaxSpan)
}

func testLoadChunks(t *testing.T, kind slashertypes.ChunkKind) {
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx := context.Background()

	// Check if the chunk at chunk index already exists in-memory.
	s, err := New(context.Background(),
		&ServiceConfig{
			Database:      slasherDB,
			StateNotifier: &mock.MockStateNotifier{},
			ClockWaiter:   startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)

	defaultParams := s.params

	// If a chunk at a chunk index does not exist, ensure it
	// is initialized as an empty chunk.
	var emptyChunk Chunker
	if kind == slashertypes.MinSpan {
		emptyChunk = EmptyMinSpanChunksSlice(defaultParams)
	} else {
		emptyChunk = EmptyMaxSpanChunksSlice(defaultParams)
	}
	chunkIdx := uint64(2)
	received, err := s.loadChunksFromDisk(ctx, 0, kind, []uint64{chunkIdx})
	require.NoError(t, err)
	wanted := map[uint64]Chunker{
		chunkIdx: emptyChunk,
	}
	require.DeepEqual(t, wanted, received)

	// Save chunks to disk, then load them properly from disk.
	var existingChunk Chunker
	if kind == slashertypes.MinSpan {
		existingChunk = EmptyMinSpanChunksSlice(defaultParams)
	} else {
		existingChunk = EmptyMaxSpanChunksSlice(defaultParams)
	}
	validatorIdx := primitives.ValidatorIndex(0)
	epochInChunk := primitives.Epoch(0)
	targetEpoch := primitives.Epoch(2)
	err = setChunkDataAtEpoch(
		defaultParams,
		existingChunk.Chunk(),
		validatorIdx,
		epochInChunk,
		targetEpoch,
	)
	require.NoError(t, err)
	require.DeepNotEqual(t, existingChunk, emptyChunk)

	updatedChunks := map[uint64]Chunker{
		2: existingChunk,
		4: existingChunk,
		6: existingChunk,
	}

	chunkByChunkIndexByValidatorChunkIndex := map[uint64]map[uint64]Chunker{
		0: updatedChunks,
	}

	err = s.saveChunksToDisk(ctx, kind, chunkByChunkIndexByValidatorChunkIndex)
	require.NoError(t, err)
	// Check if the retrieved chunks match what we just saved to disk.
	received, err = s.loadChunksFromDisk(ctx, 0, kind, []uint64{2, 4, 6})
	require.NoError(t, err)
	require.DeepEqual(t, updatedChunks, received)
}

func TestService_processQueuedAttestations(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	slot, err := slots.EpochStart(1)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &slot,
	}

	s, err := New(context.Background(),
		&ServiceConfig{
			Database:         slasherDB,
			StateNotifier:    &mock.MockStateNotifier{},
			HeadStateFetcher: mockChain,
			ClockWaiter:      startup.NewClockSynchronizer(),
		})
	require.NoError(t, err)

	s.attsQueue.extend([]*slashertypes.IndexedAttestationWrapper{
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 1, []uint64{0, 1} /* indices */, nil /* signingRoot */),
	})
	ctx, cancel := context.WithCancel(context.Background())
	tickerChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedAttestations(ctx, tickerChan)
	}()

	// Send a value over the ticker.
	tickerChan <- 1
	cancel()
	s.wg.Wait()
	assert.LogsContain(t, hook, "Start processing queued attestations")
	assert.LogsContain(t, hook, "Done processing queued attestations")
}

func Benchmark_saveChunksToDisk(b *testing.B) {
	// Define the parameters.
	const (
		chunkKind                    = slashertypes.MinSpan
		validatorsChunksCount        = 6000 // Corresponds to 1_536_000 validators x 256 validators / chunk
		chunkIndex            uint64 = 13
		validatorChunkIndex   uint64 = 42
	)

	params := DefaultParams()

	// Get a context.
	ctx := context.Background()

	chunkByChunkIndexByValidatorChunkIndex := make(map[uint64]map[uint64]Chunker, validatorsChunksCount)

	// Populate the chunkers.
	for i := 0; i < validatorsChunksCount; i++ {
		data := make([]uint16, params.chunkSize)
		for j := 0; j < int(params.chunkSize); j++ {
			data[j] = uint16(rand.Intn(1 << 16))
		}

		chunker := map[uint64]Chunker{chunkIndex: &MinSpanChunksSlice{params: params, data: data}}
		chunkByChunkIndexByValidatorChunkIndex[uint64(i)] = chunker
	}

	// Initialize the slasher database.
	slasherDB := dbtest.SetupSlasherDB(b)

	// Initialize the slasher service.
	service, err := New(ctx, &ServiceConfig{Database: slasherDB})
	require.NoError(b, err)

	// Reset the benchmark timer.
	b.ResetTimer()

	// Run the benchmark.
	for i := 0; i < b.N; i++ {
		b.StartTimer()
		err = service.saveChunksToDisk(ctx, slashertypes.MinSpan, chunkByChunkIndexByValidatorChunkIndex)
		b.StopTimer()
		require.NoError(b, err)
	}
}

func BenchmarkCheckSlashableAttestations(b *testing.B) {
	slasherDB := dbtest.SetupSlasherDB(b)

	beaconState, err := util.NewBeaconState()
	require.NoError(b, err)
	slot := primitives.Slot(0)
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &slot,
	}

	s, err := New(context.Background(), &ServiceConfig{
		Database:         slasherDB,
		StateNotifier:    &mock.MockStateNotifier{},
		HeadStateFetcher: mockChain,
		ClockWaiter:      startup.NewClockSynchronizer(),
	})
	require.NoError(b, err)

	b.Run("1 attestation 1 validator", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1, 1 /* validator */)
	})
	b.Run("1 attestation 100 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1, 100 /* validator */)
	})
	b.Run("1 attestation 1000 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1, 1000 /* validator */)
	})

	b.Run("100 attestations 1 validator", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 100, 1 /* validator */)
	})
	b.Run("100 attestations 100 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 100, 100 /* validator */)
	})
	b.Run("100 attestations 1000 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 100, 1000 /* validator */)
	})

	b.Run("1000 attestations 1 validator", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1000, 1 /* validator */)
	})
	b.Run("1000 attestations 100 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1000, 100 /* validator */)
	})
	b.Run("1000 attestations 1000 validators", func(b *testing.B) {
		b.ResetTimer()
		runAttestationsBenchmark(b, s, 1000, 1000 /* validator */)
	})
}

func runAttestationsBenchmark(b *testing.B, s *Service, numAtts, numValidators uint64) {
	indices := make([]uint64, numValidators)
	for i := uint64(0); i < numValidators; i++ {
		indices[i] = i
	}
	atts := make([]*slashertypes.IndexedAttestationWrapper, numAtts)
	for i := uint64(0); i < numAtts; i++ {
		source := primitives.Epoch(i)
		target := primitives.Epoch(i + 1)
		var signingRoot [32]byte
		copy(signingRoot[:], fmt.Sprintf("%d", i))
		atts[i] = createAttestationWrapperEmptySig(
			b,
			version.Phase0,
			source,
			target,         /* target */
			indices,        /* indices */
			signingRoot[:], /* signingRoot */
		)
	}
	for i := 0; i < b.N; i++ {
		numEpochs := numAtts
		totalSeconds := numEpochs * uint64(params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().SecondsPerSlot
		genesisTime := time.Now().Add(-time.Second * time.Duration(totalSeconds))
		s.genesisTime = genesisTime

		epoch := slots.EpochsSinceGenesis(genesisTime)
		_, err := s.checkSlashableAttestations(context.Background(), epoch, atts)
		require.NoError(b, err)
	}
}

func Benchmark_checkSurroundVotes(b *testing.B) {
	const (
		// Approximately the number of Holesky active validators on 2024-02-16
		// This number is both a multiple of 32 (the number of slots per epoch) and 256 (the number of validators per chunk)
		validatorsCount = 1_638_400
		slotsPerEpoch   = 32

		targetEpoch  = 42
		sourceEpoch  = 43
		currentEpoch = 43
	)
	// Create a context.
	ctx := context.Background()

	// Initialize the slasher database.
	slasherDB := dbtest.SetupSlasherDB(b)

	// Initialize the slasher service.
	service, err := New(ctx, &ServiceConfig{Database: slasherDB})
	require.NoError(b, err)

	// Create the attesting validators indexes.
	// The best case scenario would be to have all validators attesting for a slot with contiguous indexes.
	// So for 1_638_400 validators with 32 slots per epoch, we would have 48_000 attestation wrappers per slot.
	// With 256 validators per chunk, we would have only 188 modified chunks.
	//
	// In this benchmark, we use the worst case scenario where attesting validators are evenly split across all validators chunks.
	// We also suppose that only one chunk per validator chunk index is modified.
	// For one given validator index, multiple chunk indexes could be modified.
	//
	// With 1_638_400 validators we have 6400 chunks. If exactly 8 validators per chunks attest, we have:
	// 6_400 chunks * 8 = 51_200 validators attesting per slot. And 51_200 validators * 32 slots = 1_638_400
	// attesting validators per epoch.
	// ==> Attesting validator indexes will be computed as follows:
	//         validator chunk index 0               validator chunk index 1                   validator chunk index 6_399
	// [0, 32, 64, 96, 128, 160, 192, 224 | 256, 288, 320, 352, 384, 416, 448, 480 | ... | ..., 1_638_606,  1_638_368, 1_638_400]
	//

	attestingValidatorsCount := validatorsCount / slotsPerEpoch
	validatorIndexes := make([]uint64, attestingValidatorsCount)
	for i := 0; i < attestingValidatorsCount; i++ {
		validatorIndexes[i] = 32 * uint64(i)
	}

	// Create the attestation wrapper.
	// This benchmark assume that all validators produced the exact same head, source and target votes.
	attWrapper := createAttestationWrapperEmptySig(b, version.Phase0, sourceEpoch, targetEpoch, validatorIndexes, nil)
	attWrappers := []*slashertypes.IndexedAttestationWrapper{attWrapper}

	// Run the benchmark.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StartTimer()
		_, err = service.checkSurroundVotes(ctx, attWrappers, currentEpoch)
		b.StopTimer()

		require.NoError(b, err)
	}
}

// createAttestationWrapperEmptySig creates an attestation wrapper with source and target,
// for validators with indices, and a beacon block root (corresponding to the head vote).
// For source and target epochs, the corresponding root is null.
// The signature of the returned wrapped attestation is empty.
func createAttestationWrapperEmptySig(
	t testing.TB,
	ver int,
	source, target primitives.Epoch,
	indices []uint64,
	beaconBlockRoot []byte,
) *slashertypes.IndexedAttestationWrapper {
	data := &ethpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo(beaconBlockRoot, 32),
		Source: &ethpb.Checkpoint{
			Epoch: source,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: target,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
	}

	dataRoot, err := data.HashTreeRoot()
	require.NoError(t, err)

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

// createAttestationWrapper creates an attestation wrapper with source and target,
// for validators with indices, and a beacon block root (corresponding to the head vote).
// For source and target epochs, the corresponding root is null.
// if validatorIndice = indices[i], then the corresponding private key is privateKeys[validatorIndice].
func createAttestationWrapper(
	t testing.TB,
	ver int,
	domain []byte,
	privateKeys []common.SecretKey,
	source, target primitives.Epoch,
	indices []uint64,
	beaconBlockRoot []byte,
) *slashertypes.IndexedAttestationWrapper {
	// Create attestation data.
	attestationData := &ethpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo(beaconBlockRoot, 32),
		Source: &ethpb.Checkpoint{
			Epoch: source,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: target,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
	}

	// Compute attestation data root.
	attestationDataRoot, err := attestationData.HashTreeRoot()
	require.NoError(t, err)

	// Create valid signatures for all input attestations in the test.
	signingRoot, err := signing.ComputeSigningRoot(attestationData, domain)
	require.NoError(t, err)

	// For each attesting indice in the indexed attestation, create a signature.
	signatures := make([]bls.Signature, 0, len(indices))
	for _, indice := range indices {
		// Check that the indice is within the range of private keys.
		require.Equal(t, true, indice < uint64(len(privateKeys)))

		// Retrieve the corresponding private key.
		privateKey := privateKeys[indice]

		// Sign the signing root.
		signature := privateKey.Sign(signingRoot[:])

		// Append the signature to the signatures list.
		signatures = append(signatures, signature)
	}

	// Compute the aggregated signature.
	signature := bls.AggregateSignatures(signatures).Marshal()

	// Create the attestation wrapper.
	if ver >= version.Electra {
		return &slashertypes.IndexedAttestationWrapper{
			IndexedAttestation: &ethpb.IndexedAttestationElectra{
				AttestingIndices: indices,
				Data:             attestationData,
				Signature:        signature,
			},
			DataRoot: attestationDataRoot,
		}
	}

	return &slashertypes.IndexedAttestationWrapper{
		IndexedAttestation: &ethpb.IndexedAttestation{
			AttestingIndices: indices,
			Data:             attestationData,
			Signature:        signature,
		},
		DataRoot: attestationDataRoot,
	}
}
