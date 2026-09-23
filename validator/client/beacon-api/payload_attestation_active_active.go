package beacon_api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

// Prefer a REST node whose payload attestation data matches the announced head.
func payloadAttestationFreshnessOptions(ctx context.Context) []rest.QueryOption {
	var opts []rest.QueryOption
	if interval, onRetry, ok := iface.PayloadAttestationRetryFromContext(ctx); ok {
		if deadline, bounded := ctx.Deadline(); bounded {
			opts = append(opts, rest.WithDeadline(deadline), rest.WithIndependentRepoll(interval, onRetry),
				rest.WithSSZResponseValidator(func(body []byte, header http.Header) error {
					_, err := decodePayloadAttestationData(body, header)
					return err
				}))
		}
	}
	hint, ok := freshnessHint(ctx)
	if !ok {
		return opts
	}

	accept := func(body []byte, hdr http.Header) bool {
		want, known := hint.Head()
		if !known {
			// With no known head to match, accept the first successful response.
			return true
		}

		gotRoot, payloadPresent, ok := payloadAttestationHead(body, hdr)
		if !ok || gotRoot != want.Root {
			return false
		}

		// A node announced the payload for the head: prefer a node that also saw it,
		// rather than a lagging one that would vote the payload absent.
		return want.PayloadStatus != api.PayloadStatusFull || payloadPresent
	}

	return append(opts, rest.WithRace(), rest.WithSSZAccept(accept))
}

// payloadAttestationHead extracts the beacon_block_root and payload_present
// fields from a payload attestation data response, which GetSSZ may return as SSZ
// or JSON.
func payloadAttestationHead(body []byte, hdr http.Header) ([32]byte, bool, bool) {
	if strings.Contains(hdr.Get("Content-Type"), api.OctetStreamMediaType) {
		d := &ethpb.PayloadAttestationData{}
		if err := d.UnmarshalSSZ(body); err != nil {
			return [32]byte{}, false, false
		}

		return bytesutil.ToBytes32(d.BeaconBlockRoot), d.PayloadPresent, true
	}

	var resp structs.GetPayloadAttestationDataResponse
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data == nil {
		return [32]byte{}, false, false
	}

	root, err := bytesutil.DecodeHex32(resp.Data.BeaconBlockRoot)
	if err != nil {
		return [32]byte{}, false, false
	}

	return root, resp.Data.PayloadPresent, true
}
