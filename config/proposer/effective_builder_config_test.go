package proposer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

// These cases mirror the test vectors proposed upstream on keymanager-APIs #87
// for builder-config inheritance granularity.
func TestEffectiveBuilderConfig(t *testing.T) {
	entryA := &BuilderEntry{URL: "https://a"}
	entryB := &BuilderEntry{URL: "https://b"}
	entryC := &BuilderEntry{URL: "https://c"}

	t.Run("nil per-key returns default", func(t *testing.T) {
		def := &BuilderConfig{Enabled: proto.Bool(true)}
		require.Equal(t, def, EffectiveBuilderConfig(nil, def))
	})
	t.Run("nil default returns per-key", func(t *testing.T) {
		perKey := &BuilderConfig{Enabled: proto.Bool(true)}
		require.Equal(t, perKey, EffectiveBuilderConfig(perKey, nil))
	})
	t.Run("min_bid inherits when per-key omits it", func(t *testing.T) {
		def := &BuilderConfig{Enabled: proto.Bool(true), Builders: []*BuilderEntry{entryA, entryB}, MinBid: uint64ValPtr(5000000)}
		perKey := &BuilderConfig{Enabled: proto.Bool(true), Builders: []*BuilderEntry{entryC}}
		eff := EffectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MinBid)
		require.Equal(t, validator.Uint64(5000000), *eff.MinBid)
		require.Equal(t, 1, len(eff.Builders))
		require.Equal(t, "https://c", eff.Builders[0].URL)
	})
	t.Run("explicit zero max payment is preserved, not inherited over", func(t *testing.T) {
		def := &BuilderConfig{MaxExecutionPayment: uint64ValPtr(1000000000)}
		perKey := &BuilderConfig{Enabled: proto.Bool(true), MaxExecutionPayment: uint64ValPtr(0)}
		eff := EffectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MaxExecutionPayment)
		require.Equal(t, validator.Uint64(0), *eff.MaxExecutionPayment)
	})
	t.Run("unset max payment inherits default", func(t *testing.T) {
		def := &BuilderConfig{MaxExecutionPayment: uint64ValPtr(1000000000)}
		perKey := &BuilderConfig{Enabled: proto.Bool(true)}
		eff := EffectiveBuilderConfig(perKey, def)
		require.NotNil(t, eff.MaxExecutionPayment)
		require.Equal(t, validator.Uint64(1000000000), *eff.MaxExecutionPayment)
	})
	t.Run("explicit per-key disable wins over enabled default", func(t *testing.T) {
		def := &BuilderConfig{Enabled: proto.Bool(true)}
		perKey := &BuilderConfig{Enabled: proto.Bool(false), MinBid: uint64ValPtr(1)}
		require.Equal(t, false, EffectiveBuilderConfig(perKey, def).IsEnabled())
	})
	t.Run("unset per-key enabled inherits enabled default", func(t *testing.T) {
		def := &BuilderConfig{Enabled: proto.Bool(true)}
		perKey := &BuilderConfig{MinBid: uint64ValPtr(1)}
		require.Equal(t, true, EffectiveBuilderConfig(perKey, def).IsEnabled())
	})
	t.Run("present builders list replaces, never unions", func(t *testing.T) {
		def := &BuilderConfig{Builders: []*BuilderEntry{entryA, entryB}}
		perKey := &BuilderConfig{Builders: []*BuilderEntry{entryC}}
		eff := EffectiveBuilderConfig(perKey, def)
		require.Equal(t, 1, len(eff.Builders))
		require.Equal(t, "https://c", eff.Builders[0].URL)
	})
	t.Run("absent builders list inherits default list", func(t *testing.T) {
		def := &BuilderConfig{Builders: []*BuilderEntry{entryA, entryB}}
		perKey := &BuilderConfig{Enabled: proto.Bool(true)}
		require.Equal(t, 2, len(EffectiveBuilderConfig(perKey, def).Builders))
	})
	t.Run("zero gas limit inherits default gas limit", func(t *testing.T) {
		def := &BuilderConfig{GasLimit: validator.Uint64(30000000)}
		perKey := &BuilderConfig{Enabled: proto.Bool(true)}
		require.Equal(t, validator.Uint64(30000000), EffectiveBuilderConfig(perKey, def).GasLimit)
	})
	t.Run("proxy and boost inherit per field", func(t *testing.T) {
		def := &BuilderConfig{Proxy: proto.String("http://side-car:9001"), BuilderBoostFactor: uint64ValPtr(90)}
		perKey := &BuilderConfig{Enabled: proto.Bool(true), BuilderBoostFactor: uint64ValPtr(120)}
		eff := EffectiveBuilderConfig(perKey, def)
		require.Equal(t, "http://side-car:9001", *eff.Proxy)
		require.Equal(t, validator.Uint64(120), *eff.BuilderBoostFactor)
	})
	t.Run("both-set min_bid: per-key wins", func(t *testing.T) {
		def := &BuilderConfig{MinBid: uint64ValPtr(5000000)}
		perKey := &BuilderConfig{MinBid: uint64ValPtr(7000000)}
		require.Equal(t, validator.Uint64(7000000), *EffectiveBuilderConfig(perKey, def).MinBid)
	})
	t.Run("both-set proxy: per-key wins", func(t *testing.T) {
		def := &BuilderConfig{Proxy: proto.String("http://default:1")}
		perKey := &BuilderConfig{Proxy: proto.String("http://mine:2")}
		require.Equal(t, "http://mine:2", *EffectiveBuilderConfig(perKey, def).Proxy)
	})
	t.Run("nonzero per-key gas limit wins over default", func(t *testing.T) {
		def := &BuilderConfig{GasLimit: validator.Uint64(30000000)}
		perKey := &BuilderConfig{GasLimit: validator.Uint64(45000000)}
		require.Equal(t, validator.Uint64(45000000), EffectiveBuilderConfig(perKey, def).GasLimit)
	})
	t.Run("present relays replace default relays", func(t *testing.T) {
		def := &BuilderConfig{Relays: []string{"https://r-def"}}
		perKey := &BuilderConfig{Relays: []string{"https://r-key"}}
		eff := EffectiveBuilderConfig(perKey, def)
		require.Equal(t, 1, len(eff.Relays))
		require.Equal(t, "https://r-key", eff.Relays[0])
	})
	t.Run("absent relays inherit default relays", func(t *testing.T) {
		def := &BuilderConfig{Relays: []string{"https://r-def"}}
		perKey := &BuilderConfig{Enabled: proto.Bool(true)}
		require.Equal(t, 1, len(EffectiveBuilderConfig(perKey, def).Relays))
	})
	// Builders and relays are independent fields: setting one per-key does not
	// suppress inheriting the other. Documented composite, pinned deliberately.
	t.Run("per-key relays plus default builders compose", func(t *testing.T) {
		def := &BuilderConfig{Builders: []*BuilderEntry{entryA}}
		perKey := &BuilderConfig{Relays: []string{"https://r-key"}}
		eff := EffectiveBuilderConfig(perKey, def)
		require.Equal(t, 1, len(eff.Builders))
		require.Equal(t, 1, len(eff.Relays))
	})
	t.Run("nil nil is nil", func(t *testing.T) {
		require.Equal(t, (*BuilderConfig)(nil), EffectiveBuilderConfig(nil, nil))
	})
}
