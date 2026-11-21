// Package timing exports PQ gossip arrival and proving timings in the format of the Lighthouse pq-measurement reports.
package timing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/crypto/rand"
)

const Stride = 3

type Run struct {
	Output                string
	Observer              bool
	Network               string
	Version               string
	Backend               string
	GenesisTime           time.Time
	GenesisValidatorsRoot [32]byte
	Slot                  time.Duration
	AttestationDue        time.Duration
	CommitteeCollection   time.Duration
	AggregateDue          time.Duration
	InitialMerge          time.Duration
}

type session struct {
	rec     *recorder
	run     Run
	nextJob atomic.Uint64
}

var active atomic.Pointer[session]

func Open(run Run) (uint64, error) {
	seed := rand.NewGenerator().Uint64()
	start := time.Now().UnixNano()
	id := fmt.Sprintf("%d-%016x", start, seed)
	var threads any
	if n, err := strconv.ParseUint(os.Getenv("LEANVM_THREADS"), 10, 64); err == nil {
		threads = n
	}
	rec, err := openRecorder(run.Output, id, map[string]any{
		"schema_version":            1,
		"run_id":                    id,
		"started_unix_ns":           start,
		"observer":                  run.Observer,
		"network":                   run.Network,
		"genesis_time":              run.GenesisTime.Unix(),
		"genesis_slot":              0,
		"genesis_validators_root":   "0x" + hex.EncodeToString(run.GenesisValidatorsRoot[:]),
		"client":                    "prysm",
		"prysm_version":             run.Version,
		"prover_backend":            run.Backend,
		"prover_cpu_profile":        nil,
		"prover_threads":            nil,
		"prover_threads_configured": threads,
		"hardware":                  hardware(),
		"timing": map[string]any{
			"slot_ns":                 run.Slot.Nanoseconds(),
			"attestation_ns":          run.AttestationDue.Nanoseconds(),
			"committee_collection_ns": run.CommitteeCollection.Nanoseconds(),
			"aggregate_ns":            run.AggregateDue.Nanoseconds(),
			"initial_merge_ns":        run.InitialMerge.Nanoseconds(),
			"proposal_lag_slots":      2,
		},
		"sampling":      map[string]any{"seed": seed, "anchor_phase": seed % Stride, "stride": Stride},
		"receive_point": "gossip validator entry, after the pubsub seen cache and before Snappy and SSZ decode",
	})
	if err != nil {
		return 0, err
	}
	active.Store(&session{rec: rec, run: run})
	log.WithField("runDir", rec.dir).Info("Recording PQ timing locally")
	return seed, nil
}

func Close() {
	if s := active.Swap(nil); s != nil {
		s.rec.close()
	}
}

func Enabled() bool {
	s := active.Load()
	return s != nil && !s.rec.failed.Load()
}

func Status(kind string, fields map[string]any) {
	if s := active.Load(); s != nil {
		now := time.Now()
		s.rec.emit(classStatus, kind, now, now, fields)
	}
}

func ProofID(proof []byte) string {
	if len(proof) == 0 {
		return ""
	}
	sum := sha256.Sum256(proof)
	return "0x" + hex.EncodeToString(sum[:])
}

type receiptKey struct{}

type Receipt struct {
	at   time.Time
	kind string
	id   string
	slot uint64
}

func WithReceipt(ctx context.Context) (context.Context, *Receipt) {
	if !Enabled() {
		return ctx, nil
	}
	r := &Receipt{at: time.Now()}
	return context.WithValue(ctx, receiptKey{}, r), r
}

func ReceiptFrom(ctx context.Context) *Receipt {
	r, _ := ctx.Value(receiptKey{}).(*Receipt)
	return r
}

func (r *Receipt) Received(kind string, id []byte, slot uint64, committee, subnet any, signers uint64, bytes int, voteDataRoot [32]byte) {
	s := active.Load()
	if r == nil || s == nil {
		return
	}
	r.kind, r.id, r.slot = kind, "0x"+hex.EncodeToString(id), slot
	s.rec.emit(classReceipt, "message_received", r.at, r.at, record{
		"message_kind":   kind,
		"message_id":     r.id,
		"source":         "network",
		"reference_slot": slot,
		"slot":           slot,
		"committee":      committee,
		"subnet":         subnet,
		"message_bytes":  bytes,
		"byte_basis":     "decoded_ssz",
		"signer_count":   signers,
		"vote_data_root": "0x" + hex.EncodeToString(voteDataRoot[:]),
	})
}

func (r *Receipt) Validated(outcome string) {
	s := active.Load()
	if r == nil || s == nil || r.kind == "" {
		return
	}
	now := time.Now()
	s.rec.emit(classValidation, "validated", now, now, record{
		"message_kind":             r.kind,
		"message_id":               r.id,
		"source":                   "network",
		"reference_slot":           r.slot,
		"slot":                     r.slot,
		"outcome":                  outcome,
		"receipt_to_validation_ns": now.Sub(r.at).Nanoseconds(),
	})
}

type experimentKey struct{}

type Experiment struct {
	ID               uint64
	Scenario         string
	CommitteeCount   uint64
	ActiveValidators uint64
	Deadline         time.Time
}

func WithExperiment(ctx context.Context, e *Experiment) context.Context {
	return context.WithValue(ctx, experimentKey{}, e)
}

type roundKey struct{}

func WithRound(ctx context.Context, round int) context.Context {
	return context.WithValue(ctx, roundKey{}, round)
}

type Job struct {
	s          *session
	round      int
	id         uint64
	work       string
	slot       uint64
	reference  uint64
	experiment *Experiment
	enqueued   time.Time
}

func NewJob(ctx context.Context, work string, slot, reference uint64) *Job {
	s := active.Load()
	if s == nil || s.rec.failed.Load() {
		return nil
	}
	e, _ := ctx.Value(experimentKey{}).(*Experiment)
	if e != nil {
		// Every job of an experiment is placed on the timeline of its anchor slot.
		reference = e.ID
	}
	round, _ := ctx.Value(roundKey{}).(int)
	round = max(round, 1)
	if work == "initial" && round > 1 {
		work = "improvement"
	}
	return &Job{s: s, round: round, id: s.nextJob.Add(1), work: work, slot: slot, reference: reference, experiment: e, enqueued: time.Now()}
}

func (j *Job) Production() bool {
	return j != nil && j.experiment == nil
}

func (j *Job) deadline() time.Time {
	if j.experiment != nil {
		return j.experiment.Deadline
	}
	if j.work != "committee" {
		return time.Time{}
	}
	// #nosec G115 -- slot durations are far below the int64 range
	return j.s.run.GenesisTime.Add(time.Duration(j.slot) * j.s.run.Slot).Add(j.s.run.AggregateDue)
}

func (j *Job) Emit(kind, outcome string, fields map[string]any) {
	if j == nil {
		return
	}
	if fields == nil {
		fields = record{}
	}
	fields["outcome"] = outcome
	fields["job_id"] = j.id
	fields["work"] = j.work
	fields["slot"] = j.slot
	fields["reference_slot"] = j.reference
	fields["round"] = j.round
	fields["previous_recursive_depth"] = 0
	fields["enqueued_wall_ns"] = j.enqueued.UnixNano()
	if e := j.experiment; e != nil {
		fields["origin"] = "measurement"
		fields["experiment_id"] = e.ID
		fields["sequence_id"] = fmt.Sprintf("measurement:%d", e.ID)
		fields["scenario"] = e.Scenario
		fields["committee_count"] = e.CommitteeCount
		fields["active_validators"] = e.ActiveValidators
	} else {
		fields["origin"] = "production"
		fields["experiment_id"] = nil
		fields["sequence_id"] = fmt.Sprintf("production:%d", j.slot)
		fields["scenario"] = "production"
	}
	class := classWork
	if kind == "proposal_snapshot" {
		class = classStatus
	}
	now := time.Now()
	j.s.rec.emit(class, kind, now, now, fields)
}

func (j *Job) Run(fields map[string]any, backend func() ([]byte, error)) ([]byte, error) {
	if j == nil {
		return backend()
	}
	started := record{"queue_wait_ns": time.Since(j.enqueued).Nanoseconds()}
	for k, v := range fields {
		started[k] = v
	}
	j.Emit("proof_started", "started", started)
	start := time.Now()
	proof, err := backend()
	elapsed := time.Since(start)
	outcome := "success"
	if err != nil {
		outcome = "failed"
	}
	deadline := j.deadline()
	finished := record{
		"backend_duration_ns": elapsed.Nanoseconds(),
		"late":                !deadline.IsZero() && !time.Now().Before(deadline),
		"proof_bytes":         len(proof),
		"proof_id":            ProofID(proof),
	}
	for k, v := range fields {
		finished[k] = v
	}
	j.Emit("proof_finished", outcome, finished)
	return proof, err
}
