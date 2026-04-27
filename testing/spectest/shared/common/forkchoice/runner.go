package forkchoice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/snappy"
)

// These are proposer boost spec tests that assume the clock starts 3 seconds into the slot.
// Example: Tick is 51, which corresponds to 3 seconds into slot 4.
var proposerBoostTests3s = []string{
	"proposer_boost_is_first_block",
	"proposer_boost",
}

func init() {
	transition.SkipSlotCache.Disable()
}

// Run executes "forkchoice"  and "sync" test.
func Run(t *testing.T, config string, fork int) {
	runTest(t, config, fork, "fork_choice")
	// Gloas spec tarballs do not ship a "sync" test directory.
	if fork >= version.Bellatrix && fork < version.Gloas {
		runTest(t, config, fork, "sync")
	}
}

// RunCompliance executes the consensus-specs fork-choice compliance-runner suites
// emitted by tests/generators/compliance_runners/fork_choice. The on-disk layout
// is identical to the standard fork_choice tests; only the basePath differs.
//
// Data is resolved in order:
//  1. $COMPLIANCE_FC_DIR — absolute path to an unpacked compliance-tests tarball
//     (rooted at the directory containing `tests/`). Used for local development.
//  2. Bazel runfiles — the usual path when running under `bazel test`.
//
// If no compliance test data is present for the requested preset/fork, the
// function returns silently (pre-Fulu forks do not ship compliance tests).
func RunCompliance(t *testing.T, config string, fork int) {
	if root := os.Getenv("COMPLIANCE_FC_DIR"); root != "" {
		runComplianceLocal(t, root, config, fork)
		return
	}
	runTestIfPresent(t, config, fork, "fork_choice_compliance")
}

// RunComplianceSuite executes a single compliance-runner suite (e.g.
// "block_tree_test"). Splitting per suite lets bazel test sharding parallelize
// the work across processes — each shard runs its own top-level Test func, so
// global state (helpers caches, params overrides) is not shared between suites.
//
// Data resolution mirrors RunCompliance.
func RunComplianceSuite(t *testing.T, config string, fork int, suite string) {
	if root := os.Getenv("COMPLIANCE_FC_DIR"); root != "" {
		runComplianceLocalSuite(t, root, config, fork, suite)
		return
	}
	runSuiteTestIfPresent(t, config, fork, "fork_choice_compliance", suite)
}

// runTestIfPresent behaves like runTest, but returns silently when the
// requested basePath is not present in the runfiles tree. This keeps
// compliance-test entry points no-ops for presets/forks that do not ship
// compliance-runner data (for example pre-Fulu forks).
func runTestIfPresent(t *testing.T, config string, fork int, basePath string) {
	testsFolderPath := path.Join("tests", config, version.String(fork), basePath)
	if _, err := bazel.Runfile(testsFolderPath); err != nil {
		t.Logf("No compliance test data at %s; skipping", testsFolderPath)
		return
	}
	runTest(t, config, fork, basePath)
}

// runSuiteTestIfPresent is the per-suite counterpart of runTestIfPresent: it
// runs only the named suite and returns silently when the suite is absent
// from the runfiles tree.
func runSuiteTestIfPresent(t *testing.T, config string, fork int, basePath, suite string) {
	suitePath := path.Join("tests", config, version.String(fork), basePath, suite)
	if _, err := bazel.Runfile(suitePath); err != nil {
		t.Logf("No compliance test data at %s; skipping", suitePath)
		return
	}
	runSuiteTest(t, config, fork, basePath, suite)
}

// runSuiteTest dispatches every case under <basePath>/<suite>/pyspec_tests via
// utils.TestFolders, the bazel-runfiles-aware loader. It mirrors runTest's
// inner loop but skips the outer suite iteration so each suite is its own
// top-level Test func — required for bazel shard distribution.
func runSuiteTest(t *testing.T, config string, fork int, basePath, suite string) {
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, fork)

	casesFolderPath := path.Join(basePath, suite, "pyspec_tests")
	cases, casesPath := utils.TestFolders(t, config, version.String(fork), casesFolderPath)
	if len(cases) == 0 {
		t.Fatalf("No test cases found for %s/%s/%s", config, version.String(fork), casesFolderPath)
	}

	skipTests := map[string]bool{
		// Skipping because of #4807 backporting issues
		"voting_source_beyond_two_epoch":         true,
		"justified_update_always_if_better":      true,
		"justified_update_not_realized_finality": true,
	}
	for _, c := range cases {
		if skipTests[c.Name()] {
			t.Logf("Skipping test %s due to known issues", c.Name())
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			runTestCase(t, fork, c, casesPath)
		})
	}
}

// runComplianceLocalSuite is the per-suite counterpart of runComplianceLocal,
// reading cases directly from the filesystem under $COMPLIANCE_FC_DIR.
func runComplianceLocalSuite(t *testing.T, rootDir, config string, fork int, suite string) {
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, fork)

	if !filepath.IsAbs(rootDir) {
		t.Fatalf("COMPLIANCE_FC_DIR=%q must be an absolute path (Bazel resolves relative paths from the sandbox cwd)", rootDir)
	}
	casesPath := filepath.Join(
		rootDir, "tests", config, version.String(fork), "fork_choice_compliance", suite, "pyspec_tests",
	)
	cases, err := os.ReadDir(casesPath)
	if err != nil {
		t.Fatalf("COMPLIANCE_FC_DIR=%s: no cases at %s: %v", rootDir, casesPath, err)
	}
	ran := 0
	for _, cs := range cases {
		if !cs.IsDir() {
			continue
		}
		t.Run(cs.Name(), func(t *testing.T) {
			runTestCase(t, fork, cs, casesPath)
		})
		ran++
	}
	if ran == 0 {
		t.Fatalf("COMPLIANCE_FC_DIR=%s is readable but contained zero cases under %s", rootDir, casesPath)
	}
}

// runComplianceLocal walks a locally-unpacked compliance-tests tarball on the
// filesystem and dispatches each case through runTestCase. It bypasses Bazel
// runfiles entirely; util.BazelFileBytes accepts absolute paths unchanged.
func runComplianceLocal(t *testing.T, rootDir, config string, fork int) {
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, fork)

	const basePath = "fork_choice_compliance"
	forkStr := version.String(fork)
	if !filepath.IsAbs(rootDir) {
		t.Fatalf("COMPLIANCE_FC_DIR=%q must be an absolute path (Bazel resolves relative paths from the sandbox cwd)", rootDir)
	}
	absBase := filepath.Join(rootDir, "tests", config, forkStr, basePath)
	suites, err := os.ReadDir(absBase)
	if err != nil {
		// The env var was set explicitly by the user, so a missing directory is
		// almost certainly a config mistake (wrong path, wrong preset for this
		// tarball). Fail loudly rather than passing trivially with zero subtests.
		t.Fatalf("COMPLIANCE_FC_DIR is set but no compliance data at %s: %v", absBase, err)
	}
	ran := 0
	for _, suite := range suites {
		if !suite.IsDir() {
			continue
		}
		casesPath := filepath.Join(absBase, suite.Name(), "pyspec_tests")
		cases, err := os.ReadDir(casesPath)
		if err != nil {
			t.Logf("No pyspec_tests under %s: %v", casesPath, err)
			continue
		}
		for _, cs := range cases {
			if !cs.IsDir() {
				continue
			}
			t.Run(path.Join(suite.Name(), cs.Name()), func(t *testing.T) {
				runTestCase(t, fork, cs, casesPath)
			})
			ran++
		}
	}
	if ran == 0 {
		t.Fatalf("COMPLIANCE_FC_DIR=%s is readable but contained zero cases under %s", rootDir, absBase)
	}
}

func runTest(t *testing.T, config string, fork int, basePath string) { // nolint:gocognit
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, fork)
	testFolders, _ := utils.TestFolders(t, config, version.String(fork), basePath)
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, version.String(fork), basePath)
	}

	for _, folder := range testFolders {
		folderPath := path.Join(basePath, folder.Name(), "pyspec_tests")
		testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(fork), folderPath)
		if len(testFolders) == 0 {
			t.Fatalf("No test folders found for %s/%s/%s", config, version.String(fork), folderPath)
		}
		var skipTests = map[string]bool{
			// Skipping because of #4807 backporting issues
			"voting_source_beyond_two_epoch":         true,
			"justified_update_always_if_better":      true,
			"justified_update_not_realized_finality": true,
		}
		for _, folder := range testFolders {
			if skipTests[folder.Name()] {
				t.Logf("Skipping test %s due to known issues", folder.Name())
				continue
			}
			t.Run(folder.Name(), func(t *testing.T) {
				runTestCase(t, fork, folder, testsFolderPath)
			})
		}
	}
}

func runTestCase(t *testing.T, fork int, folder os.DirEntry, testsFolderPath string) { // nolint:gocognit
	helpers.ClearCache()
	preStepsFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "steps.yaml")
	require.NoError(t, err)
	var steps []Step
	require.NoError(t, utils.UnmarshalYaml(preStepsFile, &steps))

	preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "anchor_state.ssz_snappy")
	require.NoError(t, err)
	preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
	require.NoError(t, err)

	blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "anchor_block.ssz_snappy")
	require.NoError(t, err)
	blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
	require.NoError(t, err)

	var beaconState state.BeaconState
	var beaconBlock interfaces.ReadOnlySignedBeaconBlock
	switch fork {
	case version.Phase0:
		beaconState = unmarshalPhase0State(t, preBeaconStateSSZ)
		beaconBlock = unmarshalPhase0Block(t, blockSSZ)
	case version.Altair:
		beaconState = unmarshalAltairState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalAltairBlock(t, blockSSZ)
	case version.Bellatrix:
		beaconState = unmarshalBellatrixState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalBellatrixBlock(t, blockSSZ)
	case version.Capella:
		beaconState = unmarshalCapellaState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalCapellaBlock(t, blockSSZ)
	case version.Deneb:
		beaconState = unmarshalDenebState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalDenebBlock(t, blockSSZ)
	case version.Electra:
		beaconState = unmarshalElectraState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalElectraBlock(t, blockSSZ)
	case version.Fulu:
		beaconState = unmarshalFuluState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalFuluBlock(t, blockSSZ)
	case version.Gloas:
		beaconState = unmarshalGloasState(t, preBeaconStateSSZ)
		beaconBlock = unmarshalGloasBlock(t, blockSSZ)
	default:
		t.Fatalf("unknown fork version: %v", fork)
	}

	builder := NewBuilder(t, beaconState, beaconBlock)

	for _, step := range steps {
		if step.Tick != nil {
			tick := int64(*step.Tick)
			// If the test is for proposer boost starting 3 seconds into the slot and the tick aligns with this,
			// we provide an additional second buffer. Instead of starting 3 seconds into the slot, we start 2 seconds in to avoid missing the proposer boost.
			// A 1-second buffer has proven insufficient during parallel spec test runs, as the likelihood of missing the proposer boost increases significantly,
			// often extending to 4 seconds. Starting 2 seconds into the slot ensures close to a 100% pass rate.
			if slices.Contains(proposerBoostTests3s, folder.Name()) {
				deadline := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
				if uint64(tick)%params.BeaconConfig().SecondsPerSlot == deadline-1 {
					tick--
				}
			}
			builder.Tick(t, tick)
		}
		var beaconBlock interfaces.ReadOnlySignedBeaconBlock
		if step.Block != nil {
			blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.Block, ".ssz_snappy"))
			require.NoError(t, err)
			blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
			require.NoError(t, err)
			switch fork {
			case version.Phase0:
				beaconBlock = unmarshalSignedPhase0Block(t, blockSSZ)
			case version.Altair:
				beaconBlock = unmarshalSignedAltairBlock(t, blockSSZ)
			case version.Bellatrix:
				beaconBlock = unmarshalSignedBellatrixBlock(t, blockSSZ)
			case version.Capella:
				beaconBlock = unmarshalSignedCapellaBlock(t, blockSSZ)
			case version.Deneb:
				beaconBlock = unmarshalSignedDenebBlock(t, blockSSZ)
			case version.Electra:
				beaconBlock = unmarshalSignedElectraBlock(t, blockSSZ)
			case version.Fulu:
				beaconBlock = unmarshalSignedFuluBlock(t, blockSSZ)
			case version.Gloas:
				beaconBlock = unmarshalSignedGloasBlock(t, blockSSZ)
			default:
				t.Fatalf("unknown fork version: %v", fork)
			}
		}
		runBlobStep(t, step, beaconBlock, fork, folder, testsFolderPath, builder)
		if len(step.DataColumns) > 0 {
			runDataColumnStep(t, step, beaconBlock, fork, folder, testsFolderPath, builder)
		}
		if beaconBlock != nil {
			if step.Valid != nil && !*step.Valid {
				builder.InvalidBlock(t, beaconBlock)
			} else {
				builder.ValidBlock(t, beaconBlock)
			}
		}
		if step.AttesterSlashing != nil {
			slashingFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.AttesterSlashing, ".ssz_snappy"))
			require.NoError(t, err)
			slashingSSZ, err := snappy.Decode(nil /* dst */, slashingFile)
			require.NoError(t, err)
			slashing := &ethpb.AttesterSlashing{}
			require.NoError(t, slashing.UnmarshalSSZ(slashingSSZ), "Failed to unmarshal")
			builder.AttesterSlashing(slashing)
		}
		if step.Attestation != nil {
			attFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.Attestation, ".ssz_snappy"))
			require.NoError(t, err)
			attSSZ, err := snappy.Decode(nil /* dst */, attFile)
			require.NoError(t, err)
			var att ethpb.Att
			if fork < version.Electra {
				att = &ethpb.Attestation{}
			} else {
				att = &ethpb.AttestationElectra{}
			}
			require.NoError(t, att.UnmarshalSSZ(attSSZ), "Failed to unmarshal")
			builder.Attestation(t, att)
		}
		if step.PayloadStatus != nil {
			require.NoError(t, builder.SetPayloadStatus(step.PayloadStatus))
		}
		if step.PowBlock != nil {
			powBlockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.PowBlock, ".ssz_snappy"))
			require.NoError(t, err)
			p, err := snappy.Decode(nil /* dst */, powBlockFile)
			require.NoError(t, err)
			pb := &ethpb.PowBlock{}
			require.NoError(t, pb.UnmarshalSSZ(p), "Failed to unmarshal")
			builder.PoWBlock(pb)
		}
		if step.ExecutionPayload != nil {
			require.Equal(t, true, fork >= version.Gloas)
			envelope := unmarshalSignedExecutionPayloadEnvelope(t, testsFolderPath, folder.Name(), *step.ExecutionPayload)
			valid := step.Valid == nil || *step.Valid
			builder.ExecutionPayloadEnvelope(t, envelope, valid)
		}
		if step.PayloadAttestation != nil {
			require.Equal(t, true, fork >= version.Gloas)
			msg := unmarshalPayloadAttestationMessage(t, testsFolderPath, folder.Name(), *step.PayloadAttestation)
			valid := step.Valid == nil || *step.Valid
			builder.PayloadAttestation(t, msg, valid)
		}
		builder.Check(t, step.Check)
	}
}

func runBlobStep(t *testing.T,
	step Step,
	beaconBlock interfaces.ReadOnlySignedBeaconBlock,
	fork int,
	folder os.DirEntry,
	testsFolderPath string,
	builder *Builder,
) {
	blobs := step.Blobs
	proofs := step.Proofs
	if blobs != nil && *blobs != "null" {
		require.NotNil(t, beaconBlock)
		require.Equal(t, true, fork >= version.Deneb)

		block := beaconBlock.Block()
		root, err := block.HashTreeRoot()
		require.NoError(t, err)
		kzgs, err := block.Body().BlobKzgCommitments()
		require.NoError(t, err)

		blobsFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*blobs, ".ssz_snappy"))
		require.NoError(t, err)
		blobsSSZ, err := snappy.Decode(nil /* dst */, blobsFile)
		require.NoError(t, err)
		sh, err := beaconBlock.Header()
		require.NoError(t, err)
		requireVerifyExpected := errAssertionForStep(step, verification.ErrBlobInvalid)
		for index := 0; index*fieldparams.BlobLength < len(blobsSSZ); index++ {
			var proof []byte
			if index < len(proofs) {
				proofPTR := proofs[index]
				require.NotNil(t, proofPTR)
				proof, err = hexutil.Decode(*proofPTR)
				require.NoError(t, err)
			}

			blob := [fieldparams.BlobLength]byte{}
			copy(blob[:], blobsSSZ[index*fieldparams.BlobLength:])
			if len(proof) == 0 {
				proof = make([]byte, 48)
			}

			inclusionProof, err := blocks.MerkleProofKZGCommitment(block.Body(), index)
			require.NoError(t, err)
			pb := &ethpb.BlobSidecar{
				Index:                    uint64(index),
				Blob:                     blob[:],
				KzgCommitment:            kzgs[index],
				KzgProof:                 proof,
				SignedBlockHeader:        sh,
				CommitmentInclusionProof: inclusionProof,
			}
			ro, err := blocks.NewROBlobWithRoot(pb, root)
			require.NoError(t, err)
			ini, err := builder.vwait.WaitForInitializer(context.Background())
			require.NoError(t, err)
			bv := ini.NewBlobVerifier(ro, verification.SpectestBlobSidecarRequirements)
			ctx := context.Background()
			if err := bv.BlobIndexInBounds(); err != nil {
				t.Logf("BlobIndexInBounds error: %s", err.Error())
			}
			if err := bv.NotFromFutureSlot(); err != nil {
				t.Logf("NotFromFutureSlot error: %s", err.Error())
			}
			if err := bv.SlotAboveFinalized(); err != nil {
				t.Logf("SlotAboveFinalized error: %s", err.Error())
			}
			if err := bv.SidecarInclusionProven(); err != nil {
				t.Logf("SidecarInclusionProven error: %s", err.Error())
			}
			if err := bv.SidecarKzgProofVerified(); err != nil {
				t.Logf("SidecarKzgProofVerified error: %s", err.Error())
			}
			if err := bv.ValidProposerSignature(ctx); err != nil {
				t.Logf("ValidProposerSignature error: %s", err.Error())
			}
			if err := bv.SidecarParentSlotLower(); err != nil {
				t.Logf("SidecarParentSlotLower error: %s", err.Error())
			}
			if err := bv.SidecarDescendsFromFinalized(); err != nil {
				t.Logf("SidecarDescendsFromFinalized error: %s", err.Error())
			}
			if err := bv.SidecarProposerExpected(ctx); err != nil {
				t.Logf("SidecarProposerExpected error: %s", err.Error())
			}

			vsc, err := bv.VerifiedROBlob()
			requireVerifyExpected(t, err)

			if err == nil {
				require.NoError(t, builder.service.ReceiveBlob(context.Background(), vsc))
			}
		}
	}
}

func runDataColumnStep(t *testing.T,
	step Step,
	beaconBlock interfaces.ReadOnlySignedBeaconBlock,
	fork int,
	folder os.DirEntry,
	testsFolderPath string,
	builder *Builder,
) {
	columnFiles := step.DataColumns

	require.NotNil(t, beaconBlock)
	require.Equal(t, true, fork >= version.Fulu)

	block := beaconBlock.Block()
	root, err := block.HashTreeRoot()
	require.NoError(t, err)
	kzgs, err := block.Body().BlobKzgCommitments()
	require.NoError(t, err)
	sh, err := beaconBlock.Header()
	require.NoError(t, err)
	// Use the same error that the verification system returns for data columns
	errDataColumnsInvalid := errors.New("data columns failed verification")
	requireVerifyExpected := errAssertionForStep(step, errDataColumnsInvalid)

	var allColumns []blocks.RODataColumn

	for columnIndex, columnFile := range columnFiles {
		if columnFile == nil || *columnFile == "null" {
			continue
		}

		dataColumnFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*columnFile, ".ssz_snappy"))
		require.NoError(t, err)
		dataColumnSSZ, err := snappy.Decode(nil /* dst */, dataColumnFile)
		require.NoError(t, err)

		var pb *ethpb.DataColumnSidecar

		if step.Valid != nil && !*step.Valid {
			pb = &ethpb.DataColumnSidecar{}
			if err := pb.UnmarshalSSZ(dataColumnSSZ); err != nil {
				pb = &ethpb.DataColumnSidecar{
					Index:             uint64(columnIndex),
					Column:            [][]byte{},
					KzgCommitments:    kzgs,
					KzgProofs:         make([][]byte, 0),
					SignedBlockHeader: sh,
				}
			}
		} else {
			numCells := len(kzgs)
			column := make([][]byte, numCells)
			for cellIndex := range numCells {
				cell := make([]byte, 2048)
				cellStart := cellIndex * 2048
				cellEnd := cellStart + 2048
				if cellEnd <= len(dataColumnSSZ) {
					copy(cell, dataColumnSSZ[cellStart:cellEnd])
				}
				column[cellIndex] = cell
			}

			inclusionProof, err := blocks.MerkleProofKZGCommitments(block.Body())
			require.NoError(t, err)

			pb = &ethpb.DataColumnSidecar{
				Index:                        uint64(columnIndex),
				Column:                       column,
				KzgCommitments:               kzgs,
				SignedBlockHeader:            sh,
				KzgCommitmentsInclusionProof: inclusionProof,
			}
		}

		ro, err := blocks.NewRODataColumnWithRoot(pb, root)
		require.NoError(t, err)
		allColumns = append(allColumns, ro)
	}

	if len(allColumns) > 0 {
		ini, err := builder.vwait.WaitForInitializer(context.Background())
		require.NoError(t, err)
		// Use different verification requirements based on whether this is a valid or invalid test case
		var forkchoiceReqs []verification.Requirement
		if step.Valid != nil && !*step.Valid {
			forkchoiceReqs = verification.SpectestDataColumnSidecarRequirements
		} else {
			forkchoiceReqs = []verification.Requirement{
				verification.RequireNotFromFutureSlot,
				verification.RequireSlotAboveFinalized,
				verification.RequireValidProposerSignature,
				verification.RequireSidecarParentSlotLower,
				verification.RequireSidecarDescendsFromFinalized,
				verification.RequireSidecarInclusionProven,
				verification.RequireSidecarProposerExpected,
			}
		}
		dv := ini.NewDataColumnsVerifier(allColumns, forkchoiceReqs)
		ctx := t.Context()

		if step.Valid != nil && !*step.Valid {
			if err := dv.ValidFields(); err != nil {
				t.Logf("ValidFields error: %s", err.Error())
			}
		}

		if err := dv.NotFromFutureSlot(); err != nil {
			t.Logf("NotFromFutureSlot error: %s", err.Error())
		}
		if err := dv.SlotAboveFinalized(); err != nil {
			t.Logf("SlotAboveFinalized error: %s", err.Error())
		}
		if err := dv.SidecarInclusionProven(); err != nil {
			t.Logf("SidecarInclusionProven error: %s", err.Error())
		}
		if err := dv.ValidProposerSignature(ctx); err != nil {
			t.Logf("ValidProposerSignature error: %s", err.Error())
		}
		if err := dv.SidecarParentSlotLower(); err != nil {
			t.Logf("SidecarParentSlotLower error: %s", err.Error())
		}
		if err := dv.SidecarDescendsFromFinalized(); err != nil {
			t.Logf("SidecarDescendsFromFinalized error: %s", err.Error())
		}
		if err := dv.SidecarProposerExpected(ctx); err != nil {
			t.Logf("SidecarProposerExpected error: %s", err.Error())
		}

		vdc, err := dv.VerifiedRODataColumns()
		requireVerifyExpected(t, err)

		if err == nil {
			for _, column := range vdc {
				require.NoError(t, builder.service.ReceiveDataColumn(column))
			}
		}
	}
}

func errAssertionForStep(step Step, expect error) func(t *testing.T, err error) {
	if !*step.Valid {
		return func(t *testing.T, err error) {
			if expect.Error() == "data columns failed verification" {
				require.NotNil(t, err)
				require.Equal(t, true, strings.Contains(err.Error(), expect.Error()))
			} else {
				require.ErrorIs(t, err, expect)
			}
		}
	}
	return func(t *testing.T, err error) {
		if err != nil {
			require.ErrorIs(t, err, verification.ErrBlobInvalid)
			var me verification.VerificationMultiError
			ok := errors.As(err, &me)
			require.Equal(t, true, ok)
			fails := me.Failures()
			// we haven't performed any verification, so all the results should be this type
			fmsg := make([]string, 0, len(fails))
			for k, v := range fails {
				fmsg = append(fmsg, fmt.Sprintf("%s - %s", v.Error(), k.String()))
			}
			t.Fatal(strings.Join(fmsg, ";"))
		}
	}
}

// ----------------------------------------------------------------------------
// Phase 0
// ----------------------------------------------------------------------------

func unmarshalPhase0State(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconState{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafePhase0(base)
	require.NoError(t, err)
	return st
}

func unmarshalPhase0Block(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.BeaconBlock{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedPhase0Block(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.SignedBeaconBlock{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Altair
// ----------------------------------------------------------------------------

func unmarshalAltairState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeAltair(base)
	require.NoError(t, err)
	return st
}

func unmarshalAltairBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.BeaconBlockAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedAltairBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Bellatrix
// ----------------------------------------------------------------------------

func unmarshalBellatrixState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeBellatrix(base)
	require.NoError(t, err)
	return st
}

func unmarshalBellatrixBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.BeaconBlockBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedBellatrixBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Capella
// ----------------------------------------------------------------------------

func unmarshalCapellaState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeCapella(base)
	require.NoError(t, err)
	return st
}

func unmarshalCapellaBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.BeaconBlockCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedCapellaBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Deneb
// ----------------------------------------------------------------------------

func unmarshalDenebState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateDeneb{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeDeneb(base)
	require.NoError(t, err)
	return st
}

func unmarshalDenebBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.BeaconBlockDeneb{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedDenebBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockDeneb{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Electra
// ----------------------------------------------------------------------------

func unmarshalElectraState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateElectra{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeElectra(base)
	require.NoError(t, err)
	return st
}

func unmarshalElectraBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.BeaconBlockElectra{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockElectra{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedElectraBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockElectra{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Fulu
// ----------------------------------------------------------------------------

func unmarshalFuluState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateFulu{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeFulu(base)
	require.NoError(t, err)
	return st
}

func unmarshalFuluBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.BeaconBlockElectra{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockFulu{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedFuluBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockFulu{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

// ----------------------------------------------------------------------------
// Gloas
// ----------------------------------------------------------------------------

func unmarshalGloasState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateGloas{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	return st
}

func unmarshalGloasBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.BeaconBlockGloas{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{Block: base, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedGloasBlock(t *testing.T, raw []byte) interfaces.SignedBeaconBlock {
	base := &ethpb.SignedBeaconBlockGloas{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

func unmarshalSignedExecutionPayloadEnvelope(t *testing.T, testsFolderPath, caseName, fileBase string) interfaces.ROSignedExecutionPayloadEnvelope {
	raw, err := util.BazelFileBytes(testsFolderPath, caseName, fmt.Sprint(fileBase, ".ssz_snappy"))
	require.NoError(t, err)
	decoded, err := snappy.Decode(nil /* dst */, raw)
	require.NoError(t, err)
	pb := &ethpb.SignedExecutionPayloadEnvelope{}
	require.NoError(t, pb.UnmarshalSSZ(decoded), "Failed to unmarshal execution payload envelope")
	env, err := blocks.WrappedROSignedExecutionPayloadEnvelope(pb)
	require.NoError(t, err)
	return env
}

func unmarshalPayloadAttestationMessage(t *testing.T, testsFolderPath, caseName, fileBase string) *ethpb.PayloadAttestationMessage {
	raw, err := util.BazelFileBytes(testsFolderPath, caseName, fmt.Sprint(fileBase, ".ssz_snappy"))
	require.NoError(t, err)
	decoded, err := snappy.Decode(nil /* dst */, raw)
	require.NoError(t, err)
	msg := &ethpb.PayloadAttestationMessage{}
	require.NoError(t, msg.UnmarshalSSZ(decoded), "Failed to unmarshal payload attestation message")
	return msg
}
