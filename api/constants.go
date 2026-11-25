package api

const (
	WebUrlPrefix        = "/v2/validator/"
	WebApiUrlPrefix     = "/api/v2/validator/"
	KeymanagerApiPrefix = "/eth/v1"
	SystemLogsPrefix    = "health/logs"
	AuthTokenFileName   = "auth-token"
)

// Reasons returned by the gRPC GetHealth endpoint when the beacon node is not ready.
const (
	HealthSyncingReason    = "node is syncing"
	HealthOptimisticReason = "node is optimistic"
)
