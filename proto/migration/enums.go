package migration

import (
	ethpb "github.com/OffchainLabs/prysm/v6/proto/eth/v1"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

func V1Alpha1ConnectionStateToV1(connState eth.ConnectionState) ethpb.ConnectionState {
	alphaString := connState.String()
	v1Value := ethpb.ConnectionState_value[alphaString]
	return ethpb.ConnectionState(v1Value)
}

func V1Alpha1PeerDirectionToV1(peerDirection eth.PeerDirection) (ethpb.PeerDirection, error) {
	alphaString := peerDirection.String()
	if alphaString == eth.PeerDirection_UNKNOWN.String() {
		return 0, errors.New("peer direction unknown")
	}
	v1Value := ethpb.PeerDirection_value[alphaString]
	return ethpb.PeerDirection(v1Value), nil
}
