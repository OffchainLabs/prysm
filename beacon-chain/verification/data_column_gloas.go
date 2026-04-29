package verification

import (
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/pkg/errors"
)

// GossipDataColumnSidecarRequirementsGloas defines the full Gloas gossip validation set.
var GossipDataColumnSidecarRequirementsGloas = []Requirement{
	RequireBlockSeenGloas,
	RequireSlotMatchesBlockGloas,
	RequireValidFieldsGloas,
	RequireCorrectSubnet,
	RequireSidecarKzgProofVerifiedGloas,
	RequireNotSeenGloas,
}

// PendingGloasColumnRequirements defines the requirements for columns queued before their block arrived.
var PendingGloasColumnRequirements = []Requirement{
	RequireSlotMatchesBlockGloas,
	RequireValidFieldsGloas,
	RequireSidecarKzgProofVerifiedGloas,
}

type ROGloasDataColumnVerifier struct {
	sidecar blocks.RODataColumn
	block   interfaces.ReadOnlyBeaconBlock
	results *results

	kzgCommitments [][]byte
}

var _ GloasDataColumnVerifier = &ROGloasDataColumnVerifier{}

func NewGloasDataColumnVerifier(sidecar blocks.RODataColumn, block interfaces.ReadOnlyBeaconBlock, reqs []Requirement) GloasDataColumnVerifier {
	return &ROGloasDataColumnVerifier{
		sidecar: sidecar,
		block:   block,
		results: newResults(reqs...),
	}
}

func (v *ROGloasDataColumnVerifier) SatisfyRequirement(req Requirement) {
	v.results.record(req, nil)
}

func (v *ROGloasDataColumnVerifier) VerifiedRODataColumn() (blocks.VerifiedRODataColumn, error) {
	if !v.results.allSatisfied() {
		return blocks.VerifiedRODataColumn{}, v.results.errors(errColumnsInvalid)
	}
	return blocks.NewVerifiedRODataColumn(v.sidecar), nil
}

func (v *ROGloasDataColumnVerifier) VerifyDataColumnSidecarSlotMatchesBlockGloas() (err error) {
	if ok, err := v.results.cached(RequireSlotMatchesBlockGloas); ok {
		return err
	}
	defer v.record(RequireSlotMatchesBlockGloas, &err)

	if v.sidecar.Slot() != v.block.Slot() {
		return errors.New("data column sidecar slot does not match block slot")
	}
	return nil
}

func (v *ROGloasDataColumnVerifier) VerifyDataColumnSidecarGloas() (err error) {
	if ok, err := v.results.cached(RequireValidFieldsGloas); ok {
		return err
	}
	defer v.record(RequireValidFieldsGloas, &err)

	kzgCommitments, err := v.blobKzgCommitments()
	if err != nil {
		return err
	}
	if v.sidecar.Index() >= fieldparams.NumberOfColumns {
		return peerdas.ErrIndexTooLarge
	}
	if len(v.sidecar.Column()) == 0 {
		return peerdas.ErrNoKzgCommitments
	}
	if len(v.sidecar.Column()) != len(kzgCommitments) || len(v.sidecar.Column()) != len(v.sidecar.KzgProofs()) {
		return peerdas.ErrMismatchLength
	}
	return nil
}

func (v *ROGloasDataColumnVerifier) CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) (err error) {
	if ok, err := v.results.cached(RequireCorrectSubnet); ok {
		return err
	}
	defer v.record(RequireCorrectSubnet, &err)

	if len(expectedTopics) != 1 {
		return columnErrBuilder(errBadTopicLength)
	}
	expectedTopic := expectedTopics[0] + "/"
	actualSubnet := peerdas.ComputeSubnetForDataColumnSidecar(v.sidecar.Index())
	actualSubTopic := fmt.Sprintf(dataColumnSidecarSubTopic, actualSubnet)
	if !strings.Contains(expectedTopic, actualSubTopic) {
		return columnErrBuilder(errBadTopic)
	}
	return nil
}

func (v *ROGloasDataColumnVerifier) VerifyDataColumnSidecarKzgProofsGloas() (err error) {
	if ok, err := v.results.cached(RequireSidecarKzgProofVerifiedGloas); ok {
		return err
	}
	defer v.record(RequireSidecarKzgProofVerifiedGloas, &err)

	kzgCommitments, err := v.blobKzgCommitments()
	if err != nil {
		return err
	}
	v.sidecar.SetBidCommitments(kzgCommitments)
	bundle := blocks.RODataColumnsToCellProofBundlesWithSize(v.sidecar)
	return peerdas.VerifyDataColumnsCellsKZGProofs(bundle.Size, bundle.Iterator)
}

func (v *ROGloasDataColumnVerifier) blobKzgCommitments() ([][]byte, error) {
	if v.kzgCommitments != nil {
		return v.kzgCommitments, nil
	}
	signedBid, err := v.block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return nil, errors.Wrap(err, "read signed execution payload bid")
	}
	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return nil, errors.Wrap(err, "wrap signed execution payload bid")
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		return nil, errors.Wrap(err, "read execution payload bid")
	}
	v.kzgCommitments = bid.BlobKzgCommitments()
	return v.kzgCommitments, nil
}

func (v *ROGloasDataColumnVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}
	v.results.record(req, *err)
}
