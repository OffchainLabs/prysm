package grpc

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
//
// CustomErrorMetadataKey is the name of the metadata key storing additional error information.
// Metadata value is expected to be a byte-encoded JSON object.
const CustomErrorMetadataKey = "Custom-Error"

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
//
// HttpCodeMetadataKey is the key to use when setting custom HTTP status codes in gRPC metadata.
const HttpCodeMetadataKey = "X-Http-Code"

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
//
// MetadataPrefix is the prefix for grpc headers on metadata
const MetadataPrefix = "Grpc-Metadata"

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
//
// WithPrefix creates a new string with grpc metadata prefix
func WithPrefix(value string) string {
	return MetadataPrefix + "-" + value
}
