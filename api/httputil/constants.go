package httputil

const (
	WebUrlPrefix        = "/v2/validator/"
	WebApiUrlPrefix     = "/api/v2/validator/"
	KeymanagerApiPrefix = "/eth/v1"
	SystemLogsPrefix    = "health/logs"
	AuthTokenFileName   = "auth-token"
)

const (
	MaxBodySize      int64 = 1 << 23 // 8MB default, WithMaxBodySize can override
	MaxBodySizeState int64 = 1 << 29 // 512MB
	MaxErrBodySize   int64 = 1 << 17 // 128KB
)
