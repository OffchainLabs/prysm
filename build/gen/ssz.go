package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// sszTarget is one fastssz/sszgen invocation: generate out in pkg for objs,
// excluding exclude. libInc are config-invariant Go source dirs passed to
// --include as-is; protoInc are proto packages whose generated *.pb.go must be
// staged (source dirs trip sszgen's loader) before being added to --include.
type sszTarget struct {
	pkg, out string
	libInc   []string
	protoInc []string
	objs     []string
	exclude  []string
}

// genSSZ regenerates the committed *.ssz.go with fastssz/sszgen (mainnet-only,
// untagged.
func genSSZ() error {
	const v1 = "proto/prysm/v1alpha1"

	libV1 := []string{"consensus-types/primitives", "math"}
	protoV1 := []string{"proto/engine/v1"}

	phase0 := []string{"AggregateAttestationAndProof", "Attestation", "AttestationData", "AttesterSlashing",
		"BeaconBlock", "BeaconBlockHeader", "BeaconState", "Checkpoint", "Deposit", "Deposit_Data", "DepositMessage",
		"ENRForkID", "Eth1Data", "Fork", "ForkData", "HistoricalBatch", "IndexedAttestation", "PowBlock", "ProposerSlashing",
		"SignedAggregateAttestationAndProof", "SignedBeaconBlock", "SignedBeaconBlockHeader", "SignedVoluntaryExit",
		"SigningData", "Status", "Status", "Validator", "ValidatorIdentity", "VoluntaryExit"}

	altair := []string{"BeaconBlockAltair", "BeaconBlockBodyAltair", "BeaconStateAltair", "ContributionAndProof",
		"LightClientHeaderAltair", "LightClientBootstrapAltair", "LightClientUpdateAltair",
		"LightClientFinalityUpdateAltair", "LightClientOptimisticUpdateAltair", "SignedBeaconBlockAltair",
		"SignedContributionAndProof", "SyncAggregate", "SyncAggregate", "SyncAggregatorSelectionData", "SyncCommittee",
		"SyncCommitteeContribution", "SyncCommitteeMessage"}

	bellatrix := []string{"BeaconBlockBellatrix", "BeaconBlockBodyBellatrix", "BeaconStateBellatrix",
		"BlindedBeaconBlockBellatrix", "BlindedBeaconBlockBodyBellatrix", "SignedBeaconBlockBellatrix",
		"SignedBlindedBeaconBlockBellatrix"}

	capella := []string{"BLSToExecutionChange", "BeaconBlockBodyCapella", "BeaconBlockCapella", "BeaconStateCapella",
		"BlindedBeaconBlockBodyCapella", "BlindedBeaconBlockCapella", "BuilderBidCapella", "HistoricalSummary",
		"LightClientHeaderCapella", "LightClientBootstrapCapella", "LightClientUpdateCapella",
		"LightClientFinalityUpdateCapella", "LightClientOptimisticUpdateCapella", "SignedBLSToExecutionChange",
		"SignedBeaconBlockCapella", "SignedBlindedBeaconBlockCapella", "Withdrawal", "SignedBuilderBidCapella"}

	deneb := []string{"BeaconBlockBodyDeneb", "BeaconBlockContentsDeneb", "BeaconBlockDeneb", "BeaconStateDeneb",
		"BlindedBeaconBlockBodyDeneb", "BlindedBeaconBlockDeneb", "BlobIdentifier", "BlobSidecar", "BlobSidecars",
		"BuilderBidDeneb", "LightClientHeaderDeneb", "LightClientBootstrapDeneb", "LightClientUpdateDeneb",
		"LightClientFinalityUpdateDeneb", "LightClientOptimisticUpdateDeneb", "SignedBeaconBlockContentsDeneb",
		"SignedBeaconBlockDeneb", "SignedBlindedBeaconBlockDeneb", "SignedBuilderBidDeneb"}

	electra := []string{"AggregateAttestationAndProofElectra", "AggregateAttestationAndProofSingle",
		"AttestationElectra", "AttesterSlashingElectra", "BeaconBlockElectra", "BeaconBlockBodyElectra",
		"BeaconBlockContentsElectra", "BeaconStateElectra", "BlindedBeaconBlockBodyElectra", "BlindedBeaconBlockElectra",
		"BuilderBidElectra", "Consolidation", "IndexedAttestationElectra", "LightClientHeaderElectra",
		"LightClientBootstrapElectra", "LightClientUpdateElectra", "LightClientFinalityUpdateElectra",
		"PendingDeposit", "PendingDeposits", "PendingConsolidation", "PendingPartialWithdrawal",
		"SignedAggregateAttestationAndProofElectra", "SignedAggregateAttestationAndProofSingle",
		"SignedBeaconBlockContentsElectra", "SignedBeaconBlockElectra", "SignedBlindedBeaconBlockElectra",
		"SignedConsolidation", "SingleAttestation", "SignedBuilderBidElectra"}

	fulu := []string{"BeaconBlockContentsFulu", "BeaconStateFulu", "BlindedBeaconBlockFulu", "DataColumnIdentifier",
		"DataColumnsByRootIdentifier", "DataColumnSidecar", "StatusV2", "SignedBeaconBlockContentsFulu",
		"SignedBeaconBlockFulu", "SignedBlindedBeaconBlockFulu"}

	gloas := []string{"BlindedExecutionPayloadEnvelope", "BuilderPendingPayment", "BuilderPendingWithdrawal",
		"DataColumnSidecarGloas", "ExecutionPayloadEnvelope", "PTCs", "ProposerPreferences", "SignedProposerPreferences",
		"PayloadAttestation", "PayloadAttestationData", "PayloadAttestationMessage", "ExecutionPayloadBid",
		"SignedExecutionPayloadBid", "SignedBlindedExecutionPayloadEnvelope", "SignedExecutionPayloadEnvelope",
		"SignedExecutionPayloadEnvelopeContents", "SignedWireBlindedExecutionPayloadEnvelope",
		"WireBlindedExecutionPayloadEnvelope", "BeaconBlockGloas", "BeaconBlockContentsGloas",
		"SignedBeaconBlockGloas", "BeaconStateGloas"}

	nonCore := []string{"BeaconBlocksByRangeRequest", "BlobSidecarsByRangeRequest", "DataColumnSidecarsByRangeRequest",
		"ExecutionPayloadEnvelopesByRangeRequest", "MetaDataV0", "MetaDataV1", "MetaDataV2",
		"SignedValidatorRegistrationV1", "ValidatorRegistrationV1", "BuilderBid", "SignedBuilderBid", "DepositSnapshot"}

	cat := func(s ...[]string) []string {
		var out []string
		for _, x := range s {
			out = append(out, x...)
		}

		return out
	}

	engine := []string{"ExecutionPayload", "ExecutionPayloadCapella", "ExecutionPayloadHeader",
		"ExecutionPayloadHeaderCapella", "ExecutionPayloadHeaderDeneb", "ExecutionPayloadDeneb", "ExecutionPayloadGloas",
		"ExecutionPayloadDenebAndBlobsBundle", "ExecutionPayloadDenebAndBlobsBundleV2", "BlindedBlobsBundle",
		"BlobsBundle", "BlobsBundleV2", "Withdrawal", "WithdrawalRequest", "DepositRequest", "ConsolidationRequest",
		"ExecutionRequests"}

	ethV1 := []string{"AggregateAttestationAndProof", "Attestation", "AttestationData", "AttesterSlashing", "BeaconBlock",
		"BeaconBlockHeader", "Checkpoint", "Deposit", "DepositData", "Eth1Data", "IndexedAttestation", "ProposerSlashing",
		"SignedAggregateAttestationAndProof", "SignedBeaconBlock", "SignedBeaconBlockHeader", "SignedVoluntaryExit",
		"SyncAggregate", "Validator", "VoluntaryExit"}

	targets := []sszTarget{
		{v1, "phase0.ssz.go", libV1, protoV1, phase0, nil},
		{v1, "altair.ssz.go", libV1, protoV1, altair, cat(phase0)},
		{v1, "bellatrix.ssz.go", libV1, protoV1, bellatrix, cat(phase0, altair)},
		{v1, "capella.ssz.go", libV1, protoV1, capella, cat(phase0, altair, bellatrix)},
		{v1, "deneb.ssz.go", libV1, protoV1, deneb, cat(phase0, altair, bellatrix, capella)},
		{v1, "electra.ssz.go", libV1, protoV1, electra, cat(phase0, altair, bellatrix, capella, deneb)},
		{v1, "fulu.ssz.go", libV1, protoV1, fulu, cat(phase0, altair, bellatrix, capella, deneb, electra)},
		{v1, "gloas.ssz.go", libV1, protoV1, gloas, cat(phase0, altair, bellatrix, capella, deneb, electra, fulu)},
		{v1, "non-core.ssz.go", libV1, protoV1, nonCore, nil},
		{"proto/engine/v1", "engine.ssz.go", []string{"consensus-types/primitives"}, nil, engine, nil},
		{"proto/eth/v1", "gateway.ssz.go", []string{"consensus-types/primitives"}, []string{"proto/engine/v1"}, ethV1, nil},
		{"proto/ssz_query", "response.ssz.go", nil, nil, []string{"SSZQueryProof", "SSZQueryResponse", "SSZQueryResponseWithProof"}, nil},
		{"proto/ssz_query/testing", "test_containers.ssz.go", nil, nil, []string{"FixedTestContainer", "FixedNestedContainer", "VariableTestContainer"}, nil},
	}

	for _, target := range targets {
		if err := genSSZTarget(target); err != nil {
			return fmt.Errorf("gen SSZ target: %w", err)
		}
	}

	return nil
}

func genSSZTarget(t sszTarget) error {
	fmt.Printf("generating %s/%s\n", t.pkg, t.out)

	// sszgen must see a package as ONLY its generated *.pb.go (hand-written
	// files import "reflect" under a different alias than the generated code,
	// which trips sszgen's loader). Stage --path and proto includes accordingly.
	stage := filepath.Join(t.pkg, ".sszgen_tmp")
	if err := stagePbgo(t.pkg, stage); err != nil {
		return fmt.Errorf("stagePbgo: %w", err)
	}

	defer unstage(stage)

	inc := slices.Clone(t.libInc)
	for _, p := range t.protoInc {
		istage := filepath.Join(p, ".sszinc_tmp")
		if err := stagePbgo(p, istage); err != nil {
			return err
		}

		defer unstage(istage)
		inc = append(inc, istage)
	}

	tmp, err := os.CreateTemp("", "sszgen-*.go")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	defer func() { _ = os.Remove(tmpName) }()

	args := []string{"--output=" + tmpName, "--path=" + stage, "--objs=" + strings.Join(t.objs, ",")}
	if len(inc) > 0 {
		args = append(args, "--include="+strings.Join(inc, ","))
	}

	if len(t.exclude) > 0 {
		args = append(args, "--exclude-objs="+strings.Join(t.exclude, ","))
	}

	if err := sh("go", append([]string{"tool", "sszgen"}, args...)...); err != nil {
		return fmt.Errorf("sh: %w", err)
	}

	// Strip the `// Hash: ...` line (as the old bazel copy-back did).
	data, err := os.ReadFile(tmpName) // #nosec G304 -- tmpName is our own os.CreateTemp output
	if err != nil {
		return fmt.Errorf("readFile: %w", err)
	}

	var b strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if strings.Contains(line, "// Hash: ") {
			continue
		}

		b.WriteString(line)
	}

	if err := os.WriteFile(filepath.Join(t.pkg, t.out), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	return nil
}

// stagePbgo copies only the generated *.pb.go (excluding committed minimal
// twins) from pkgDir into a fresh stageDir.
func stagePbgo(pkgDir, stageDir string) error {
	if err := os.MkdirAll(stageDir, 0o750); err != nil {
		return fmt.Errorf("mkdirAll: %w", err)
	}

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("readDir: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".minimal.pb.go") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(pkgDir, name)) // #nosec G304 -- pkgDir/name from a controlled ReadDir of repo proto packages
		if err != nil {
			return fmt.Errorf("readFile: %w", err)
		}

		if err := os.WriteFile(filepath.Join(stageDir, name), data, 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}
	}

	return nil
}

func unstage(dir string) { _ = os.RemoveAll(dir) }
