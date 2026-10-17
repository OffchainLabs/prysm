package payloadattestation

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var payloadAttestationPoolSize = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "payload_attestation_pool_size",
		Help: "The number of unique payload attestation entries currently in the pool.",
	},
)

var payloadAttestationInsertsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payload_attestation_inserts_total",
		Help: "Count of PTC seat claims successfully inserted or aggregated into the pool, partitioned by the (payload_present, blob_data_available) claim. A validator sampled into multiple PTC seats counts once per seat.",
	},
	[]string{"payload_present", "blob_data_available"},
)

// observeInsertedPayloadAttestation records a successful insert (either a new
// pool entry or newly-set aggregation bits on an existing one) labelled by
// the claim the attesting validator made about payload and blob availability.
// seats is the number of PTC positions the validator occupies, so the counter
// is seat-weighted to match how PTC votes are tallied, rather than counting
// validator messages.
func observeInsertedPayloadAttestation(data *ethpb.PayloadAttestationData, seats int) {
	payloadAttestationInsertsTotal.WithLabelValues(
		strconv.FormatBool(data.PayloadPresent),
		strconv.FormatBool(data.BlobDataAvailable),
	).Add(float64(seats))
}
