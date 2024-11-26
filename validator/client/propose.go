package client

// Validator client proposer functions.
import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/async"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	domainDataErr           = "could not get domain data"
	signingRootErr          = "could not get signing root"
	signExitErr             = "could not sign voluntary exit proposal"
	failedBlockSignLocalErr = "block rejected by local protection"
)

// ProposeBlock proposes a new beacon block for a given slot. This method collects the
// previous beacon block, any pending deposits, and ETH1 data from the beacon
// chain node to construct the new block. The new block is then processed with
// the state root computation, and finally signed by the validator before being
// sent back to the beacon node for broadcasting.
func (v *validator) ProposeBlock(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) {
	if slot == 0 {
		log.Debug("Assigned to genesis slot, skipping proposal")
		return
	}
	ctx, span := trace.StartSpan(ctx, "validator.ProposeBlock")
	defer span.End()

	lock := async.NewMultilock(fmt.Sprint(iface.RoleProposer), string(pubKey[:]))
	lock.Lock()
	defer lock.Unlock()

	fmtKey := fmt.Sprintf("%#x", pubKey[:])
	span.SetAttributes(trace.StringAttribute("validator", fmtKey))
	log := log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])))

	// Sign randao reveal, it's used to request block from beacon node
	epoch := primitives.Epoch(slot / params.BeaconConfig().SlotsPerEpoch)
	randaoReveal, err := v.signRandaoReveal(ctx, pubKey, epoch, slot)
	if err != nil {
		log.WithError(err).Error("Failed to sign randao reveal")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	g, err := v.Graffiti(ctx, pubKey)
	if err != nil {
		// Graffiti is not a critical enough to fail block production and cause
		// validator to miss block reward. When failed, validator should continue
		// to produce the block.
		log.WithError(err).Warn("Could not get graffiti")
	}

	// Request block from beacon node
	b, err := v.validatorClient.BeaconBlock(ctx, &ethpb.BlockRequest{
		Slot:         slot,
		RandaoReveal: randaoReveal,
		Graffiti:     g,
	})
	if err != nil {
		log.WithField("slot", slot).WithError(err).Error("Failed to request block from beacon node")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	// Sign returned block from beacon node
	wb, err := blocks.NewBeaconBlock(b.Block)
	if err != nil {
		log.WithError(err).Error("Failed to wrap block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	sig, signingRoot, err := v.signBlock(ctx, pubKey, epoch, slot, wb)
	if err != nil {
		log.WithError(err).Error("Failed to sign block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	blk, err := blocks.BuildSignedBeaconBlock(wb, sig)
	if err != nil {
		log.WithError(err).Error("Failed to build signed beacon block")
		return
	}

	if err := v.db.SlashableProposalCheck(ctx, pubKey, blk, signingRoot, v.emitAccountMetrics, ValidatorProposeFailVec); err != nil {
		log.WithFields(
			blockLogFields(pubKey, wb, nil),
		).WithError(err).Error("Failed block slashing protection check")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	var genericSignedBlock *ethpb.GenericSignedBeaconBlock
	// Special handling for Deneb blocks and later version because of blob side cars.
	if blk.Version() >= version.Deneb && !blk.IsBlinded() {
		pb, err := blk.Proto()
		if err != nil {
			log.WithError(err).Error("Failed to get deneb block")
			return
		}
		switch blk.Version() {
		case version.Deneb:
			genericSignedBlock, err = buildGenericSignedBlockDenebWithBlobs(pb, b)
			if err != nil {
				log.WithError(err).Error("Failed to build generic signed block")
				return
			}
		case version.Electra:
			genericSignedBlock, err = buildGenericSignedBlockElectraWithBlobs(pb, b)
			if err != nil {
				log.WithError(err).Error("Failed to build generic signed block")
				return
			}
		default:
			log.Errorf("Unsupported block version %s", version.String(blk.Version()))
		}
	} else {
		genericSignedBlock, err = blk.PbGenericBlock()
		if err != nil {
			log.WithError(err).Error("Failed to create proposal request")
			if v.emitAccountMetrics {
				ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
			}
			return
		}
	}

	blkResp, err := v.validatorClient.ProposeBeaconBlock(ctx, genericSignedBlock)
	if err != nil {
		log.WithField("slot", slot).WithError(err).Error("Failed to propose block")
		if v.emitAccountMetrics {
			ValidatorProposeFailVec.WithLabelValues(fmtKey).Inc()
		}
		return
	}

	span.SetAttributes(
		trace.StringAttribute("blockRoot", fmt.Sprintf("%#x", blkResp.BlockRoot)),
		trace.Int64Attribute("numDeposits", int64(len(blk.Block().Body().Deposits()))),
		trace.Int64Attribute("numAttestations", int64(len(blk.Block().Body().Attestations()))),
	)

	if err := logProposedBlock(log, blk, blkResp.BlockRoot); err != nil {
		log.WithError(err).Error("Failed to log proposed block")
	}

	if v.emitAccountMetrics {
		ValidatorProposeSuccessVec.WithLabelValues(fmtKey).Inc()
	}
}

func logProposedBlock(log *logrus.Entry, blk interfaces.SignedBeaconBlock, blkRoot []byte) error {
	if blk.Version() >= version.Bellatrix {
		p, err := blk.Block().Body().Execution()
		if err != nil {
			return errors.Wrap(err, "failed to get execution payload")
		}
		log = log.WithFields(logrus.Fields{
			"payloadHash": fmt.Sprintf("%#x", bytesutil.Trunc(p.BlockHash())),
			"parentHash":  fmt.Sprintf("%#x", bytesutil.Trunc(p.ParentHash())),
			"blockNumber": p.BlockNumber(),
		})
		if !blk.IsBlinded() {
			txs, err := p.Transactions()
			if err != nil {
				return errors.Wrap(err, "failed to get execution payload transactions")
			}
			log = log.WithField("txCount", len(txs))
		}
		if p.GasLimit() != 0 {
			log = log.WithField("gasUtilized", float64(p.GasUsed())/float64(p.GasLimit()))
		}
		if blk.Version() >= version.Capella && !blk.IsBlinded() {
			withdrawals, err := p.Withdrawals()
			if err != nil {
				return errors.Wrap(err, "failed to get execution payload withdrawals")
			}
			log = log.WithField("withdrawalCount", len(withdrawals))
		}
		if blk.Version() >= version.Deneb {
			kzgs, err := blk.Block().Body().BlobKzgCommitments()
			if err != nil {
				return errors.Wrap(err, "failed to get kzg commitments")
			} else if len(kzgs) != 0 {
				log = log.WithField("kzgCommitmentCount", len(kzgs))
			}
		}
	}

	br := fmt.Sprintf("%#x", bytesutil.Trunc(blkRoot))
	graffiti := blk.Block().Body().Graffiti()
	log.WithFields(logrus.Fields{
		"slot":             blk.Block().Slot(),
		"blockRoot":        br,
		"attestationCount": len(blk.Block().Body().Attestations()),
		"depositCount":     len(blk.Block().Body().Deposits()),
		"graffiti":         string(graffiti[:]),
		"fork":             version.String(blk.Block().Version()),
	}).Info("Submitted new block")

	return nil
}

func buildGenericSignedBlockDenebWithBlobs(pb proto.Message, b *ethpb.GenericBeaconBlock) (*ethpb.GenericSignedBeaconBlock, error) {
	denebBlock, ok := pb.(*ethpb.SignedBeaconBlockDeneb)
	if !ok {
		return nil, errors.New("could cast to deneb block")
	}
	return &ethpb.GenericSignedBeaconBlock{
		Block: &ethpb.GenericSignedBeaconBlock_Deneb{
			Deneb: &ethpb.SignedBeaconBlockContentsDeneb{
				Block:     denebBlock,
				KzgProofs: b.GetDeneb().KzgProofs,
				Blobs:     b.GetDeneb().Blobs,
			},
		},
	}, nil
}

func buildGenericSignedBlockElectraWithBlobs(pb proto.Message, b *ethpb.GenericBeaconBlock) (*ethpb.GenericSignedBeaconBlock, error) {
	electraBlock, ok := pb.(*ethpb.SignedBeaconBlockElectra)
	if !ok {
		return nil, errors.New("could cast to electra block")
	}
	return &ethpb.GenericSignedBeaconBlock{
		Block: &ethpb.GenericSignedBeaconBlock_Electra{
			Electra: &ethpb.SignedBeaconBlockContentsElectra{
				Block:     electraBlock,
				KzgProofs: b.GetElectra().KzgProofs,
				Blobs:     b.GetElectra().Blobs,
			},
		},
	}, nil
}

// ProposeExit performs a voluntary exit on a validator.
// The exit is signed by the validator before being sent to the beacon node for broadcasting.
func ProposeExit(
	ctx context.Context,
	validatorClient iface.ValidatorClient,
	signer iface.SigningFunc,
	pubKey []byte,
	epoch primitives.Epoch,
) error {
	ctx, span := trace.StartSpan(ctx, "validator.ProposeExit")
	defer span.End()

	signedExit, err := CreateSignedVoluntaryExit(ctx, validatorClient, signer, pubKey, epoch)
	if err != nil {
		return errors.Wrap(err, "failed to create signed voluntary exit")
	}
	exitResp, err := validatorClient.ProposeExit(ctx, signedExit)
	if err != nil {
		return errors.Wrap(err, "failed to propose voluntary exit")
	}

	span.SetAttributes(
		trace.StringAttribute("exitRoot", fmt.Sprintf("%#x", exitResp.ExitRoot)),
	)
	return nil
}

func CurrentEpoch(genesisTime *timestamp.Timestamp) (primitives.Epoch, error) {
	totalSecondsPassed := prysmTime.Now().Unix() - genesisTime.Seconds
	currentSlot := primitives.Slot((uint64(totalSecondsPassed)) / params.BeaconConfig().SecondsPerSlot)
	currentEpoch := slots.ToEpoch(currentSlot)
	return currentEpoch, nil
}

func CreateSignedVoluntaryExit(
	ctx context.Context,
	validatorClient iface.ValidatorClient,
	signer iface.SigningFunc,
	pubKey []byte,
	epoch primitives.Epoch,
) (*ethpb.SignedVoluntaryExit, error) {
	ctx, span := trace.StartSpan(ctx, "validator.CreateSignedVoluntaryExit")
	defer span.End()

	indexResponse, err := validatorClient.ValidatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: pubKey})
	if err != nil {
		return nil, errors.Wrap(err, "gRPC call to get validator index failed")
	}
	exit := &ethpb.VoluntaryExit{Epoch: epoch, ValidatorIndex: indexResponse.Index}
	slot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve slot")
	}
	sig, err := signVoluntaryExit(ctx, validatorClient, signer, pubKey, exit, slot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign voluntary exit")
	}

	return &ethpb.SignedVoluntaryExit{Exit: exit, Signature: sig}, nil
}

// Sign randao reveal with randao domain and private key.
func (v *validator) signRandaoReveal(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch, slot primitives.Slot) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signRandaoReveal")
	defer span.End()

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainRandao[:])
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}

	var randaoReveal bls.Signature
	sszUint := primitives.SSZUint64(epoch)
	root, err := signing.ComputeSigningRoot(&sszUint, domain.SignatureDomain)
	if err != nil {
		return nil, err
	}
	randaoReveal, err = v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_Epoch{Epoch: epoch},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, err
	}
	return randaoReveal.Marshal(), nil
}

// Sign block with proposer domain and private key.
// Returns the signature, block signing root, and any error.
func (v *validator) signBlock(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch, slot primitives.Slot, b interfaces.ReadOnlyBeaconBlock) ([]byte, [32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signBlock")
	defer span.End()

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBeaconProposer[:])
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, [32]byte{}, errors.New(domainDataErr)
	}

	blockRoot, err := signing.ComputeSigningRoot(b, domain.SignatureDomain)
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, signingRootErr)
	}
	sro, err := b.AsSignRequestObject()
	if err != nil {
		return nil, [32]byte{}, err
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     blockRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          sro,
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "could not sign block proposal")
	}
	return sig.Marshal(), blockRoot, nil
}

// Sign voluntary exit with proposer domain and private key.
func signVoluntaryExit(
	ctx context.Context,
	validatorClient iface.ValidatorClient,
	signer iface.SigningFunc,
	pubKey []byte,
	exit *ethpb.VoluntaryExit,
	slot primitives.Slot,
) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signVoluntaryExit")
	defer span.End()

	req := &ethpb.DomainRequest{
		Epoch:  exit.Epoch,
		Domain: params.BeaconConfig().DomainVoluntaryExit[:],
	}

	domain, err := validatorClient.DomainData(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}

	exitRoot, err := signing.ComputeSigningRoot(exit, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, signingRootErr)
	}

	sig, err := signer(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey,
		SigningRoot:     exitRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_Exit{Exit: exit},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, signExitErr)
	}
	return sig.Marshal(), nil
}

// Graffiti gets the graffiti from cli or file for the validator public key.
func (v *validator) Graffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.Graffiti")
	defer span.End()

	if v.proposerSettings != nil {
		// Check proposer settings for specific key first
		if v.proposerSettings.ProposeConfig != nil {
			option, ok := v.proposerSettings.ProposeConfig[pubKey]
			if ok && option.GraffitiConfig != nil {
				return []byte(option.GraffitiConfig.Graffiti), nil
			}
		}
		// Check proposer settings for default settings second
		if v.proposerSettings.DefaultConfig != nil {
			if v.proposerSettings.DefaultConfig.GraffitiConfig != nil {
				return []byte(v.proposerSettings.DefaultConfig.GraffitiConfig.Graffiti), nil
			}
		}
	}

	// When specified, use default graffiti from the command line.
	if len(v.graffiti) != 0 {
		return bytesutil.PadTo(v.graffiti, 32), nil
	}

	if v.graffitiStruct == nil {
		return nil, errors.New("graffitiStruct can't be nil")
	}

	// When specified, individual validator specified graffiti takes the third priority.
	idx, err := v.validatorClient.ValidatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: pubKey[:]})
	if err != nil {
		return nil, err
	}
	g, ok := v.graffitiStruct.Specific[idx.Index]
	if ok {
		return bytesutil.PadTo([]byte(g), 32), nil
	}

	// When specified, a graffiti from the ordered list in the file take fourth priority.
	if v.graffitiOrderedIndex < uint64(len(v.graffitiStruct.Ordered)) {
		graffiti := v.graffitiStruct.Ordered[v.graffitiOrderedIndex]
		v.graffitiOrderedIndex = v.graffitiOrderedIndex + 1
		err := v.db.SaveGraffitiOrderedIndex(ctx, v.graffitiOrderedIndex)
		if err != nil {
			return nil, errors.Wrap(err, "failed to update graffiti ordered index")
		}
		return bytesutil.PadTo([]byte(graffiti), 32), nil
	}

	// When specified, a graffiti from the random list in the file take Fifth priority.
	if len(v.graffitiStruct.Random) != 0 {
		r := rand.NewGenerator()
		r.Seed(time.Now().Unix())
		i := r.Uint64() % uint64(len(v.graffitiStruct.Random))
		return bytesutil.PadTo([]byte(v.graffitiStruct.Random[i]), 32), nil
	}

	// Finally, default graffiti if specified in the file will be used.
	if v.graffitiStruct.Default != "" {
		return bytesutil.PadTo([]byte(v.graffitiStruct.Default), 32), nil
	}

	return []byte{}, nil
}

func (v *validator) SetGraffiti(ctx context.Context, pubkey [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error {
	ctx, span := trace.StartSpan(ctx, "validator.SetGraffiti")
	defer span.End()

	if graffiti == nil {
		return nil
	}
	settings := &proposer.Settings{}
	if v.proposerSettings != nil {
		settings = v.proposerSettings.Clone()
	}
	if settings.ProposeConfig == nil {
		settings.ProposeConfig = map[[48]byte]*proposer.Option{pubkey: {GraffitiConfig: &proposer.GraffitiConfig{Graffiti: string(graffiti)}}}
		return v.SetProposerSettings(ctx, settings)
	}
	option, ok := settings.ProposeConfig[pubkey]
	if !ok || option == nil {
		settings.ProposeConfig[pubkey] = &proposer.Option{GraffitiConfig: &proposer.GraffitiConfig{
			Graffiti: string(graffiti),
		}}
	} else {
		option.GraffitiConfig = &proposer.GraffitiConfig{
			Graffiti: string(graffiti),
		}
	}
	return v.SetProposerSettings(ctx, settings) // save the proposer settings
}

func (v *validator) DeleteGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	ctx, span := trace.StartSpan(ctx, "validator.DeleteGraffiti")
	defer span.End()

	if v.proposerSettings == nil || v.proposerSettings.ProposeConfig == nil {
		return errors.New("attempted to delete graffiti without proposer settings, graffiti will default to flag options")
	}
	ps := v.proposerSettings.Clone()
	option, ok := ps.ProposeConfig[pubKey]
	if !ok || option == nil {
		return fmt.Errorf("graffiti not found in proposer settings for pubkey:%s", hexutil.Encode(pubKey[:]))
	}
	option.GraffitiConfig = nil
	return v.SetProposerSettings(ctx, ps) // save the proposer settings
}

func blockLogFields(pubKey [fieldparams.BLSPubkeyLength]byte, blk interfaces.ReadOnlyBeaconBlock, sig []byte) logrus.Fields {
	fields := logrus.Fields{
		"proposerPublicKey": fmt.Sprintf("%#x", pubKey),
		"proposerIndex":     blk.ProposerIndex(),
		"blockSlot":         blk.Slot(),
	}
	if sig != nil {
		fields["signature"] = fmt.Sprintf("%#x", sig)
	}
	return fields
}
