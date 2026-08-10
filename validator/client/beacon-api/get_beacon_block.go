package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) beaconBlock(ctx context.Context, slot primitives.Slot, randaoReveal, graffiti []byte) (*ethpb.GenericBeaconBlock, error) {
	queryParams := neturl.Values{}
	queryParams.Add("randao_reveal", hexutil.Encode(randaoReveal))
	if len(graffiti) > 0 {
		queryParams.Add("graffiti", hexutil.Encode(graffiti))
	}

	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		return c.beaconBlockV4(ctx, slot, queryParams)
	}

	queryUrl := apiutil.BuildURL(fmt.Sprintf("/eth/v3/validator/blocks/%d", slot), queryParams)

	var (
		decodedData  []byte
		decodedBlock *ethpb.GenericBeaconBlock
	)

	decode := func(data []byte, header http.Header) (*ethpb.GenericBeaconBlock, error) {
		block, err := decodeBlockV3Response(data, header, queryUrl)
		if err != nil {
			return nil, fmt.Errorf("decode block v3 response: %w", err)
		}

		decodedData, decodedBlock = data, block

		return block, nil
	}

	opts := blockFreshnessOptions(ctx, decode)
	data, header, err := c.handler.GetSSZ(ctx, queryUrl, opts...)
	if err != nil {
		return nil, fmt.Errorf("get ssz: %w", err)
	}

	if decodedBlock != nil && bytes.Equal(data, decodedData) {
		return decodedBlock, nil
	}

	block, err := decodeBlockV3Response(data, header, queryUrl)
	if err != nil {
		return nil, fmt.Errorf("decode block v3 response: %w", err)
	}

	return block, nil
}

func (c *beaconApiValidatorClient) beaconBlockV4(ctx context.Context, slot primitives.Slot, queryParams neturl.Values) (*ethpb.GenericBeaconBlock, error) {
	queryParams.Set("include_payload", strconv.FormatBool(c.stateless))
	queryUrl := apiutil.BuildURL(fmt.Sprintf("/eth/v4/validator/blocks/%d", slot), queryParams)

	var (
		decodedData     []byte
		decodedBlock    *ethpb.GenericBeaconBlock
		decodedContents *ethpb.BeaconBlockContentsGloas
	)

	decode := func(data []byte, header http.Header) (*ethpb.GenericBeaconBlock, error) {
		block, contents, err := decodeBlockV4Response(data, header, queryUrl)
		if err == nil {
			decodedData, decodedBlock, decodedContents = data, block, contents
		}
		return block, err
	}

	opts := blockFreshnessOptions(ctx, decode)
	data, header, err := c.handler.GetSSZ(ctx, queryUrl, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "could not get v4 beacon block")
	}

	block, contents := decodedBlock, decodedContents
	if block == nil || !bytes.Equal(data, decodedData) {
		block, contents, err = decodeBlockV4Response(data, header, queryUrl)
		if err != nil {
			return nil, err
		}
	}

	// Cache the envelope only for the winning response.
	if c.stateless && contents != nil && contents.ExecutionPayloadEnvelope != nil {
		c.envelopeCache.Add(slot, contents.ExecutionPayloadEnvelope, contents.Blobs, contents.KzgProofs)
	}

	return block, nil
}

// decodeBlockV3Response turns a raw V3 response (SSZ or JSON) into a generic block.
func decodeBlockV3Response(data []byte, header http.Header, queryUrl string) (*ethpb.GenericBeaconBlock, error) {
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		ver, err := version.FromString(header.Get(api.VersionHeader))
		if err != nil {
			return nil, fmt.Errorf("unsupported header version %s: %w", header.Get(api.VersionHeader), err)
		}

		isBlindedRaw := header.Get(api.ExecutionPayloadBlindedHeader)
		isBlinded, err := strconv.ParseBool(isBlindedRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid execution payload blinded header %s: %w", isBlindedRaw, err)
		}

		return processBlockSSZResponse(ver, data, isBlinded)
	}

	decoder := json.NewDecoder(bytes.NewBuffer(data))
	produceBlockV3ResponseJson := structs.ProduceBlockV3Response{}
	if err := decoder.Decode(&produceBlockV3ResponseJson); err != nil {
		return nil, fmt.Errorf("failed to decode response body into json for %s: %w", queryUrl, err)
	}

	return processBlockJSONResponse(
		produceBlockV3ResponseJson.Version,
		produceBlockV3ResponseJson.ExecutionPayloadBlinded,
		json.NewDecoder(bytes.NewReader(produceBlockV3ResponseJson.Data)),
	)
}

// decodeBlockV4Response turns a raw V4 response into a generic block.
func decodeBlockV4Response(data []byte, header http.Header, queryUrl string) (*ethpb.GenericBeaconBlock, *ethpb.BeaconBlockContentsGloas, error) {
	payloadIncluded := header.Get(api.ExecutionPayloadIncludedHeader) == "true"
	isSSZ := strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType)

	// JSON is only acceptable when the response carries the block alone. The
	// full BlockContents body (block + envelope + blobs + KZG proofs) is
	// multi-MB and impractical over JSON, so payload-included responses must
	// be SSZ.
	if payloadIncluded && !isSSZ {
		return nil, nil, errors.Errorf("v4 payload-included response must be SSZ, got content-type %q", header.Get("Content-Type"))
	}

	if isSSZ {
		if payloadIncluded {
			contents := &ethpb.BeaconBlockContentsGloas{}
			if err := contents.UnmarshalSSZ(data); err != nil {
				return nil, nil, fmt.Errorf("contents unmarshal SSZ: %w", err)
			}

			return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Gloas{Gloas: contents.Block}}, contents, nil
		}

		block := &ethpb.BeaconBlockGloas{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return nil, nil, fmt.Errorf("block unmarshal SSZ: %w", err)
		}

		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Gloas{Gloas: block}}, nil, nil
	}

	// JSON, payload not included: parse the bare block.
	resp := structs.ProduceBlockV4Response{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, fmt.Errorf("json unmarshal: %w", err)
	}

	block := &structs.BeaconBlockGloas{}
	if err := json.Unmarshal(resp.Data, block); err != nil {
		return nil, nil, fmt.Errorf("json unmarshal: %w", err)
	}

	blk, err := block.ToGeneric()
	if err != nil {
		return nil, nil, fmt.Errorf("to generic: %w", err)
	}

	return blk, nil, nil
}

// sszBlockCodec defines SSZ unmarshalers for a fork's block and blinded block types.
type sszBlockCodec struct {
	unmarshalBlock   func([]byte) (*ethpb.GenericBeaconBlock, error)
	unmarshalBlinded func([]byte) (*ethpb.GenericBeaconBlock, error) // nil for Phase0/Altair
}

type sszCodecEntry struct {
	minVersion int
	codec      sszBlockCodec
}

// sszCodecs is ordered descending by version so that unknown future versions
// fall through to the latest known fork (matching the original if-cascade).
var sszCodecs = []sszCodecEntry{
	{
		minVersion: version.Fulu,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockContentsFulu{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Fulu{Fulu: block}}, nil
			},
			unmarshalBlinded: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				blindedBlock := &ethpb.BlindedBeaconBlockFulu{}
				if err := blindedBlock.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedFulu{BlindedFulu: blindedBlock}, IsBlinded: true}, nil
			},
		},
	},
	{
		minVersion: version.Electra,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockContentsElectra{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Electra{Electra: block}}, nil
			},
			unmarshalBlinded: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				blindedBlock := &ethpb.BlindedBeaconBlockElectra{}
				if err := blindedBlock.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedElectra{BlindedElectra: blindedBlock}, IsBlinded: true}, nil
			},
		},
	},
	{
		minVersion: version.Deneb,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockContentsDeneb{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Deneb{Deneb: block}}, nil
			},
			unmarshalBlinded: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				blindedBlock := &ethpb.BlindedBeaconBlockDeneb{}
				if err := blindedBlock.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedDeneb{BlindedDeneb: blindedBlock}, IsBlinded: true}, nil
			},
		},
	},
	{
		minVersion: version.Capella,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockCapella{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Capella{Capella: block}}, nil
			},
			unmarshalBlinded: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				blindedBlock := &ethpb.BlindedBeaconBlockCapella{}
				if err := blindedBlock.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedCapella{BlindedCapella: blindedBlock}, IsBlinded: true}, nil
			},
		},
	},
	{
		minVersion: version.Bellatrix,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockBellatrix{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Bellatrix{Bellatrix: block}}, nil
			},
			unmarshalBlinded: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				blindedBlock := &ethpb.BlindedBeaconBlockBellatrix{}
				if err := blindedBlock.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: blindedBlock}, IsBlinded: true}, nil
			},
		},
	},
	{
		minVersion: version.Altair,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlockAltair{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Altair{Altair: block}}, nil
			},
		},
	},
	{
		minVersion: version.Phase0,
		codec: sszBlockCodec{
			unmarshalBlock: func(data []byte) (*ethpb.GenericBeaconBlock, error) {
				block := &ethpb.BeaconBlock{}
				if err := block.UnmarshalSSZ(data); err != nil {
					return nil, err
				}
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Phase0{Phase0: block}}, nil
			},
		},
	},
}

func processBlockSSZResponse(ver int, data []byte, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	for _, entry := range sszCodecs {
		if ver >= entry.minVersion {
			if isBlinded && entry.codec.unmarshalBlinded != nil {
				return entry.codec.unmarshalBlinded(data)
			}
			return entry.codec.unmarshalBlock(data)
		}
	}
	return nil, fmt.Errorf("unsupported block version %s", version.String(ver))
}

func convertBlockToGeneric(decoder *json.Decoder, dest ethpb.GenericConverter, version string, isBlinded bool) (*ethpb.GenericBeaconBlock, error) {
	typeName := version
	if isBlinded {
		typeName = "blinded " + typeName
	}

	if err := decoder.Decode(dest); err != nil {
		return nil, errors.Wrapf(err, "failed to decode %s block response json", typeName)
	}

	genericBlock, err := dest.ToGeneric()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert %s block", typeName)
	}
	return genericBlock, nil
}

// jsonBlockTypes defines factory functions for creating block and blinded block structs for JSON decoding.
type jsonBlockTypes struct {
	newBlock   func() ethpb.GenericConverter
	newBlinded func() ethpb.GenericConverter // nil for Phase0/Altair
}

var jsonBlockFactories = map[string]jsonBlockTypes{
	version.String(version.Phase0): {
		newBlock: func() ethpb.GenericConverter { return &structs.BeaconBlock{} },
	},
	version.String(version.Altair): {
		newBlock: func() ethpb.GenericConverter { return &structs.BeaconBlockAltair{} },
	},
	version.String(version.Bellatrix): {
		newBlock:   func() ethpb.GenericConverter { return &structs.BeaconBlockBellatrix{} },
		newBlinded: func() ethpb.GenericConverter { return &structs.BlindedBeaconBlockBellatrix{} },
	},
	version.String(version.Capella): {
		newBlock:   func() ethpb.GenericConverter { return &structs.BeaconBlockCapella{} },
		newBlinded: func() ethpb.GenericConverter { return &structs.BlindedBeaconBlockCapella{} },
	},
	version.String(version.Deneb): {
		newBlock:   func() ethpb.GenericConverter { return &structs.BeaconBlockContentsDeneb{} },
		newBlinded: func() ethpb.GenericConverter { return &structs.BlindedBeaconBlockDeneb{} },
	},
	version.String(version.Electra): {
		newBlock:   func() ethpb.GenericConverter { return &structs.BeaconBlockContentsElectra{} },
		newBlinded: func() ethpb.GenericConverter { return &structs.BlindedBeaconBlockElectra{} },
	},
	version.String(version.Fulu): {
		newBlock:   func() ethpb.GenericConverter { return &structs.BeaconBlockContentsFulu{} },
		newBlinded: func() ethpb.GenericConverter { return &structs.BlindedBeaconBlockFulu{} },
	},
}

func processBlockJSONResponse(ver string, isBlinded bool, decoder *json.Decoder) (*ethpb.GenericBeaconBlock, error) {
	if decoder == nil {
		return nil, errors.New("no produce block json decoder found")
	}

	factory, ok := jsonBlockFactories[ver]
	if !ok {
		return nil, errors.Errorf("unsupported consensus version `%s`", ver)
	}
	if isBlinded && factory.newBlinded != nil {
		return convertBlockToGeneric(decoder, factory.newBlinded(), ver, true)
	}
	return convertBlockToGeneric(decoder, factory.newBlock(), ver, false)
}
