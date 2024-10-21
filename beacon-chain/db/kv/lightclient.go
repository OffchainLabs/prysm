package kv

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

func (s *Store) SaveLightClientUpdate(ctx context.Context, period uint64, update interfaces.LightClientUpdate) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveLightClientUpdate")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(lightClientUpdatesBucket)
		enc, err := encodeLightClientUpdate(update)
		if err != nil {
			return err
		}
		return bkt.Put(bytesutil.Uint64ToBytesBigEndian(period), enc)
	})
}

func (s *Store) LightClientUpdates(ctx context.Context, startPeriod, endPeriod uint64) (map[uint64]interfaces.LightClientUpdate, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LightClientUpdates")
	defer span.End()

	if startPeriod > endPeriod {
		return nil, fmt.Errorf("start period %d is greater than end period %d", startPeriod, endPeriod)
	}

	updates := make(map[uint64]interfaces.LightClientUpdate)
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(lightClientUpdatesBucket)
		c := bkt.Cursor()

		firstPeriodInDb, _ := c.First()
		if firstPeriodInDb == nil {
			return nil
		}

		for k, v := c.Seek(bytesutil.Uint64ToBytesBigEndian(startPeriod)); k != nil && binary.BigEndian.Uint64(k) <= endPeriod; k, v = c.Next() {
			currentPeriod := binary.BigEndian.Uint64(k)

			update, err := decodeLightClientUpdate(v)
			if err != nil {
				return err
			}
			updates[currentPeriod] = update
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return updates, err
}

func (s *Store) LightClientUpdate(ctx context.Context, period uint64) (interfaces.LightClientUpdate, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LightClientUpdate")
	defer span.End()

	var update interfaces.LightClientUpdate
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(lightClientUpdatesBucket)
		updateBytes := bkt.Get(bytesutil.Uint64ToBytesBigEndian(period))
		if updateBytes == nil {
			return nil
		}
		var err error
		update, err = decodeLightClientUpdate(updateBytes)
		return err
	})
	return update, err
}

func encodeLightClientUpdate(update interfaces.LightClientUpdate) ([]byte, error) {
	key, err := keyForLightClientUpdate(update)
	if err != nil {
		return nil, err
	}
	enc, err := update.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal light client update")
	}
	fullEnc := make([]byte, len(key)+len(enc))
	copy(fullEnc[len(key):], enc)
	return snappy.Encode(nil, fullEnc), nil
}

func decodeLightClientUpdate(enc []byte) (interfaces.LightClientUpdate, error) {
	var err error
	enc, err = snappy.Decode(nil, enc)
	if err != nil {
		return nil, errors.Wrap(err, "could not snappy decode light client update")
	}
	var m proto.Message
	switch {
	case hasAltairKey(enc):
		update := &ethpb.LightClientUpdateAltair{}
		if err := update.UnmarshalSSZ(enc[len(altairKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Altair light client update")
		}
		m = update
	case hasCapellaKey(enc):
		update := &ethpb.LightClientUpdateCapella{}
		if err := update.UnmarshalSSZ(enc[len(capellaKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Capella light client update")
		}
		m = update
	case hasDenebKey(enc):
		update := &ethpb.LightClientUpdateDeneb{}
		if err := update.UnmarshalSSZ(enc[len(denebKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Deneb light client update")
		}
		m = update
	case hasElectraKey(enc):
		update := &ethpb.LightClientUpdateElectra{}
		if err := update.UnmarshalSSZ(enc[len(electraKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Electra light client update")
		}
		m = update
	default:
		return nil, errors.New("decoding of saved light client update is unsupported")
	}
	return light_client.NewWrappedUpdate(m)
}

func keyForLightClientUpdate(update interfaces.LightClientUpdate) ([]byte, error) {
	switch v := update.Version(); v {
	case version.Electra:
		return electraKey, nil
	case version.Deneb:
		return denebKey, nil
	case version.Capella:
		return capellaKey, nil
	case version.Altair:
		return altairKey, nil
	default:
		return nil, fmt.Errorf("unsupported light client update version %s", version.String(v))
	}
}
