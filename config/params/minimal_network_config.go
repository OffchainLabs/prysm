package params

func MinimalNetSpecConfig() *NetworkConfig {
	minimalNetConfig := BeaconNetworkConfig().Copy()
	// Networking
	// Properties in comments are consistent in minimal and mainnet
	// GossipMaxSize & GossipMaxSizeBellatrix
	// MaxChunkSize & MaxChunkSizeBellatrix
	// AttestationSubnetCount
	// MaxRequestBlocks
	// TtfbTimeout
	// RespTimeout
	// MaximumGossipClockDisparity
	// MessageDomainInvalidSnappy MessageDomainValidSnappy
	// MinEpochsForBlobsSidecarsRequest
	// MaxRequestBlobSidecars
	// MaxRequestBlocksDeneb
	return minimalNetConfig
}
