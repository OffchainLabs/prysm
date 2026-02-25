package beacon

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/cmd"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestServer_AttestationPool_Pagination_ExceedsMaxPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &ethpb.AttestationPoolRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_OutOfRange(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := []ethpb.Att{
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: bytesutil.PadTo([]byte{1}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{1}, fieldparams.BLSSignatureLength),
		},
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            2,
				BeaconBlockRoot: bytesutil.PadTo([]byte{2}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{2}, fieldparams.BLSSignatureLength),
		},
		&ethpb.Attestation{
			Data: &ethpb.AttestationData{
				Slot:            3,
				BeaconBlockRoot: bytesutil.PadTo([]byte{3}, 32),
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signature:       bytesutil.PadTo([]byte{3}, fieldparams.BLSSignatureLength),
		},
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{
		PageToken: strconv.Itoa(1),
		PageSize:  100,
	}
	wanted := fmt.Sprintf("page start %d >= list %d", req.PageSize, len(atts))
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_DefaultPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := make([]ethpb.Att, params.BeaconConfig().DefaultPageSize+1)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{}
	res, err := bs.AttestationPool(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DefaultPageSize, len(res.Attestations), "Unexpected number of attestations")
	assert.Equal(t, params.BeaconConfig().DefaultPageSize+1, int(res.TotalSize), "Unexpected total size")
}

func TestServer_AttestationPool_Pagination_CustomPageSize(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	numAtts := 100
	atts := make([]ethpb.Att, numAtts)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))
	tests := []struct {
		req *ethpb.AttestationPoolRequest
		res *ethpb.AttestationPoolResponse
	}{
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "2",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(3),
				PageSize:  30,
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &ethpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(0),
				PageSize:  int32(numAtts),
			},
			res: &ethpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
	}
	for _, tt := range tests {
		res, err := bs.AttestationPool(ctx, tt.req)
		require.NoError(t, err)
		assert.Equal(t, tt.res.TotalSize, res.TotalSize, "Unexpected total size")
		assert.Equal(t, tt.res.NextPageToken, res.NextPageToken, "Unexpected next page token")
	}
}

func TestServer_AttestationPoolElectra(t *testing.T) {
	ctx := t.Context()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := make([]ethpb.Att, params.BeaconConfig().DefaultPageSize+1)
	for i := range atts {
		att := util.NewAttestationElectra()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &ethpb.AttestationPoolRequest{}
	res, err := bs.AttestationPoolElectra(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DefaultPageSize, len(res.Attestations), "Unexpected number of attestations")
	assert.Equal(t, params.BeaconConfig().DefaultPageSize+1, int(res.TotalSize), "Unexpected total size")
}
