package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// LogRequests logs the gRPC backend as well as request duration when the log level is set to debug
// or higher.
func LogRequests(
	ctx context.Context,
	method string, req,
	reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	// Shortcut when debug logging is not enabled.
	if logrus.GetLevel() < logrus.DebugLevel {
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	var header metadata.MD
	opts = append(
		opts,
		grpc.Header(&header),
	)
	start := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)
	log.WithField("backend", header["x-backend"]).
		WithField("method", method).WithField("duration", time.Since(start)).
		Debug("gRPC request finished.")
	return err
}

// LogStream prints the method at DEBUG level at the start of the stream.
func LogStream(
	ctx context.Context,
	sd *grpc.StreamDesc,
	conn *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	// Shortcut when debug logging is not enabled.
	if logrus.GetLevel() < logrus.DebugLevel {
		return streamer(ctx, sd, conn, method, opts...)
	}

	var header metadata.MD
	opts = append(
		opts,
		grpc.Header(&header),
	)
	strm, err := streamer(ctx, sd, conn, method, opts...)
	log.WithField("backend", header["x-backend"]).
		WithField("method", method).
		Debug("gRPC stream started.")
	return strm, err
}

// AppendHeaders parses the provided GRPC headers
// and attaches them to the provided context.
func AppendHeaders(parent context.Context, headers []string) context.Context {
	// Batch key-value pairs to make a single AppendToOutgoingContext call,
	// avoiding a new context allocation per header.
	pairs := make([]string, 0, len(headers)*2)
	for _, h := range headers {
		if h != "" {
			keyValue := strings.Split(h, "=")
			if len(keyValue) < 2 {
				log.Warnf("Incorrect gRPC header flag format. Skipping %v", keyValue[0])
				continue
			}
			pairs = append(pairs, keyValue[0], strings.Join(keyValue[1:], "="))
		}
	}
	if len(pairs) > 0 {
		parent = metadata.AppendToOutgoingContext(parent, pairs...) // nolint:fatcontext
	}
	return parent
}
