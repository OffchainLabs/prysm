package proposer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// These cases mirror the test vectors proposed upstream on keymanager-APIs #87
// for builder-config inheritance granularity.
func TestEffectiveBuilderConfig(t *testing.T) {
	entryA := &BuilderEntry{URL: "https://a"}
	entryB := &BuilderEntry{URL: "https://b"}
	entryC := &BuilderEntry{URL: "https://c"}

	t.Run("nil per-key returns default", func(t *testing.T) {
		def := &BuilderConfig{Enabled: true}
		require.Equal(t, def, effectiveBuilderConfig(nil, def))
	})
	t.Run("nil default returns per-key", func(t *testing.T) {
		perKey := &BuilderConfig{Enabled: true}
		require.Equal(t, perKey, effectiveBuilderConfig(perKey, nil))
	})
	t.Run("min_bid inherits when per-key omits it", func(t *testing.T) {
		def := &BuilderConfig{Enabled: true, Builders: []*BuilderEntry{entryA, entryB}, MinBid: uint64ValPtr(5000000)}
		perKey := &BuilderConfig{Enabled: true, Builders: []*BuilderEntry{entryC}}
		eff := effectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MinBid)
		require.Equal(t, validator.Uint64(5000000), *eff.MinBid)
		require.Equal(t, 1, len(eff.Builders))
		require.Equal(t, "https://c", eff.Builders[0].URL)
	})
	t.Run("explicit zero max payment is preserved, not inherited over", func(t *testing.T) {
		def := &BuilderConfig{MaxExecutionPayment: uint64ValPtr(1000000000)}
		perKey := &BuilderConfig{Enabled: true, MaxExecutionPayment: uint64ValPtr(0)}
		eff := effectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MaxExecutionPayment)
		require.Equal(t, validator.Uint64(0), *eff.MaxExecutionPayment)
	})
	t.Run("unset max payment inherits default", func(t *testing.T) {
		def := &BuilderConfig{MaxExecutionPayment: uint64ValPtr(1000000000)}
		perKey := &BuilderConfig{Enabled: true}
		eff := effectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MaxExecutionPayment)
		require.Equal(t, validator.Uint64(1000000000), *eff.MaxExecutionPayment)
	})
	t.Run("explicit per-key disable wins over enabled default", func(t *testing.T) {
		def := &BuilderConfig{Enabled: true}
		perKey := &BuilderConfig{Enabled: false, MinBid: uint64ValPtr(1)}
		require.Equal(t, false, effectiveBuilderConfig(perKey, def).IsEnabled())
	})
	t.Run("present per-key builder config is authoritative on enabled", func(t *testing.T) {
		// A per-key config with enabled false does not inherit an enabled default;
		// whole-config inheritance happens only when the per-key builder config is nil.
		def := &BuilderConfig{Enabled: true}
		perKey := &BuilderConfig{MinBid: uint64ValPtr(1)}
		require.Equal(t, false, effectiveBuilderConfig(perKey, def).IsEnabled())
		require.Equal(t, true, effectiveBuilderConfig(nil, def).IsEnabled())
	})
	t.Run("present builders list replaces, never unions", func(t *testing.T) {
		def := &BuilderConfig{Builders: []*BuilderEntry{entryA, entryB}}
		perKey := &BuilderConfig{Builders: []*BuilderEntry{entryC}}
		eff := effectiveBuilderConfig(perKey, def)
		require.Equal(t, 1, len(eff.Builders))
		require.Equal(t, "https://c", eff.Builders[0].URL)
	})
	t.Run("absent builders list inherits default list", func(t *testing.T) {
		def := &BuilderConfig{Builders: []*BuilderEntry{entryA, entryB}}
		perKey := &BuilderConfig{Enabled: true}
		require.Equal(t, 2, len(effectiveBuilderConfig(perKey, def).Builders))
	})
	t.Run("zero gas limit inherits default gas limit", func(t *testing.T) {
		def := &BuilderConfig{GasLimit: validator.Uint64(30000000)}
		perKey := &BuilderConfig{Enabled: true}
		require.Equal(t, validator.Uint64(30000000), effectiveBuilderConfig(perKey, def).GasLimit)
	})
	t.Run("boost factor inherits per field", func(t *testing.T) {
		def := &BuilderConfig{MinBid: uint64ValPtr(5000000), BuilderBoostFactor: uint64ValPtr(90)}
		perKey := &BuilderConfig{Enabled: true, BuilderBoostFactor: uint64ValPtr(120)}
		eff := effectiveBuilderConfig(perKey, def)
		require.Equal(t, validator.Uint64(5000000), *eff.MinBid)
		require.Equal(t, validator.Uint64(120), *eff.BuilderBoostFactor)
	})
	t.Run("both-set min_bid: per-key wins", func(t *testing.T) {
		def := &BuilderConfig{MinBid: uint64ValPtr(5000000)}
		perKey := &BuilderConfig{MinBid: uint64ValPtr(7000000)}
		require.Equal(t, validator.Uint64(7000000), *effectiveBuilderConfig(perKey, def).MinBid)
	})
	t.Run("nonzero per-key gas limit wins over default", func(t *testing.T) {
		def := &BuilderConfig{GasLimit: validator.Uint64(30000000)}
		perKey := &BuilderConfig{GasLimit: validator.Uint64(45000000)}
		require.Equal(t, validator.Uint64(45000000), effectiveBuilderConfig(perKey, def).GasLimit)
	})
	t.Run("nil nil is nil", func(t *testing.T) {
		require.Equal(t, (*BuilderConfig)(nil), effectiveBuilderConfig(nil, nil))
	})
}
