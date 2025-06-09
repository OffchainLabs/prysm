package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) beaconBlock(ctx context.Context, slot primitives.Slot, randaoReveal, graffiti []byte) (*ethpb.GenericBeaconBlock, error) {
	queryParams := neturl.Values{}
	queryParams.Add("randao_reveal", hexutil.Encode(randaoReveal))
	if len(graffiti) > 0 {
		queryParams.Add("graffiti", hexutil.Encode(graffiti))
	}
	queryUrl := apiutil.BuildURL(fmt.Sprintf("/eth/v3/validator/blocks/%d", slot), queryParams)
	if features.Get().EnableSSZ {
		data, header, err := c.jsonRestHandler.GetSSZ(ctx, queryUrl)
		if err != nil {
			return nil, err
		}
		ver, err := version.FromString(header.Get(api.VersionHeader))
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("unsupported header version %s", header.Get(api.VersionHeader)))
		}
		isBlindedRaw := header.Get(api.ExecutionPayloadBlindedHeader)
		isBlinded, err := strconv.ParseBool(isBlindedRaw)
		if err != nil {
			return nil, err
		}
		return processBlockSSZResponse(ver, data, isBlinded)
	}

	return c.getBeaconBlockJSON(ctx, queryUrl)
}

func (c *beaconApiValidatorClient) getBeaconBlockJSON(ctx context.Context, queryUrl string) (*ethpb.GenericBeaconBlock, error) {
	produceBlockV3ResponseJson := structs.ProduceBlockV3Response{}
	err := c.jsonRestHandler.Get(ctx, queryUrl, &produceBlockV3ResponseJson)
	if err != nil {
		return nil, err
	}

	return processBlockResponse(
		produceBlockV3ResponseJson.Version,
		produceBlockV3ResponseJson.ExecutionPayloadBlinded,
		json.NewDecoder(bytes.NewReader(produceBlockV3ResponseJson.Data)),
	)
}

func processBlockSSZResponse(ver int, data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if ver >= version.Fulu {
		return processBlockSSZResponseFulu(data, isBlinded)
	}
	if ver >= version.Electra {
		return processBlockSSZResponseElectra(data, isBlinded)
	}
	if ver >= version.Deneb {
		return processBlockSSZResponseDeneb(data, isBlinded)
	}
	if ver >= version.Capella {
		return processBlockSSZResponseCapella(data, isBlinded)
	}
	if ver >= version.Bellatrix {
		return processBlockSSZResponseBellatrix(data, isBlinded)
	}
	if ver >= version.Altair {
		block := &ethpb.BeaconBlockAltair{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Altair{Altair: block}}, nil
	}
	if ver >= version.Phase0 {
		block := &ethpb.BeaconBlock{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Phase0{Phase0: block}}, nil
	}
	return nil, fmt.Errorf("unsupported block version %d", ver)
}

func processBlockSSZResponseFulu(data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		blindedBlock := &ethpb.BlindedBeaconBlockFulu{}
		if err := blindedBlock.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedFulu{BlindedFulu: blindedBlock}, IsBlinded: true}, nil
	}
	block := &ethpb.BeaconBlockContentsFulu{}
	if err := block.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Fulu{Fulu: block}}, nil
}

func processBlockSSZResponseElectra(data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		blindedBlock := &ethpb.BlindedBeaconBlockElectra{}
		if err := blindedBlock.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedElectra{BlindedElectra: blindedBlock}, IsBlinded: true}, nil
	}
	block := &ethpb.BeaconBlockContentsElectra{}
	if err := block.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Electra{Electra: block}}, nil
}

func processBlockSSZResponseDeneb(data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		blindedBlock := &ethpb.BlindedBeaconBlockDeneb{}
		if err := blindedBlock.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedDeneb{BlindedDeneb: blindedBlock}, IsBlinded: true}, nil
	}
	block := &ethpb.BeaconBlockContentsDeneb{}
	if err := block.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Deneb{Deneb: block}}, nil
}

func processBlockSSZResponseCapella(data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		blindedBlock := &ethpb.BlindedBeaconBlockCapella{}
		if err := blindedBlock.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedCapella{BlindedCapella: blindedBlock}, IsBlinded: true}, nil
	}
	block := &ethpb.BeaconBlockCapella{}
	if err := block.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Capella{Capella: block}}, nil
}

func processBlockSSZResponseBellatrix(data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		blindedBlock := &ethpb.BlindedBeaconBlockBellatrix{}
		if err := blindedBlock.UnmarshalSSZ(data); err != nil {
			return nil, err
		}
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: blindedBlock}, IsBlinded: true}, nil
	}
	block := &ethpb.BeaconBlockBellatrix{}
	if err := block.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Bellatrix{Bellatrix: block}}, nil
}

func processAndConvert(decoder *json.Decoder, dest ethpb.GenericConverter, version string, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	typeName := version
	if isBlinded {
		typeName = "blinded " + typeName
	}

	if err := decoder.Decode(dest); err != nil {
		return nil, errors.Wrapf(err, "failed to decode %s block response json", typeName)
	}

	genericBlock, err := dest.ToGeneric()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s block", typeName)
	}
	return genericBlock, nil
}

func processBlockResponse(ver string, isBlinded bool, decoder *json.Decoder) (*ethpb.GenericBeaconBlock, error) {
	if decoder == nil {
		return nil, errors.New("no produce block json decoder found")
	}

	switch ver {
	case version.String(version.Phase0):
		return processAndConvert(decoder, &structs.BeaconBlock{}, "phase0", false)

	case version.String(version.Altair):
		return processAndConvert(decoder, &structs.BeaconBlockAltair{}, "altair", false)

	case version.String(version.Bellatrix):
		return processBellatrixBlock(decoder, isBlinded)

	case version.String(version.Capella):
		return processCapellaBlock(decoder, isBlinded)

	case version.String(version.Deneb):
		return processDenebBlock(decoder, isBlinded)

	case version.String(version.Electra):
		return processElectraBlock(decoder, isBlinded)

	case version.String(version.Fulu):
		return processFuluBlock(decoder, isBlinded)

	default:
		return nil, errors.Errorf("unsupported consensus version `%s`", ver)
	}
}

func processBellatrixBlock(decoder *json.Decoder, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		return processAndConvert(decoder, &structs.BlindedBeaconBlockBellatrix{}, "bellatrix", true)
	}
	return processAndConvert(decoder, &structs.BeaconBlockBellatrix{}, "bellatrix", false)
}

func processCapellaBlock(decoder *json.Decoder, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		return processAndConvert(decoder, &structs.BlindedBeaconBlockCapella{}, "capella", true)
	}
	return processAndConvert(decoder, &structs.BeaconBlockCapella{}, "capella", false)
}

func processDenebBlock(decoder *json.Decoder, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		return processAndConvert(decoder, &structs.BlindedBeaconBlockDeneb{}, "deneb", true)
	}
	return processAndConvert(decoder, &structs.BeaconBlockContentsDeneb{}, "deneb", false)
}

func processElectraBlock(decoder *json.Decoder, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		return processAndConvert(decoder, &structs.BlindedBeaconBlockElectra{}, "electra", true)
	}
	return processAndConvert(decoder, &structs.BeaconBlockContentsElectra{}, "electra", false)
}

func processFuluBlock(decoder *json.Decoder, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	if isBlinded {
		return processAndConvert(decoder, &structs.BlindedBeaconBlockFulu{}, "fulu", true)
	}
	return processAndConvert(decoder, &structs.BeaconBlockContentsFulu{}, "fulu", false)
}
