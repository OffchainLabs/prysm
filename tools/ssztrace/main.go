// Command ssztrace emits a merkleization trace (flat g -> root map, or nested
// YAML) for a gloas SSZ fixture, using prysm's methodical-ssz-generated
// HashTreeRootWith and the tracer in github.com/OffchainLabs/methodical-ssz/ssz.
//
// It is the Go side of cross-library bisection: diff its flat map against the
// Python/remerkleable tracer's (ssz-tracer/trace_flat.py) and take the highest
// generalized index whose root differs to localize a hash_tree_root
// disagreement to a single node.
//
// Usage:
//
//	go run ./tools/ssztrace [-yaml] [-o out.yaml] <TypeName> <case_dir>
//
// <TypeName> is a spectest folder name (e.g. ExecutionPayloadBid,
// BeaconBlockBody, Attestation) — the same names used by the gloas ssz_static
// unmarshaller. <case_dir> holds serialized.ssz_snappy and roots.yaml. The
// trace self-verifies against the value's generated HashTreeRoot, and (when
// roots.yaml is present) against the fixture root, before emitting.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sszlib "github.com/OffchainLabs/methodical-ssz/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/snappy"
)

// traceable is satisfied by the generated ssz types: deserialize a fixture and
// drive the methodical-ssz Hasher.
type traceable interface {
	sszlib.HashRoot
	UnmarshalSSZ([]byte) error
}

// newObject maps a spectest folder name to the gloas Go type, mirroring
// testing/spectest/shared/gloas/ssz_static/ssz_static.go. Only consensus types
// with a generated methodical HTR are included (native-state-only and
// p2p/skipped types are omitted).
func newObject(name string) (traceable, error) {
	switch name {
	// Gloas-specific
	case "ExecutionPayloadBid":
		return &ethpb.ExecutionPayloadBid{}, nil
	case "SignedExecutionPayloadBid":
		return &ethpb.SignedExecutionPayloadBid{}, nil
	case "PayloadAttestationData":
		return &ethpb.PayloadAttestationData{}, nil
	case "PayloadAttestation":
		return &ethpb.PayloadAttestation{}, nil
	case "PayloadAttestationMessage":
		return &ethpb.PayloadAttestationMessage{}, nil
	case "BeaconBlock":
		return &ethpb.BeaconBlockGloas{}, nil
	case "BeaconBlockBody":
		return &ethpb.BeaconBlockBodyGloas{}, nil
	case "BeaconState":
		return &ethpb.BeaconStateGloas{}, nil
	case "ExecutionPayloadEnvelope":
		return &ethpb.ExecutionPayloadEnvelope{}, nil
	case "SignedExecutionPayloadEnvelope":
		return &ethpb.SignedExecutionPayloadEnvelope{}, nil
	case "DataColumnSidecar":
		return &ethpb.DataColumnSidecarGloas{}, nil
	case "Builder":
		return &ethpb.Builder{}, nil
	case "BuilderPendingPayment":
		return &ethpb.BuilderPendingPayment{}, nil
	case "BuilderPendingWithdrawal":
		return &ethpb.BuilderPendingWithdrawal{}, nil

	// Standard types that also appear in gloas fixtures
	case "ExecutionPayload":
		return &enginev1.ExecutionPayloadGloas{}, nil
	case "ExecutionPayloadHeader":
		return &enginev1.ExecutionPayloadHeaderDeneb{}, nil
	case "ExecutionRequests":
		return &enginev1.ExecutionRequestsGloas{}, nil
	case "Attestation":
		return &ethpb.AttestationGloas{}, nil
	case "AttestationData":
		return &ethpb.AttestationData{}, nil
	case "AttesterSlashing":
		return &ethpb.AttesterSlashingGloas{}, nil
	case "AggregateAndProof":
		return &ethpb.AggregateAttestationAndProofGloas{}, nil
	case "SignedAggregateAndProof":
		return &ethpb.SignedAggregateAttestationAndProofGloas{}, nil
	case "IndexedAttestation":
		return &ethpb.IndexedAttestationGloas{}, nil
	case "SignedBeaconBlock":
		return &ethpb.SignedBeaconBlockGloas{}, nil
	case "Validator":
		return &ethpb.Validator{}, nil
	case "PendingConsolidation":
		return &ethpb.PendingConsolidation{}, nil
	case "PendingDeposit":
		return &ethpb.PendingDeposit{}, nil
	case "PendingPartialWithdrawal":
		return &ethpb.PendingPartialWithdrawal{}, nil
	default:
		return nil, fmt.Errorf("type %q not in ssztrace registry (add it to newObject)", name)
	}
}

func fixtureRoot(caseDir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(caseDir, "roots.yaml"))
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(string(b), "root:", 2)
	if len(parts) != 2 {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(parts[1]), "'\""), true
}

func main() {
	yaml := flag.Bool("yaml", false, "emit the nested YAML tree instead of the flat g -> root map")
	out := flag.String("o", "", "write to this file instead of stdout")
	flag.Parse()
	// Accept flags on either side of the positional args (stdlib flag stops at
	// the first non-flag, so re-parse anything trailing the two positionals).
	rest := flag.Args()
	if len(rest) < 2 {
		fmt.Fprintln(os.Stderr, "usage: ssztrace [-yaml] [-o out] <TypeName> <case_dir>")
		os.Exit(2)
	}
	typeName, caseDir := rest[0], rest[1]
	if err := flag.CommandLine.Parse(rest[2:]); err != nil {
		os.Exit(2)
	}

	obj, err := newObject(typeName)
	if err != nil {
		fatal(err)
	}

	compressed, err := os.ReadFile(filepath.Join(caseDir, "serialized.ssz_snappy"))
	if err != nil {
		fatal(fmt.Errorf("read fixture: %w", err))
	}
	serialized, err := snappy.Decode(nil, compressed)
	if err != nil {
		fatal(fmt.Errorf("snappy decode: %w", err))
	}
	if err := obj.UnmarshalSSZ(serialized); err != nil {
		fatal(fmt.Errorf("unmarshal %s: %w", typeName, err))
	}

	res, err := sszlib.TraceValue(obj, typeName)
	if err != nil {
		fatal(err)
	}

	// The trace already self-verified internally (reconstruct == generated HTR).
	// Cross-check against the fixture root as a version-alignment signal only:
	// a mismatch means the generated Go type and the fixture were built from
	// different spec versions, which is a warning (still emit — the trace is
	// faithful to whatever the Go type computes), not a tracer failure.
	if want, ok := fixtureRoot(caseDir); ok {
		got := fmt.Sprintf("0x%x", res.Root)
		if got != want {
			fmt.Fprintf(os.Stderr, "WARNING: trace root %s != fixture %s (spec-version skew; trace still self-verified)\n", got, want)
		} else {
			fmt.Fprintf(os.Stderr, "self-verified; root %s matches fixture\n", got)
		}
	} else {
		fmt.Fprintf(os.Stderr, "self-verified; root 0x%x (no roots.yaml to cross-check)\n", res.Root)
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fatal(err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				fatal(err)
			}
		}()
		w = f
	}

	if *yaml {
		err = res.WriteYAML(w)
	} else {
		err = res.WriteFlat(w)
	}
	if err != nil {
		fatal(err)
	}
	if *out != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ssztrace:", err)
	os.Exit(1)
}
