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
		Help: "Count of payload attestation messages successfully inserted or aggregated into the pool, partitioned by the (payload_present, blob_data_available) claim.",
	},
	[]string{"payload_present", "blob_data_available"},
)

// observeInsertedPayloadAttestation records a successful insert (either a new
// pool entry or a newly-set aggregation bit on an existing one) labelled by
// the claim the attesting validator made about payload and blob availability.
func observeInsertedPayloadAttestation(data *ethpb.PayloadAttestationData) {
	payloadAttestationInsertsTotal.WithLabelValues(
		strconv.FormatBool(data.PayloadPresent),
		strconv.FormatBool(data.BlobDataAvailable),
	).Inc()
}
