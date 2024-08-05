package p2p

const (
	GossipSignedExecutionPayloadHeader   = "signed_execution_payload_header"
	GossipSignedExecutionPayloadEnvelope = "signed_execution_payload_envelope"
	GossipPayloadAttestationMessage      = "payload_attestation_message"

	SignedExecutionPayloadHeaderTopicFormat   = GossipProtocolAndDigest + GossipSignedExecutionPayloadHeader
	SignedExecutionPayloadEnvelopeTopicFormat = GossipProtocolAndDigest + GossipSignedExecutionPayloadEnvelope
	PayloadAttestationMessageTopicFormat      = GossipProtocolAndDigest + GossipPayloadAttestationMessage
)
