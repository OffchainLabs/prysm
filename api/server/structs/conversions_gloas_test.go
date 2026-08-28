package structs

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func consensusBuilderEntry() *ethpb.BuilderEntry {
	return &ethpb.BuilderEntry{
		Url: []byte("http://builder.example"),
		Auth: &ethpb.SignedRequestAuth{
			Message:   &ethpb.RequestAuth{Data: []byte{0xaa, 0xbb}, Slot: 7},
			Signature: make([]byte, 96),
		},
		BuilderPubkeys:      [][]byte{make([]byte, 48)},
		MaxExecutionPayment: primitives.Gwei(1000),
		MinBid:              primitives.Gwei(2),
		BuilderBoostFactor:  90,
	}
}

func TestBuilderConfig_ToConsensus(t *testing.T) {
	consensusCfg := &ethpb.BuilderConfig{
		MinBid:             primitives.Gwei(1),
		BuilderBoostFactor: 100,
		Builders:           []*ethpb.BuilderEntry{consensusBuilderEntry()},
	}

	t.Run("round trip", func(t *testing.T) {
		got, err := BuilderConfigFromConsensus(consensusCfg).ToConsensus()
		require.NoError(t, err)
		require.DeepEqual(t, consensusCfg, got)
	})

	t.Run("too many builders", func(t *testing.T) {
		cfg := BuilderConfigFromConsensus(consensusCfg)
		entry := cfg.Builders[0]
		for range maxBuilderEntries {
			cfg.Builders = append(cfg.Builders, entry)
		}
		_, err := cfg.ToConsensus()
		require.ErrorContains(t, "more than 64 items", err)
	})

	t.Run("invalid min bid", func(t *testing.T) {
		cfg := BuilderConfigFromConsensus(consensusCfg)
		cfg.MinBid = "not-a-number"
		_, err := cfg.ToConsensus()
		require.ErrorContains(t, "MinBid", err)
	})
}

func TestBuilderEntry_ToConsensus(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		entry := consensusBuilderEntry()
		got, err := BuilderEntryFromConsensus(entry).ToConsensus()
		require.NoError(t, err)
		require.DeepEqual(t, entry, got)
	})

	t.Run("empty url", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.Url = ""
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Url", err)
	})

	t.Run("url too long", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.Url = "http://" + strings.Repeat("a", maxBuilderUrlLength)
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Url", err)
	})

	t.Run("nil auth", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.Auth = nil
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Auth", err)
	})

	t.Run("empty auth data", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.Auth.Message.Data = "0x"
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Data", err)
	})

	t.Run("auth data too long", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.Auth.Message.Data = "0x" + strings.Repeat("ab", maxRequestAuthDataLength+1)
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Data", err)
	})

	t.Run("too many builder pubkeys", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		pk := entry.BuilderPubkeys[0]
		for range maxBuilderPubkeys {
			entry.BuilderPubkeys = append(entry.BuilderPubkeys, pk)
		}
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "BuilderPubkeys", err)
	})

	t.Run("bad builder pubkey length", func(t *testing.T) {
		entry := BuilderEntryFromConsensus(consensusBuilderEntry())
		entry.BuilderPubkeys[0] = "0xff"
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "BuilderPubkeys[0]", err)
	})
}

func TestBuilderPreferencesEntry_ToConsensus(t *testing.T) {
	consensusEntry := &ethpb.BuilderPreferencesEntry{
		ProposerPubkey: make([]byte, 48),
		Url:            []byte("http://builder.example"),
		Auth: &ethpb.SignedRequestAuth{
			Message:   &ethpb.RequestAuth{Data: []byte{0xaa}, Slot: 3},
			Signature: make([]byte, 96),
		},
		MaxExecutionPayment: primitives.Gwei(500),
	}

	t.Run("round trip", func(t *testing.T) {
		got, err := BuilderPreferencesEntryFromConsensus(consensusEntry).ToConsensus()
		require.NoError(t, err)
		require.DeepEqual(t, consensusEntry, got)
	})

	t.Run("bad proposer pubkey", func(t *testing.T) {
		entry := BuilderPreferencesEntryFromConsensus(consensusEntry)
		entry.ProposerPubkey = "0xff"
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "ProposerPubkey", err)
	})

	t.Run("bad signature length", func(t *testing.T) {
		entry := BuilderPreferencesEntryFromConsensus(consensusEntry)
		entry.Auth.Signature = "0xff"
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "Signature", err)
	})

	t.Run("invalid max execution payment", func(t *testing.T) {
		entry := BuilderPreferencesEntryFromConsensus(consensusEntry)
		entry.MaxExecutionPayment = "-1"
		_, err := entry.ToConsensus()
		require.ErrorContains(t, "MaxExecutionPayment", err)
	})
}
