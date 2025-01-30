package electra_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestProcessWithdrawRequests(t *testing.T) {
	logHook := test.NewGlobal()
	source, err := hexutil.Decode("0xb20a608c624Ca5003905aA834De7156C68b2E1d0")
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisStateElectra(t, 1)
	currentSlot := primitives.Slot(uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().ShardCommitteePeriod) + 1)
	require.NoError(t, st.SetSlot(currentSlot))
	val, err := st.ValidatorAtIndex(0)
	require.NoError(t, err)
	type args struct {
		st  state.BeaconState
		wrs []*enginev1.WithdrawalRequest
	}
	tests := []struct {
		name    string
		args    args
		wantFn  func(t *testing.T, got state.BeaconState)
		wantErr bool
	}{
		{
			name: "happy path exit and withdrawal only",
			args: args{
				st: func() state.BeaconState {
					preSt := st.Copy()
					require.NoError(t, preSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
						Index:             0,
						Amount:            params.BeaconConfig().FullExitRequestAmount,
						WithdrawableEpoch: 0,
					}))
					v, err := preSt.ValidatorAtIndex(0)
					require.NoError(t, err)
					prefix := make([]byte, 12)
					prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					v.WithdrawalCredentials = append(prefix, source...)
					require.NoError(t, preSt.SetValidators([]*eth.Validator{v}))
					return preSt
				}(),
				wrs: []*enginev1.WithdrawalRequest{
					{
						SourceAddress:   source,
						ValidatorPubkey: bytesutil.SafeCopyBytes(val.PublicKey),
						Amount:          params.BeaconConfig().FullExitRequestAmount,
					},
				},
			},
			wantFn: func(t *testing.T, got state.BeaconState) {
				wantPostSt := st.Copy()
				v, err := wantPostSt.ValidatorAtIndex(0)
				require.NoError(t, err)
				prefix := make([]byte, 12)
				prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
				v.WithdrawalCredentials = append(prefix, source...)
				v.ExitEpoch = 261
				v.WithdrawableEpoch = 517
				require.NoError(t, wantPostSt.SetValidators([]*eth.Validator{v}))
				require.NoError(t, wantPostSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
					Index:             0,
					Amount:            params.BeaconConfig().FullExitRequestAmount,
					WithdrawableEpoch: 0,
				}))
				_, err = wantPostSt.ExitEpochAndUpdateChurn(primitives.Gwei(v.EffectiveBalance))
				require.NoError(t, err)
				webc, err := wantPostSt.ExitBalanceToConsume()
				require.NoError(t, err)
				gebc, err := got.ExitBalanceToConsume()
				require.NoError(t, err)
				require.DeepEqual(t, webc, gebc)
				weee, err := wantPostSt.EarliestExitEpoch()
				require.NoError(t, err)
				geee, err := got.EarliestExitEpoch()
				require.NoError(t, err)
				require.DeepEqual(t, weee, geee)
			},
		},
		{
			name: "happy path has compounding",
			args: args{
				st: func() state.BeaconState {
					preSt := st.Copy()
					require.NoError(t, preSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
						Index:             0,
						Amount:            params.BeaconConfig().FullExitRequestAmount,
						WithdrawableEpoch: 0,
					}))
					v, err := preSt.ValidatorAtIndex(0)
					require.NoError(t, err)
					prefix := make([]byte, 12)
					prefix[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
					v.WithdrawalCredentials = append(prefix, source...)
					require.NoError(t, preSt.SetValidators([]*eth.Validator{v}))
					require.NoError(t, preSt.SetBalances([]uint64{params.BeaconConfig().MinActivationBalance + 200}))
					require.NoError(t, preSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
						Index:             0,
						Amount:            100,
						WithdrawableEpoch: 0,
					}))
					return preSt
				}(),
				wrs: []*enginev1.WithdrawalRequest{
					{
						SourceAddress:   source,
						ValidatorPubkey: bytesutil.SafeCopyBytes(val.PublicKey),
						Amount:          100,
					},
				},
			},
			wantFn: func(t *testing.T, got state.BeaconState) {
				wantPostSt := st.Copy()
				v, err := wantPostSt.ValidatorAtIndex(0)
				require.NoError(t, err)
				prefix := make([]byte, 12)
				prefix[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
				v.WithdrawalCredentials = append(prefix, source...)
				require.NoError(t, wantPostSt.SetValidators([]*eth.Validator{v}))
				bal, err := wantPostSt.BalanceAtIndex(0)
				require.NoError(t, err)
				bal += 200
				require.NoError(t, wantPostSt.SetBalances([]uint64{bal}))
				require.NoError(t, wantPostSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
					Index:             0,
					Amount:            0,
					WithdrawableEpoch: 0,
				}))
				require.NoError(t, wantPostSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
					Index:             0,
					Amount:            100,
					WithdrawableEpoch: 0,
				}))
				require.NoError(t, wantPostSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
					Index:             0,
					Amount:            100,
					WithdrawableEpoch: 517,
				}))
				wnppw, err := wantPostSt.NumPendingPartialWithdrawals()
				require.NoError(t, err)
				gnppw, err := got.NumPendingPartialWithdrawals()
				require.NoError(t, err)
				require.Equal(t, wnppw, gnppw)
				wece, err := wantPostSt.EarliestConsolidationEpoch()
				require.NoError(t, err)
				gece, err := got.EarliestConsolidationEpoch()
				require.NoError(t, err)
				require.Equal(t, wece, gece)
				_, err = wantPostSt.ExitEpochAndUpdateChurn(primitives.Gwei(100))
				require.NoError(t, err)
				webc, err := wantPostSt.ExitBalanceToConsume()
				require.NoError(t, err)
				gebc, err := got.ExitBalanceToConsume()
				require.NoError(t, err)
				require.DeepEqual(t, webc, gebc)
			},
		},
		{
			name: "validator already submitted exit",
			args: args{
				st: func() state.BeaconState {
					preSt := st.Copy()
					v, err := preSt.ValidatorAtIndex(0)
					require.NoError(t, err)
					prefix := make([]byte, 12)
					prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					v.WithdrawalCredentials = append(prefix, source...)
					v.ExitEpoch = 1000
					require.NoError(t, preSt.SetValidators([]*eth.Validator{v}))
					return preSt
				}(),
				wrs: []*enginev1.WithdrawalRequest{
					{
						SourceAddress:   source,
						ValidatorPubkey: bytesutil.SafeCopyBytes(val.PublicKey),
						Amount:          params.BeaconConfig().FullExitRequestAmount,
					},
				},
			},
			wantFn: func(t *testing.T, got state.BeaconState) {
				wantPostSt := st.Copy()
				v, err := wantPostSt.ValidatorAtIndex(0)
				require.NoError(t, err)
				prefix := make([]byte, 12)
				prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
				v.WithdrawalCredentials = append(prefix, source...)
				v.ExitEpoch = 1000
				require.NoError(t, wantPostSt.SetValidators([]*eth.Validator{v}))
				eee, err := got.EarliestExitEpoch()
				require.NoError(t, err)
				require.Equal(t, eee, primitives.Epoch(0))
				require.DeepEqual(t, wantPostSt.Validators(), got.Validators())
			},
		},
		{
			name: "validator too new",
			args: args{
				st: func() state.BeaconState {
					preSt := st.Copy()
					require.NoError(t, preSt.SetSlot(0))
					v, err := preSt.ValidatorAtIndex(0)
					require.NoError(t, err)
					prefix := make([]byte, 12)
					prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					v.WithdrawalCredentials = append(prefix, source...)
					require.NoError(t, preSt.SetValidators([]*eth.Validator{v}))
					return preSt
				}(),
				wrs: []*enginev1.WithdrawalRequest{
					{
						SourceAddress:   source,
						ValidatorPubkey: bytesutil.SafeCopyBytes(val.PublicKey),
						Amount:          params.BeaconConfig().FullExitRequestAmount,
					},
				},
			},
			wantFn: func(t *testing.T, got state.BeaconState) {
				wantPostSt := st.Copy()
				require.NoError(t, wantPostSt.SetSlot(0))
				v, err := wantPostSt.ValidatorAtIndex(0)
				require.NoError(t, err)
				prefix := make([]byte, 12)
				prefix[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
				v.WithdrawalCredentials = append(prefix, source...)
				require.NoError(t, wantPostSt.SetValidators([]*eth.Validator{v}))
				eee, err := got.EarliestExitEpoch()
				require.NoError(t, err)
				require.Equal(t, eee, primitives.Epoch(0))
				require.DeepEqual(t, wantPostSt.Validators(), got.Validators())
			},
		},
		{
			name: "PendingPartialWithdrawalsLimit reached with partial withdrawal results in a skip",
			args: args{
				st: func() state.BeaconState {
					cfg := params.BeaconConfig().Copy()
					cfg.PendingPartialWithdrawalsLimit = 1
					params.OverrideBeaconConfig(cfg)
					logrus.SetLevel(logrus.DebugLevel)
					preSt := st.Copy()
					require.NoError(t, preSt.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{
						Index:             0,
						Amount:            params.BeaconConfig().FullExitRequestAmount,
						WithdrawableEpoch: 0,
					}))
					return preSt
				}(),
				wrs: []*enginev1.WithdrawalRequest{
					{
						SourceAddress:   source,
						ValidatorPubkey: bytesutil.SafeCopyBytes(val.PublicKey),
						Amount:          100,
					},
				},
			},
			wantFn: func(t *testing.T, got state.BeaconState) {
				assert.LogsContain(t, logHook, "Skipping execution layer withdrawal request, PendingPartialWithdrawalsLimit reached")
				params.SetupTestConfigCleanup(t)
				logrus.SetLevel(logrus.InfoLevel) // reset
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := electra.ProcessWithdrawalRequests(context.Background(), tt.args.st, tt.args.wrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessWithdrawalRequests() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.wantFn(t, got)
		})
	}
}
