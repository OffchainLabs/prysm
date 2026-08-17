package iface

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
)

type NodeClient interface {
	SyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error)
	Genesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error)
	IsReady(ctx context.Context) bool
}
