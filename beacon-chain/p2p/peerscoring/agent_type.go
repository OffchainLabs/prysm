package peerscoring

import (
	"slices"
	"strings"
)

// Agent types a peer's libp2p agent string is classified into; distinct agents
// (e.g. two Lighthouse builds) share one agent type.
const (
	AgentTypeErigonCaplin = "erigon/caplin"
	AgentTypeGrandine     = "grandine"
	AgentTypeJSLibp2p     = "js-libp2p"
	AgentTypeLighthouse   = "lighthouse"
	AgentTypeLodestar     = "lodestar"
	AgentTypeNimbus       = "nimbus"
	AgentTypePrysm        = "prysm"
	AgentTypeTeku         = "teku"
	AgentTypeRustLibp2p   = "rust-libp2p"
	AgentTypeUnknown      = "unknown"
)

// knownAgentTypes is the list p2p's agent metrics have always matched against.
var knownAgentTypes = []string{
	AgentTypeErigonCaplin,
	AgentTypeGrandine,
	AgentTypeJSLibp2p,
	AgentTypeLighthouse,
	AgentTypeLodestar,
	AgentTypeNimbus,
	AgentTypePrysm,
	AgentTypeTeku,
	AgentTypeRustLibp2p,
}

// AgentTypeOf classifies a libp2p agent string by case-insensitive substring match; when
// several known types match, the last one in knownAgentTypes wins.
func AgentTypeOf(agent string) string {
	agent = strings.ToLower(agent)
	found := AgentTypeUnknown
	for _, agentType := range knownAgentTypes {
		if strings.Contains(agent, agentType) {
			found = agentType
		}
	}
	return found
}

// AgentTypes returns every agent type, unknown included.
func AgentTypes() []string {
	return append(slices.Clone(knownAgentTypes), AgentTypeUnknown)
}
