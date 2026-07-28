package structs

type SyncStatusResponse struct {
	Data *SyncStatusResponseData `json:"data"`
}

type SyncStatusResponseData struct {
	HeadSlot     string `json:"head_slot"`
	SyncDistance string `json:"sync_distance"`
	IsSyncing    bool   `json:"is_syncing"`
	IsOptimistic bool   `json:"is_optimistic"`
	ElOffline    bool   `json:"el_offline"`
}

type GetIdentityResponse struct {
	Data *Identity `json:"data"`
}

type Identity struct {
	PeerId             string    `json:"peer_id"`
	Enr                string    `json:"enr"`
	P2PAddresses       []string  `json:"p2p_addresses"`
	DiscoveryAddresses []string  `json:"discovery_addresses"`
	Metadata           *Metadata `json:"metadata"`
}

type Metadata struct {
	SeqNumber string `json:"seq_number"`
	Attnets   string `json:"attnets"`
	Syncnets  string `json:"syncnets,omitempty"`
	Cgc       string `json:"custody_group_count,omitempty"`
}

type GetPeerResponse struct {
	Data *Peer `json:"data"`
}

// Added Meta to align with beacon-api: https://ethereum.github.io/beacon-APIs/#/Node/getPeers
type Meta struct {
	Count int `json:"count"`
}

type GetPeersResponse struct {
	Data []*Peer `json:"data"`
	Meta Meta    `json:"meta"`
}

type Peer struct {
	PeerId             string `json:"peer_id"`
	Enr                string `json:"enr"`
	LastSeenP2PAddress string `json:"last_seen_p2p_address"`
	State              string `json:"state"`
	Direction          string `json:"direction"`
}

type GetPeerCountResponse struct {
	Data *PeerCount `json:"data"`
}

type PeerCount struct {
	Disconnected  string `json:"disconnected"`
	Connecting    string `json:"connecting"`
	Connected     string `json:"connected"`
	Disconnecting string `json:"disconnecting"`
}
type GetVersionV2Response struct {
	Data *VersionV2 `json:"data"`
}
type VersionV2 struct {
	BeaconNode      *ClientVersionV1 `json:"beacon_node"`
	ExecutionClient *ClientVersionV1 `json:"execution_client,omitempty"`
}
type ClientVersionV1 struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type GetVersionResponse struct {
	Data *Version `json:"data"`
}

type Version struct {
	Version string `json:"version"`
}

type AddrRequest struct {
	Addr string `json:"addr"`
}

type PeersResponse struct {
	Peers []*Peer `json:"peers"`
}

type GetCustodyResponse struct {
	Data *CustodyData `json:"data"`
}

type CustodyData struct {
	CustodyGroupCount string   `json:"custody_group_count"`
	CustodyGroups     []string `json:"custody_groups"`
	CustodyColumns    []string `json:"custody_columns"`
	// EarliestAvailableSlot is the earliest slot for which the node can serve the data it custodies.
	EarliestAvailableSlot string `json:"earliest_available_slot"`
	// IsSupernode is true when the node custodies all custody groups.
	IsSupernode bool `json:"is_supernode"`
	// IsSemiSupernode is true when the node custodies enough custody groups to reconstruct
	// all data columns, but not all of them.
	IsSemiSupernode bool `json:"is_semi_supernode"`
	// Backfill reports historical backfill progress for checkpoint-synced nodes.
	// It is omitted for nodes synced from genesis.
	Backfill *BackfillStatus `json:"backfill,omitempty"`
}

type BackfillStatus struct {
	OriginSlot string `json:"origin_slot"`
	// LowSlot is the lowest slot that backfill has filled so far.
	LowSlot string `json:"low_slot"`
}
