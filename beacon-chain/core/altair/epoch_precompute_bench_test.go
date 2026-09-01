package altair_test

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// Results at 16,384 validators (Apple M4 Pro, darwin/arm64, -benchtime=20x),
// before and after converting []*precompute.Validator / []*AttDelta to value slices:
//
//	                                      ns/op    B/op     allocs/op
//	InitializePrecomputeValidators
//	  pointer slices (before)             907479   1573280  16395
//	  value slices   (after)              850990   1311136  11
//	AttestationsDelta
//	  pointer slices (before)             239548   917504   16385
//	  value slices   (after)              90319    786434   1
const benchPrecomputeValidatorCount = 16384

func BenchmarkInitializePrecomputeValidators(b *testing.B) {
	st, _ := util.DeterministicGenesisStateAltair(b, benchPrecomputeValidatorCount)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := altair.InitializePrecomputeValidators(ctx, st); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAttestationsDelta(b *testing.B) {
	st, _ := util.DeterministicGenesisStateAltair(b, benchPrecomputeValidatorCount)
	ctx := context.Background()
	vp, bal, err := altair.InitializePrecomputeValidators(ctx, st)
	if err != nil {
		b.Fatal(err)
	}
	vp, bal, err = altair.ProcessEpochParticipation(ctx, st, bal, vp)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := altair.AttestationsDelta(st, bal, vp); err != nil {
			b.Fatal(err)
		}
	}
}
