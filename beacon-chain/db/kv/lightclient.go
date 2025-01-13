package kv

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
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

func (s *Store) SaveLightClientBootstrap(ctx context.Context, blockRoot []byte, bootstrap interfaces.LightClientBootstrap) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveLightClientBootstrap")
	defer span.End()

	bootstrapCopy, err := light_client.NewWrappedBootstrap(proto.Clone(bootstrap.Proto()))
	if err != nil {
		return errors.Wrap(err, "could not clone light client bootstrap")
	}
	syncCommitteeHash, err := bootstrapCopy.CurrentSyncCommittee().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not hash current sync committee")
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		syncCommitteeBucket := tx.Bucket(lightClientSyncCommitteeBucket)
		syncCommitteeAlreadyExists := syncCommitteeBucket.Get(syncCommitteeHash[:]) != nil
		if !syncCommitteeAlreadyExists {
			enc, err := bootstrapCopy.CurrentSyncCommittee().MarshalSSZ()
			if err != nil {
				return errors.Wrap(err, "could not marshal current sync committee")
			}
			if err := syncCommitteeBucket.Put(syncCommitteeHash[:], enc); err != nil {
				return errors.Wrap(err, "could not save current sync committee")
			}
		}

		err = bootstrapCopy.SetCurrentSyncCommittee(createEmptySyncCommittee())
		if err != nil {
			return errors.Wrap(err, "could not set current sync committee to zero while saving")
		}

		bkt := tx.Bucket(lightClientBootstrapBucket)
		enc, err := encodeLightClientBootstrap(bootstrapCopy, syncCommitteeHash)
		if err != nil {
			return err
		}
		return bkt.Put(blockRoot, enc)
	})
}

func (s *Store) LightClientBootstrap(ctx context.Context, blockRoot []byte) (interfaces.LightClientBootstrap, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LightClientBootstrap")
	defer span.End()

	var bootstrap interfaces.LightClientBootstrap
	var syncCommitteeHash []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(lightClientBootstrapBucket)
		syncCommitteeBucket := tx.Bucket(lightClientSyncCommitteeBucket)
		enc := bkt.Get(blockRoot)
		if enc == nil {
			return nil
		}
		var err error
		bootstrap, syncCommitteeHash, err = decodeLightClientBootstrap(enc)
		if err != nil {
			return errors.Wrap(err, "could not decode light client bootstrap")
		}
		var syncCommitteeBytes = syncCommitteeBucket.Get(syncCommitteeHash)
		if syncCommitteeBytes == nil {
			return errors.New("sync committee not found")
		}
		syncCommittee := &ethpb.SyncCommittee{}
		if err := syncCommittee.UnmarshalSSZ(syncCommitteeBytes); err != nil {
			return errors.Wrap(err, "could not unmarshal sync committee")
		}
		err = bootstrap.SetCurrentSyncCommittee(syncCommittee)
		if err != nil {
			return errors.Wrap(err, "could not set current sync committee while retrieving")
		}
		return err
	})
	return bootstrap, err
}

func createEmptySyncCommittee() *ethpb.SyncCommittee {
	syncCom := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for i := 0; uint64(i) < params.BeaconConfig().SyncCommitteeSize; i++ {
		syncCom[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}

	return &ethpb.SyncCommittee{
		Pubkeys:         syncCom,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}
}

func encodeLightClientBootstrap(bootstrap interfaces.LightClientBootstrap, syncCommitteeHash [32]byte) ([]byte, error) {
	key, err := keyForLightClientUpdate(bootstrap.Version())
	if err != nil {
		return nil, err
	}
	enc, err := bootstrap.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal light client bootstrap")
	}
	fullEnc := make([]byte, len(key)+32+len(enc))
	copy(fullEnc, key)
	copy(fullEnc[len(key):len(key)+32], syncCommitteeHash[:])
	copy(fullEnc[len(key)+32:], enc)
	compressedEnc := snappy.Encode(nil, fullEnc)
	return compressedEnc, nil
}

func decodeLightClientBootstrap(enc []byte) (interfaces.LightClientBootstrap, []byte, error) {
	var err error
	enc, err = snappy.Decode(nil, enc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not snappy decode light client bootstrap")
	}
	var m proto.Message
	var syncCommitteeHash []byte
	switch {
	case hasAltairKey(enc):
		bootstrap := &ethpb.LightClientBootstrapAltair{}
		if err := bootstrap.UnmarshalSSZ(enc[len(altairKey)+32:]); err != nil {
			return nil, nil, errors.Wrap(err, "could not unmarshal Altair light client bootstrap")
		}
		m = bootstrap
		syncCommitteeHash = enc[len(altairKey) : len(altairKey)+32]
	case hasCapellaKey(enc):
		bootstrap := &ethpb.LightClientBootstrapCapella{}
		if err := bootstrap.UnmarshalSSZ(enc[len(capellaKey)+32:]); err != nil {
			return nil, nil, errors.Wrap(err, "could not unmarshal Capella light client bootstrap")
		}
		m = bootstrap
		syncCommitteeHash = enc[len(capellaKey) : len(capellaKey)+32]
	case hasDenebKey(enc):
		bootstrap := &ethpb.LightClientBootstrapDeneb{}
		if err := bootstrap.UnmarshalSSZ(enc[len(denebKey)+32:]); err != nil {
			return nil, nil, errors.Wrap(err, "could not unmarshal Deneb light client bootstrap")
		}
		m = bootstrap
		syncCommitteeHash = enc[len(denebKey) : len(denebKey)+32]
	case hasElectraKey(enc):
		bootstrap := &ethpb.LightClientBootstrapElectra{}
		if err := bootstrap.UnmarshalSSZ(enc[len(electraKey)+32:]); err != nil {
			return nil, nil, errors.Wrap(err, "could not unmarshal Electra light client bootstrap")
		}
		m = bootstrap
		syncCommitteeHash = enc[len(electraKey) : len(electraKey)+32]
	default:
		return nil, nil, errors.New("decoding of saved light client bootstrap is unsupported")
	}
	bootstrap, err := light_client.NewWrappedBootstrap(m)
	return bootstrap, syncCommitteeHash, err
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
	key, err := keyForLightClientUpdate(update.Version())
	if err != nil {
		return nil, err
	}
	enc, err := update.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal light client update")
	}
	fullEnc := make([]byte, len(key)+len(enc))
	copy(fullEnc, key)
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

func keyForLightClientUpdate(v int) ([]byte, error) {
	switch v {
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
