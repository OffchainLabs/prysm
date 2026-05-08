package helpers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type verifyFn func(ctx context.Context, reqs []*enginev1.DepositRequest) ([]bool, error)

func tryBatchVerify(reqs []*enginev1.DepositRequest) bool {
	if len(reqs) == 0 {
		return true
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return false
	}
	pks := make([]bls.PublicKey, len(reqs))
	sigs := make([][]byte, len(reqs))
	msgs := make([][32]byte, len(reqs))
	for i, req := range reqs {
		dpk, err := bls.PublicKeyFromBytes(req.Pubkey)
		if err != nil {
			return false
		}
		pks[i] = dpk
		sigs[i] = req.Signature
		sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             req.Pubkey,
			WithdrawalCredentials: req.WithdrawalCredentials,
			Amount:                req.Amount,
		}, domain)
		if err != nil {
			return false
		}
		msgs[i] = sr
	}
	ok, err := bls.VerifyMultipleSignatures(sigs, msgs, pks)
	return err == nil && ok
}

func individualVerify(req *enginev1.DepositRequest) bool {
	ok, _ := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
		PublicKey:             req.Pubkey,
		WithdrawalCredentials: req.WithdrawalCredentials,
		Amount:                req.Amount,
		Signature:             req.Signature,
	})
	return ok
}

func verifyLinear(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
	out := make([]bool, len(reqs))
	if tryBatchVerify(reqs) {
		for i := range out {
			out[i] = true
		}
		return out, nil
	}
	for i, req := range reqs {
		out[i] = individualVerify(req)
	}
	return out, nil
}

func verifyDC(ctx context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
	return helpers.BatchVerifyDepositRequestSignatures(ctx, reqs)
}

func makeHybrid(threshold int) verifyFn {
	var rec func(reqs []*enginev1.DepositRequest, out []bool)
	rec = func(reqs []*enginev1.DepositRequest, out []bool) {
		if len(reqs) == 0 {
			return
		}
		if tryBatchVerify(reqs) {
			for i := range out {
				out[i] = true
			}
			return
		}
		if len(reqs) <= threshold {
			for i, req := range reqs {
				out[i] = individualVerify(req)
			}
			return
		}
		mid := len(reqs) / 2
		rec(reqs[:mid], out[:mid])
		rec(reqs[mid:], out[mid:])
	}
	return func(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
		out := make([]bool, len(reqs))
		rec(reqs, out)
		return out, nil
	}
}

func makeKaryDC(k int) verifyFn {
	var rec func(reqs []*enginev1.DepositRequest, out []bool)
	rec = func(reqs []*enginev1.DepositRequest, out []bool) {
		if len(reqs) == 0 {
			return
		}
		if tryBatchVerify(reqs) {
			for i := range out {
				out[i] = true
			}
			return
		}
		if len(reqs) == 1 {
			out[0] = false
			return
		}
		chunk := (len(reqs) + k - 1) / k
		for i := 0; i < len(reqs); i += chunk {
			end := min(i+chunk, len(reqs))
			rec(reqs[i:end], out[i:end])
		}
	}
	return func(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
		out := make([]bool, len(reqs))
		rec(reqs, out)
		return out, nil
	}
}

// k-ary adaptive: split into k subbatches; if ALL fail, linear; else recurse on the failing ones.
func makeKaryAdaptive(k int) verifyFn {
	var rec func(reqs []*enginev1.DepositRequest, out []bool)
	rec = func(reqs []*enginev1.DepositRequest, out []bool) {
		if len(reqs) == 0 {
			return
		}
		if tryBatchVerify(reqs) {
			for i := range out {
				out[i] = true
			}
			return
		}
		if len(reqs) == 1 {
			out[0] = false
			return
		}
		chunk := (len(reqs) + k - 1) / k
		type sub struct {
			reqs []*enginev1.DepositRequest
			out  []bool
			ok   bool
		}
		var subs []sub
		for i := 0; i < len(reqs); i += chunk {
			end := min(i+chunk, len(reqs))
			subs = append(subs, sub{reqs: reqs[i:end], out: out[i:end]})
		}
		failedCount := 0
		for i := range subs {
			subs[i].ok = tryBatchVerify(subs[i].reqs)
			if !subs[i].ok {
				failedCount++
			}
		}
		if failedCount == len(subs) {
			for i, req := range reqs {
				out[i] = individualVerify(req)
			}
			return
		}
		for _, s := range subs {
			if s.ok {
				for i := range s.out {
					s.out[i] = true
				}
			} else {
				rec(s.reqs, s.out)
			}
		}
	}
	return func(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
		out := make([]bool, len(reqs))
		rec(reqs, out)
		return out, nil
	}
}

// AdaptiveDepth: like Adaptive2 but tolerates `maxBoth` consecutive levels of
// both-halves-failing before bailing to linear. maxBoth=1 == Adaptive2.
func makeAdaptiveDepth(maxBoth int) verifyFn {
	var rec func(reqs []*enginev1.DepositRequest, out []bool, bothFailCount int)
	rec = func(reqs []*enginev1.DepositRequest, out []bool, bothFailCount int) {
		if len(reqs) == 0 {
			return
		}
		if tryBatchVerify(reqs) {
			for i := range out {
				out[i] = true
			}
			return
		}
		if len(reqs) == 1 {
			out[0] = false
			return
		}
		mid := len(reqs) / 2
		leftReqs, rightReqs := reqs[:mid], reqs[mid:]
		leftOut, rightOut := out[:mid], out[mid:]
		leftPassed := tryBatchVerify(leftReqs)
		rightPassed := tryBatchVerify(rightReqs)
		if leftPassed {
			for i := range leftOut {
				leftOut[i] = true
			}
		}
		if rightPassed {
			for i := range rightOut {
				rightOut[i] = true
			}
		}
		switch {
		case leftPassed && rightPassed:
			// can't happen if parent failed
		case !leftPassed && !rightPassed:
			newCount := bothFailCount + 1
			if newCount >= maxBoth {
				for i, req := range reqs {
					out[i] = individualVerify(req)
				}
				return
			}
			rec(leftReqs, leftOut, newCount)
			rec(rightReqs, rightOut, newCount)
		case !leftPassed:
			rec(leftReqs, leftOut, bothFailCount)
		case !rightPassed:
			rec(rightReqs, rightOut, bothFailCount)
		}
	}
	return func(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
		out := make([]bool, len(reqs))
		rec(reqs, out, 0)
		return out, nil
	}
}

// Adaptive D&C: when both halves fail, fall back to linear for that subtree.
func verifyAdaptive(_ context.Context, reqs []*enginev1.DepositRequest) ([]bool, error) {
	out := make([]bool, len(reqs))
	var rec func(reqs []*enginev1.DepositRequest, out []bool)
	rec = func(reqs []*enginev1.DepositRequest, out []bool) {
		if len(reqs) == 0 {
			return
		}
		if tryBatchVerify(reqs) {
			for i := range out {
				out[i] = true
			}
			return
		}
		if len(reqs) == 1 {
			out[0] = false
			return
		}
		mid := len(reqs) / 2
		leftReqs, rightReqs := reqs[:mid], reqs[mid:]
		leftOut, rightOut := out[:mid], out[mid:]
		leftPassed := tryBatchVerify(leftReqs)
		rightPassed := tryBatchVerify(rightReqs)
		if leftPassed {
			for i := range leftOut {
				leftOut[i] = true
			}
		}
		if rightPassed {
			for i := range rightOut {
				rightOut[i] = true
			}
		}
		switch {
		case leftPassed && rightPassed:
			// shouldn't happen if parent failed, but harmless
		case !leftPassed && !rightPassed:
			for i, req := range reqs {
				out[i] = individualVerify(req)
			}
		case !leftPassed:
			rec(leftReqs, leftOut)
		case !rightPassed:
			rec(rightReqs, rightOut)
		}
	}
	rec(reqs, out)
	return out, nil
}

func makeBenchRequest(tb testing.TB, valid bool, amount uint64) *enginev1.DepositRequest {
	tb.Helper()
	sk, err := bls.RandKey()
	if err != nil {
		tb.Fatal(err)
	}
	wc := make([]byte, 32)
	req := &enginev1.DepositRequest{
		Pubkey:                sk.PublicKey().Marshal(),
		WithdrawalCredentials: wc,
		Amount:                amount,
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		tb.Fatal(err)
	}
	signedAmount := amount
	if !valid {
		// well-formed sig but for a different message — forces full pairing check
		signedAmount = amount + 1
	}
	sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
		PublicKey:             req.Pubkey,
		WithdrawalCredentials: wc,
		Amount:                signedAmount,
	}, domain)
	if err != nil {
		tb.Fatal(err)
	}
	req.Signature = sk.Sign(sr[:]).Marshal()
	return req
}

func evenSpread(n, k int) map[int]struct{} {
	bad := make(map[int]struct{}, k)
	if k <= 0 {
		return bad
	}
	if k >= n {
		for i := range n {
			bad[i] = struct{}{}
		}
		return bad
	}
	if k > n/2 {
		for i := range n {
			bad[i] = struct{}{}
		}
		good := n - k
		step := float64(n) / float64(good)
		for i := range good {
			delete(bad, int(step*float64(i)))
		}
		return bad
	}
	step := float64(n) / float64(k)
	for i := range k {
		bad[int(step*float64(i))] = struct{}{}
	}
	return bad
}

type benchPool struct {
	valid   []*enginev1.DepositRequest
	invalid []*enginev1.DepositRequest
}

var benchPools = map[int]*benchPool{}

func getBenchPool(tb testing.TB, n int) *benchPool {
	if p, ok := benchPools[n]; ok {
		return p
	}
	p := &benchPool{
		valid:   make([]*enginev1.DepositRequest, n),
		invalid: make([]*enginev1.DepositRequest, n),
	}
	for i := range n {
		p.valid[i] = makeBenchRequest(tb, true, uint64(i+1))
		p.invalid[i] = makeBenchRequest(tb, false, uint64(i+1))
	}
	benchPools[n] = p
	return p
}

func buildScenario(tb testing.TB, n int, badIdx map[int]struct{}) []*enginev1.DepositRequest {
	tb.Helper()
	p := getBenchPool(tb, n)
	reqs := make([]*enginev1.DepositRequest, n)
	for i := range n {
		if _, bad := badIdx[i]; bad {
			reqs[i] = p.invalid[i]
		} else {
			reqs[i] = p.valid[i]
		}
	}
	return reqs
}

func BenchmarkVerifyStrategies(b *testing.B) {
	const n = 8192
	scenarios := []struct {
		name string
		k    int
	}{
		{"AllValid", 0},
		{"1Bad", 1},
		{"8Bad", 8},
		{"16Bad", 16},
		{"32Bad", 32},
		{"64Bad", 64},
		{"256Bad", 256},
		{"AllButOneBad", n - 1},
		{"AllBad", n},
	}
	strategies := []struct {
		name string
		fn   verifyFn
	}{
		{"Linear", verifyLinear},
		{"DC2", verifyDC},
		{"DC8", makeKaryDC(8)},
		{"AdaptD1", makeAdaptiveDepth(1)},
		{"AdaptD2", makeAdaptiveDepth(2)},
		{"AdaptD3", makeAdaptiveDepth(3)},
		{"AdaptD4", makeAdaptiveDepth(4)},
	}
	for _, sc := range scenarios {
		reqs := buildScenario(b, n, evenSpread(n, sc.k))
		for _, s := range strategies {
			b.Run(fmt.Sprintf("%s/%s", sc.name, s.name), func(b *testing.B) {
				ctx := context.Background()
				// warmup to dodge cold-cache penalty on first sub-benchmark
				_ = tryBatchVerify(reqs)
				b.ResetTimer()
				for range b.N {
					if _, err := s.fn(ctx, reqs); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
