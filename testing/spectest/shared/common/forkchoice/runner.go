package forkchoice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func init() {
	transition.SkipSlotCache.Disable()
}

// Run executes "forkchoice"  and "sync" test.
func Run(t *testing.T, config string, fork int) {
	runTest(t, config, fork, "fork_choice")
	if fork >= version.Bellatrix {
		runTest(t, config, fork, "sync")
	}
}

func runTest(t *testing.T, config string, fork int, basePath string) { // nolint:gocognit
	require.NoError(t, utils.SetConfig(t, config))
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

		for _, folder := range testFolders {
			t.Run(folder.Name(), func(t *testing.T) {
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
				default:
					t.Fatalf("unknown fork version: %v", fork)
				}

				builder := NewBuilder(t, beaconState, beaconBlock)

				for _, step := range steps {
					if step.Tick != nil {
						builder.Tick(t, int64(*step.Tick))
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
						default:
							t.Fatalf("unknown fork version: %v", fork)
						}
					}
					runBlobStep(t, step, beaconBlock, fork, folder, testsFolderPath, builder)
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
					builder.Check(t, step.Check)
				}
			})
		}
	}
}

func unmarshalPhase0State(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconState{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoPhase0(base)
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

func unmarshalAltairState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoAltair(base)
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

func unmarshalBellatrixState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoBellatrix(base)
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

func unmarshalCapellaState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoCapella(base)
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

func unmarshalDenebState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateDeneb{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoDeneb(base)
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

func unmarshalElectraState(t *testing.T, raw []byte) state.BeaconState {
	base := &ethpb.BeaconStateElectra{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoElectra(base)
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

func errAssertionForStep(step Step, expect error) func(t *testing.T, err error) {
	if !*step.Valid {
		return func(t *testing.T, err error) {
			require.ErrorIs(t, err, expect)
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
