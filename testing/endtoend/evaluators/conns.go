package evaluators

import "google.golang.org/grpc"

func firstConn(conns []*grpc.ClientConn) *grpc.ClientConn {
	if conns == nil || len(conns) == 0 {
		return nil
	}
	return conns[0]
}
