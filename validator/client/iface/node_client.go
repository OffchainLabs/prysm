package iface

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/health"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type NodeClient interface {
	SyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error)
	Genesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error)
	Version(ctx context.Context, in *empty.Empty) (*ethpb.Version, error)
	Peers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error)
	HealthTracker() health.Tracker
}
