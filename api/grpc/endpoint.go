package grpc

import (
	"net/url"
	"strings"

	logutil "github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/sirupsen/logrus"
)

// ParseGRPCEndpoints splits a comma-separated endpoint string into normalized gRPC targets.
func ParseGRPCEndpoints(endpoint string) []string {
	if endpoint == "" {
		return nil
	}
	endpoints := make([]string, 0, 1)
	for p := range strings.SplitSeq(endpoint, ",") {
		if p = strings.TrimSpace(p); p != "" {
			endpoints = append(endpoints, NormalizeGRPCEndpoint(p))
		}
	}
	return endpoints
}

// NormalizeGRPCEndpoint converts copied URL-style gRPC targets into host:port form.
// URL schemes and paths are ignored for gRPC targets. HTTPS in the target does not enable gRPC TLS.
func NormalizeGRPCEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return endpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return endpoint
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return endpoint
	}

	normalized := parsed.Host
	if normalized != endpoint {
		log.WithFields(logrus.Fields{
			"provided":  logutil.MaskCredentialsLogging(endpoint),
			"corrected": normalized,
		}).Warn("Normalizing gRPC endpoint to host:port. URL schemes and paths are ignored for gRPC targets, and https:// does not enable gRPC TLS")
	}
	return normalized
}
