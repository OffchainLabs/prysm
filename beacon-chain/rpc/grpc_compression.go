package rpc

import (
	// Installing the gzip encoding registers it as an available compressor.
	// gRPC will automatically negotiate and use gzip if the client supports it.
	_ "google.golang.org/grpc/encoding/gzip"
)
