package beacon_api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
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

		gotRoot, ok := payloadAttestationBeaconBlockRoot(body, hdr)
		return ok && gotRoot == want.Root
	}

	return append(opts, rest.WithRace(), rest.WithSSZAccept(accept))
}

func payloadAttestationBeaconBlockRoot(body []byte, hdr http.Header) ([32]byte, bool) {
	if strings.Contains(hdr.Get("Content-Type"), api.OctetStreamMediaType) {
		d := &ethpb.PayloadAttestationData{}
		if err := d.UnmarshalSSZ(body); err != nil {
			return [32]byte{}, false
		}

		return bytesutil.ToBytes32(d.BeaconBlockRoot), true
	}

	return rootExtractor("beacon_block_root")(json.RawMessage(body))
}
