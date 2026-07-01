package gloas

import (
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProcessWithdrawals(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) *withdrawalsState
		check func(t *testing.T, st *withdrawalsState)
	}{
		{
			name: "parent block not full",
			build: func(t *testing.T) *withdrawalsState {
				return &withdrawalsState{
					BeaconState: newGloasState(t, nil, nil),
					parentFull:  false,
				}
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, false, st.expectedCalled)
				require.Equal(t, false, st.decreaseCalled)
				require.Equal(t, false, st.setNextWithdrawalIndexCalled)
				require.Equal(t, false, st.setPayloadExpectedWithdrawalsCalled)
				require.Equal(t, false, st.dequeueBuilderCalled)
				require.Equal(t, false, st.dequeuePartialCalled)
				require.Equal(t, false, st.setNextBuilderIndexCalled)
				require.Equal(t, false, st.nextValidatorIndexCalled)
				require.Equal(t, false, st.setNextValidatorIndexCalled)
			},
		},
		{
			name: "updates indexes when not full payload",
			build: func(t *testing.T) *withdrawalsState {
				return &withdrawalsState{
					BeaconState:        newGloasState(t, nil, nil),
					parentFull:         true,
					numValidators:      10,
					nextValidatorIndex: 3,
					expectedResult: state.ExpectedWithdrawalsGloasResult{
						Withdrawals: []*enginev1.Withdrawal{
							{Index: 7, ValidatorIndex: 2, Amount: 1, Address: []byte{0x01}},
							{Index: 8, ValidatorIndex: 4, Amount: 2, Address: []byte{0x02}},
						},
						ProcessedBuilderWithdrawalsCount: 5,
						ProcessedPartialWithdrawalsCount: 2,
						NextWithdrawalBuilderIndex:       7,
					},
				}
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.expectedCalled)
				require.Equal(t, true, st.decreaseCalled)
				require.NotNil(t, st.setNextWithdrawalIndexArg)
				require.Equal(t, uint64(9), *st.setNextWithdrawalIndexArg)
				require.DeepEqual(t, st.expectedResult.Withdrawals, st.setPayloadExpectedWithdrawalsArg)
				require.Equal(t, uint64(5), *st.dequeueBuilderArg)
				require.Equal(t, uint64(2), *st.dequeuePartialArg)
				require.Equal(t, primitives.BuilderIndex(7), *st.setNextBuilderIndexArg)
				require.Equal(t, true, st.nextValidatorIndexCalled)

				expectedNext := (uint64(st.nextValidatorIndex) + uint64(params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)) % st.numValidators
				require.Equal(t, primitives.ValidatorIndex(expectedNext), *st.setNextValidatorIndexArg)
			},
		},
		{
			name: "full payload uses last validator index",
			build: func(t *testing.T) *withdrawalsState {
				max := int(params.BeaconConfig().MaxWithdrawalsPerPayload)
				withdrawals := make([]*enginev1.Withdrawal, max)
				for i := range max {
					withdrawals[i] = &enginev1.Withdrawal{
						Index:          uint64(i),
						ValidatorIndex: 0,
						Amount:         1,
						Address:        []byte{0x03},
					}
				}
				withdrawals[max-1].ValidatorIndex = 4

				return &withdrawalsState{
					BeaconState:   newGloasState(t, nil, nil),
					parentFull:    true,
					numValidators: 5,
					expectedResult: state.ExpectedWithdrawalsGloasResult{
						Withdrawals:                withdrawals,
						NextWithdrawalBuilderIndex: 1,
					},
				}
			},
			check: func(t *testing.T, st *withdrawalsState) {
				max := int(params.BeaconConfig().MaxWithdrawalsPerPayload)
				require.NotNil(t, st.setNextWithdrawalIndexArg)
				require.Equal(t, uint64(max), *st.setNextWithdrawalIndexArg)
				require.Equal(t, false, st.nextValidatorIndexCalled)
				require.Equal(t, primitives.ValidatorIndex(0), *st.setNextValidatorIndexArg)
			},
		},
		{
			name: "empty withdrawals skips next index update",
			build: func(t *testing.T) *withdrawalsState {
				return &withdrawalsState{
					BeaconState:   newGloasState(t, nil, nil),
					parentFull:    true,
					numValidators: 8,
					expectedResult: state.ExpectedWithdrawalsGloasResult{
						Withdrawals:                      []*enginev1.Withdrawal{},
						ProcessedBuilderWithdrawalsCount: 1,
						ProcessedPartialWithdrawalsCount: 2,
						NextWithdrawalBuilderIndex:       4,
					},
				}
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, false, st.setNextWithdrawalIndexCalled)
				require.Equal(t, true, st.setPayloadExpectedWithdrawalsCalled)
				require.Equal(t, true, st.dequeueBuilderCalled)
				require.Equal(t, true, st.dequeuePartialCalled)
				require.Equal(t, true, st.setNextBuilderIndexCalled)
				require.Equal(t, true, st.nextValidatorIndexCalled)
				require.Equal(t, true, st.setNextValidatorIndexCalled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.build(t)
			require.NoError(t, ProcessWithdrawals(st))
			if tc.check != nil {
				tc.check(t, st)
			}
		})
	}
}

func TestProcessWithdrawals_ErrorPaths(t *testing.T) {
	base := func(t *testing.T) *withdrawalsState {
		return &withdrawalsState{
			BeaconState:   newGloasState(t, nil, nil),
			parentFull:    true,
			numValidators: 16,
			expectedResult: state.ExpectedWithdrawalsGloasResult{
				Withdrawals: []*enginev1.Withdrawal{
					{Index: 1, ValidatorIndex: 2, Amount: 1, Address: []byte{0x01}},
				},
				ProcessedBuilderWithdrawalsCount: 1,
				ProcessedPartialWithdrawalsCount: 1,
				NextWithdrawalBuilderIndex:       2,
			},
			nextValidatorIndex: 5,
		}
	}

	cases := []struct {
		name  string
		err   error
		set   func(st *withdrawalsState, err error)
		check func(t *testing.T, st *withdrawalsState)
	}{
		{
			name: "parent block full error",
			err:  errors.New("parent err"),
			set: func(st *withdrawalsState, err error) {
				st.parentErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, false, st.expectedCalled)
			},
		},
		{
			name: "expected withdrawals error",
			err:  errors.New("expected err"),
			set: func(st *withdrawalsState, err error) {
				st.expectedErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.expectedCalled)
				require.Equal(t, false, st.decreaseCalled)
			},
		},
		{
			name: "decrease balances error",
			err:  errors.New("decrease err"),
			set: func(st *withdrawalsState, err error) {
				st.decreaseErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.decreaseCalled)
				require.Equal(t, false, st.setNextWithdrawalIndexCalled)
			},
		},
		{
			name: "set next withdrawal index error",
			err:  errors.New("next index err"),
			set: func(st *withdrawalsState, err error) {
				st.setNextWithdrawalIndexErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.setNextWithdrawalIndexCalled)
				require.Equal(t, false, st.setPayloadExpectedWithdrawalsCalled)
			},
		},
		{
			name: "set payload expected withdrawals error",
			err:  errors.New("payload expected err"),
			set: func(st *withdrawalsState, err error) {
				st.setPayloadExpectedWithdrawalsErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.setPayloadExpectedWithdrawalsCalled)
				require.Equal(t, false, st.dequeueBuilderCalled)
			},
		},
		{
			name: "dequeue builder pending withdrawals error",
			err:  errors.New("dequeue builder err"),
			set: func(st *withdrawalsState, err error) {
				st.dequeueBuilderErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.dequeueBuilderCalled)
				require.Equal(t, false, st.dequeuePartialCalled)
			},
		},
		{
			name: "dequeue pending partial withdrawals error",
			err:  errors.New("dequeue partial err"),
			set: func(st *withdrawalsState, err error) {
				st.dequeuePartialErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.dequeuePartialCalled)
				require.Equal(t, false, st.setNextBuilderIndexCalled)
			},
		},
		{
			name: "set next withdrawal builder index error",
			err:  errors.New("next builder err"),
			set: func(st *withdrawalsState, err error) {
				st.setNextBuilderIndexErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.setNextBuilderIndexCalled)
				require.Equal(t, false, st.nextValidatorIndexCalled)
			},
		},
		{
			name: "next withdrawal validator index error",
			err:  errors.New("next validator err"),
			set: func(st *withdrawalsState, err error) {
				st.nextValidatorIndexErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.nextValidatorIndexCalled)
				require.Equal(t, false, st.setNextValidatorIndexCalled)
			},
		},
		{
			name: "set next withdrawal validator index error",
			err:  errors.New("set next validator err"),
			set: func(st *withdrawalsState, err error) {
				st.setNextValidatorIndexErr = err
			},
			check: func(t *testing.T, st *withdrawalsState) {
				require.Equal(t, true, st.setNextValidatorIndexCalled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := base(t)
			tc.set(st, tc.err)
			err := ProcessWithdrawals(st)
			require.ErrorIs(t, err, tc.err)
			if tc.check != nil {
				tc.check(t, st)
			}
		})
	}
}

type withdrawalsState struct {
	setNextValidatorIndexCalled         bool
	nextValidatorIndexCalled            bool
	setNextBuilderIndexCalled           bool
	dequeuePartialCalled                bool
	dequeueBuilderCalled                bool
	setPayloadExpectedWithdrawalsCalled bool
	setNextWithdrawalIndexCalled        bool
	parentFull                          bool
	expectedCalled                      bool
	decreaseCalled                      bool
	numValidators                       uint64
	setNextWithdrawalIndexArg           *uint64
	nextValidatorIndex                  primitives.ValidatorIndex
	setNextBuilderIndexArg              *primitives.BuilderIndex
	dequeuePartialArg                   *uint64
	setNextValidatorIndexArg            *primitives.ValidatorIndex
	dequeueBuilderArg                   *uint64
	state.BeaconState
	setNextValidatorIndexErr         error
	setNextBuilderIndexErr           error
	dequeuePartialErr                error
	dequeueBuilderErr                error
	setPayloadExpectedWithdrawalsErr error
	nextValidatorIndexErr            error
	decreaseErr                      error
	expectedErr                      error
	parentErr                        error
	setNextWithdrawalIndexErr        error
	setPayloadExpectedWithdrawalsArg []*enginev1.Withdrawal
	expectedResult                   state.ExpectedWithdrawalsGloasResult
}

func (w *withdrawalsState) LatestBlockHashMatchesBidBlockHash() (bool, error) {
	return w.parentFull, w.parentErr
}

func (w *withdrawalsState) ExpectedWithdrawalsGloas() (state.ExpectedWithdrawalsGloasResult, error) {
	w.expectedCalled = true
	if w.expectedErr != nil {
		return state.ExpectedWithdrawalsGloasResult{}, w.expectedErr
	}
	return w.expectedResult, nil
}

func (w *withdrawalsState) DecreaseWithdrawalBalances(_ []*enginev1.Withdrawal) error {
	w.decreaseCalled = true
	return w.decreaseErr
}

func (w *withdrawalsState) SetNextWithdrawalIndex(index uint64) error {
	w.setNextWithdrawalIndexCalled = true
	w.setNextWithdrawalIndexArg = &index
	return w.setNextWithdrawalIndexErr
}

func (w *withdrawalsState) SetPayloadExpectedWithdrawals(withdrawals []*enginev1.Withdrawal) error {
	w.setPayloadExpectedWithdrawalsCalled = true
	w.setPayloadExpectedWithdrawalsArg = withdrawals
	return w.setPayloadExpectedWithdrawalsErr
}

func (w *withdrawalsState) DequeueBuilderPendingWithdrawals(n uint64) error {
	w.dequeueBuilderCalled = true
	w.dequeueBuilderArg = &n
	return w.dequeueBuilderErr
}

func (w *withdrawalsState) DequeuePendingPartialWithdrawals(n uint64) error {
	w.dequeuePartialCalled = true
	w.dequeuePartialArg = &n
	return w.dequeuePartialErr
}

func (w *withdrawalsState) SetNextWithdrawalBuilderIndex(index primitives.BuilderIndex) error {
	w.setNextBuilderIndexCalled = true
	w.setNextBuilderIndexArg = &index
	return w.setNextBuilderIndexErr
}

func (w *withdrawalsState) NextWithdrawalValidatorIndex() (primitives.ValidatorIndex, error) {
	w.nextValidatorIndexCalled = true
	if w.nextValidatorIndexErr != nil {
		return 0, w.nextValidatorIndexErr
	}
	return w.nextValidatorIndex, nil
}

func (w *withdrawalsState) NumValidators() int {
	return int(w.numValidators)
}

func (w *withdrawalsState) SetNextWithdrawalValidatorIndex(index primitives.ValidatorIndex) error {
	w.setNextValidatorIndexCalled = true
	w.setNextValidatorIndexArg = &index
	return w.setNextValidatorIndexErr
}
