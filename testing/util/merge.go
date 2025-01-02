package util

import (
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// NewBeaconBlockBellatrix creates a beacon block with minimum marshalable fields.
func NewBeaconBlockBellatrix() *ethpb.SignedBeaconBlockBellatrix {
	return HydrateSignedBeaconBlockBellatrix(&ethpb.SignedBeaconBlockBellatrix{})
}

// NewBlindedBeaconBlockBellatrix creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockBellatrix() *ethpb.SignedBlindedBeaconBlockBellatrix {
	return HydrateSignedBlindedBeaconBlockBellatrix(&ethpb.SignedBlindedBeaconBlockBellatrix{})
}

// NewBeaconBlockCapella creates a beacon block with minimum marshalable fields.
func NewBeaconBlockCapella() *ethpb.SignedBeaconBlockCapella {
	return HydrateSignedBeaconBlockCapella(&ethpb.SignedBeaconBlockCapella{})
}

// NewBlindedBeaconBlockCapella creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockCapella() *ethpb.SignedBlindedBeaconBlockCapella {
	return HydrateSignedBlindedBeaconBlockCapella(&ethpb.SignedBlindedBeaconBlockCapella{})
}

// NewBeaconBlockDeneb creates a beacon block with minimum marshalable fields.
func NewBeaconBlockDeneb() *ethpb.SignedBeaconBlockDeneb {
	return HydrateSignedBeaconBlockDeneb(&ethpb.SignedBeaconBlockDeneb{})
}

// NewBeaconBlockElectra creates a beacon block with minimum marshalable fields.
func NewBeaconBlockElectra() *ethpb.SignedBeaconBlockElectra {
	return HydrateSignedBeaconBlockElectra(&ethpb.SignedBeaconBlockElectra{})
}

// NewBeaconBlockContentsDeneb creates a beacon block with minimum marshalable fields.
func NewBeaconBlockContentsDeneb() *ethpb.SignedBeaconBlockContentsDeneb {
	return HydrateSignedBeaconBlockContentsDeneb(&ethpb.SignedBeaconBlockContentsDeneb{})
}

// NewBlindedBeaconBlockDeneb creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockDeneb() *ethpb.SignedBlindedBeaconBlockDeneb {
	return HydrateSignedBlindedBeaconBlockDeneb(&ethpb.SignedBlindedBeaconBlockDeneb{})
}

// NewBeaconBlockContentsElectra creates a beacon block with minimum marshalable fields.
func NewBeaconBlockContentsElectra() *ethpb.SignedBeaconBlockContentsElectra {
	return HydrateSignedBeaconBlockContentsElectra(&ethpb.SignedBeaconBlockContentsElectra{})
}

// NewBlindedBeaconBlockElectra creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockElectra() *ethpb.SignedBlindedBeaconBlockElectra {
	return HydrateSignedBlindedBeaconBlockElectra(&ethpb.SignedBlindedBeaconBlockElectra{})
}
