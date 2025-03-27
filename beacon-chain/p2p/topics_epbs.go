package p2p

const (
	GossipSignedExecutionPayloadHeader   = "execution_payload_header"
	GossipSignedExecutionPayloadEnvelope = "execution_payload"
	GossipPayloadAttestationMessage      = "payload_attestation_message"

	SignedExecutionPayloadHeaderTopicFormat   = GossipProtocolAndDigest + GossipSignedExecutionPayloadHeader
	SignedExecutionPayloadEnvelopeTopicFormat = GossipProtocolAndDigest + GossipSignedExecutionPayloadEnvelope
	PayloadAttestationMessageTopicFormat      = GossipProtocolAndDigest + GossipPayloadAttestationMessage
)
