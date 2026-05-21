package evaluators

import "google.golang.org/grpc"

func firstConn(conns []*grpc.ClientConn) *grpc.ClientConn {
	return conns[0]
}
