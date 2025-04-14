package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"

	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
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

// nolint: gocognit
func processBlockResponse(ver string, isBlinded bool, decoder *json.Decoder) (*ethpb.GenericBeaconBlock, error) {
	var response *ethpb.GenericBeaconBlock
	if decoder == nil {
		return nil, errors.New("no produce block json decoder found")
	}
	switch ver {
	case version.String(version.Phase0):
		jsonPhase0Block := structs.BeaconBlock{}
		if err := decoder.Decode(&jsonPhase0Block); err != nil {
			return nil, errors.Wrap(err, "failed to decode phase0 block response json")
		}
		genericBlock, err := jsonPhase0Block.ToGeneric()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get phase0 block")
		}
		response = genericBlock
	case version.String(version.Altair):
		jsonAltairBlock := structs.BeaconBlockAltair{}
		if err := decoder.Decode(&jsonAltairBlock); err != nil {
			return nil, errors.Wrap(err, "failed to decode altair block response json")
		}
		genericBlock, err := jsonAltairBlock.ToGeneric()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get altair block")
		}
		response = genericBlock
	case version.String(version.Bellatrix):
		if isBlinded {
			jsonBellatrixBlock := structs.BlindedBeaconBlockBellatrix{}
			if err := decoder.Decode(&jsonBellatrixBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode blinded bellatrix block response json")
			}
			genericBlock, err := jsonBellatrixBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get blinded bellatrix block")
			}
			response = genericBlock
		} else {
			jsonBellatrixBlock := structs.BeaconBlockBellatrix{}
			if err := decoder.Decode(&jsonBellatrixBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode bellatrix block response json")
			}
			genericBlock, err := jsonBellatrixBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get bellatrix block")
			}
			response = genericBlock
		}
	case version.String(version.Capella):
		if isBlinded {
			jsonCapellaBlock := structs.BlindedBeaconBlockCapella{}
			if err := decoder.Decode(&jsonCapellaBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode blinded capella block response json")
			}
			genericBlock, err := jsonCapellaBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get blinded capella block")
			}
			response = genericBlock
		} else {
			jsonCapellaBlock := structs.BeaconBlockCapella{}
			if err := decoder.Decode(&jsonCapellaBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode capella block response json")
			}
			genericBlock, err := jsonCapellaBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get capella block")
			}
			response = genericBlock
		}
	case version.String(version.Deneb):
		if isBlinded {
			jsonDenebBlock := structs.BlindedBeaconBlockDeneb{}
			if err := decoder.Decode(&jsonDenebBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode blinded deneb block response json")
			}
			genericBlock, err := jsonDenebBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get blinded deneb block")
			}
			response = genericBlock
		} else {
			jsonDenebBlockContents := structs.BeaconBlockContentsDeneb{}
			if err := decoder.Decode(&jsonDenebBlockContents); err != nil {
				return nil, errors.Wrap(err, "failed to decode deneb block response json")
			}
			genericBlock, err := jsonDenebBlockContents.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get deneb block")
			}
			response = genericBlock
		}
	case version.String(version.Electra):
		if isBlinded {
			jsonElectraBlock := structs.BlindedBeaconBlockElectra{}
			if err := decoder.Decode(&jsonElectraBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode blinded electra block response json")
			}
			genericBlock, err := jsonElectraBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get blinded electra block")
			}
			response = genericBlock
		} else {
			jsonElectraBlockContents := structs.BeaconBlockContentsElectra{}
			if err := decoder.Decode(&jsonElectraBlockContents); err != nil {
				return nil, errors.Wrap(err, "failed to decode electra block response json")
			}
			genericBlock, err := jsonElectraBlockContents.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get electra block")
			}
			response = genericBlock
		}
	case version.String(version.Fulu):
		if isBlinded {
			jsonFuluBlock := structs.BlindedBeaconBlockFulu{}
			if err := decoder.Decode(&jsonFuluBlock); err != nil {
				return nil, errors.Wrap(err, "failed to decode blinded fulu block response json")
			}
			genericBlock, err := jsonFuluBlock.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get blinded fulu block")
			}
			response = genericBlock
		} else {
			jsonFuluBlockContents := structs.BeaconBlockContentsFulu{}
			if err := decoder.Decode(&jsonFuluBlockContents); err != nil {
				return nil, errors.Wrap(err, "failed to decode fulu block response json")
			}
			genericBlock, err := jsonFuluBlockContents.ToGeneric()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get fulu block")
			}
			response = genericBlock
		}
	default:
		return nil, errors.Errorf("unsupported consensus version `%s`", ver)
	}
	return response, nil
}
