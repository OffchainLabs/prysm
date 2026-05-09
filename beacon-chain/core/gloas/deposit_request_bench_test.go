package gloas

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func benchBuildBuilderDeposits(b *testing.B, n int) []*enginev1.DepositRequest {
	b.Helper()
	cred := builderWithdrawalCredentials()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	reqs := make([]*enginev1.DepositRequest, n)
	for i := range n {
		sk, err := bls.RandKey()
		if err != nil {
			b.Fatal(err)
		}
		sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: cred[:],
			Amount:                uint64(1 + i),
		}, domain)
		if err != nil {
			b.Fatal(err)
		}
		reqs[i] = &enginev1.DepositRequest{
			Pubkey:                sk.PublicKey().Marshal(),
			WithdrawalCredentials: cred[:],
			Amount:                uint64(1 + i),
			Signature:             sk.Sign(sr[:]).Marshal(),
			Index:                 uint64(i),
		}
	}
	return reqs
}

func benchBuildBuilderDepositsWithBad(b *testing.B, n, badIdx int) []*enginev1.DepositRequest {
	b.Helper()
	reqs := benchBuildBuilderDeposits(b, n)
	if badIdx < 0 || badIdx >= n {
		return reqs
	}
	cred := builderWithdrawalCredentials()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	sk, err := bls.RandKey()
	if err != nil {
		b.Fatal(err)
	}
	// well-formed sig but for a different message — forces full pairing check
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                uint64(badIdx + 999999),
	}, domain)
	if err != nil {
		b.Fatal(err)
	}
	reqs[badIdx] = &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                uint64(1 + badIdx),
		Signature:             sk.Sign(sr[:]).Marshal(),
		Index:                 uint64(badIdx),
	}
	return reqs
}

func benchFreshGloasState(b *testing.B) state.BeaconState {
	b.Helper()
	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		DepositRequestsStartIndex: params.BeaconConfig().UnsetDepositRequestsStartIndex,
		PendingDeposits:           []*ethpb.PendingDeposit{},
	})
	if err != nil {
		b.Fatal(err)
	}
	return st
}

func BenchmarkProcessDepositRequests_512NewBuilders_PerRequest(b *testing.B) {
	benchPerRequest(b, 512)
}

func BenchmarkProcessDepositRequests_512NewBuilders_Batched(b *testing.B) {
	benchBatched(b, 512)
}

func BenchmarkProcessDepositRequests_8192NewBuilders_PerRequest(b *testing.B) {
	benchPerRequest(b, 8192)
}

func BenchmarkProcessDepositRequests_8192NewBuilders_Batched(b *testing.B) {
	benchBatched(b, 8192)
}

func BenchmarkProcessDepositRequests_8192NewBuilders_OneBad_PerRequest(b *testing.B) {
	reqs := benchBuildBuilderDepositsWithBad(b, 8192, 4096)
	for b.Loop() {
		b.StopTimer()
		st := benchFreshGloasState(b)
		b.StartTimer()
		if err := processDepositRequestsPerRequest(st, reqs); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessDepositRequests_8192NewBuilders_OneBad_Batched(b *testing.B) {
	reqs := benchBuildBuilderDepositsWithBad(b, 8192, 4096)
	ctx := context.Background()
	for b.Loop() {
		b.StopTimer()
		st := benchFreshGloasState(b)
		b.StartTimer()
		if err := processDepositRequests(ctx, st, reqs, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessDepositRequests_8192NewBuilders_EightBad_Batched(b *testing.B) {
	reqs := benchBuildBuilderDeposits(b, 8192)
	// replace 8 evenly spread requests with well-formed-but-wrong signatures
	step := 8192 / 8
	for i := range 8 {
		idx := i * step
		reqs[idx] = makeBadDepositRequest(b, uint64(idx))
	}
	ctx := context.Background()
	for b.Loop() {
		b.StopTimer()
		st := benchFreshGloasState(b)
		b.StartTimer()
		if err := processDepositRequests(ctx, st, reqs, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func makeBadDepositRequest(b *testing.B, idx uint64) *enginev1.DepositRequest {
	b.Helper()
	cred := builderWithdrawalCredentials()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	sk, err := bls.RandKey()
	if err != nil {
		b.Fatal(err)
	}
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                idx + 999999,
	}, domain)
	if err != nil {
		b.Fatal(err)
	}
	return &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: cred[:],
		Amount:                uint64(1 + idx),
		Signature:             sk.Sign(sr[:]).Marshal(),
		Index:                 idx,
	}
}

func benchPerRequest(b *testing.B, n int) {
	reqs := benchBuildBuilderDeposits(b, n)
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		st := benchFreshGloasState(b)
		b.StartTimer()
		if err := processDepositRequestsPerRequest(st, reqs); err != nil {
			b.Fatal(err)
		}
	}
}

func benchBatched(b *testing.B, n int) {
	reqs := benchBuildBuilderDeposits(b, n)
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		st := benchFreshGloasState(b)
		b.StartTimer()
		if err := processDepositRequests(ctx, st, reqs, nil); err != nil {
			b.Fatal(err)
		}
	}
}
