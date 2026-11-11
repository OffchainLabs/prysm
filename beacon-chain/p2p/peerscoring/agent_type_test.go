package peerscoring

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAgentTypeOf(t *testing.T) {
	for agent, want := range map[string]string{
		"Lighthouse/v8.2.2-e423a66/aarch64-linux":                                   AgentTypeLighthouse,
		"Lighthouse/v8.2.2/aarch64-macos":                                           AgentTypeLighthouse,
		"Prysm/v7.2.0/03f3712a184c1c7cc9fd38d3b1abc5328934cdef":                     AgentTypePrysm,
		"teku/teku/v26.3.0/linux-x86_64/-eclipseadoptium-openjdk64bitservervm-java": AgentTypeTeku,
		"nimbus":                               AgentTypeNimbus,
		"lodestar/v1.30.0/abcdef":              AgentTypeLodestar,
		"Grandine/2.0.5-70a5c7ea/x86_64-linux": AgentTypeGrandine,
		"erigon/caplin/3.6.0-ecc2ad99":         AgentTypeErigonCaplin,
		"js-libp2p/2.1.0":                      AgentTypeJSLibp2p,
		"rust-libp2p/0.48.0":                   AgentTypeRustLibp2p,
		// Several matches: the last known type wins, as p2p's agent metrics always did.
		"lighthouse/rust-libp2p": AgentTypeRustLibp2p,
		"tysm/v1.1.1/6d26e0e9":   AgentTypeUnknown,
		"hermes":                 AgentTypeUnknown,
		"":                       AgentTypeUnknown,
	} {
		require.Equal(t, want, AgentTypeOf(agent), "agent %q", agent)
	}
}

func TestAgentTypes(t *testing.T) {
	require.DeepEqual(t, []string{
		AgentTypeErigonCaplin, AgentTypeGrandine, AgentTypeJSLibp2p, AgentTypeLighthouse, AgentTypeLodestar,
		AgentTypeNimbus, AgentTypePrysm, AgentTypeTeku, AgentTypeRustLibp2p, AgentTypeUnknown,
	}, AgentTypes())
}
