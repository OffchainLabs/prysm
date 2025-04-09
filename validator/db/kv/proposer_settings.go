package kv

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

// ErrNoProposerSettingsFound is an error thrown when no settings are found in bucket
var ErrNoProposerSettingsFound = errors.New("no proposer settings found in bucket")

// ProposerSettings gets the current proposer settings
func (s *Store) ProposerSettings(ctx context.Context) (*proposer.Settings, error) {
	_, span := trace.StartSpan(ctx, "validator.db.Settings")
	defer span.End()
	to := &validatorpb.ProposerSettingsPayload{}
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(proposerSettingsBucket)
		b := bkt.Get(proposerSettingsKey)
		if len(b) == 0 {
			return ErrNoProposerSettingsFound
		}
		if err := proto.Unmarshal(b, to); err != nil {
			return errors.Wrap(err, "failed to unmarshal proposer settings")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return proposer.SettingFromConsensus(to)
}

// ProposerSettingsExists returns true or false if the settings exist or not
func (s *Store) ProposerSettingsExists(ctx context.Context) (bool, error) {
	ps, err := s.ProposerSettings(ctx)
	if err != nil {
		if errors.Is(err, ErrNoProposerSettingsFound) {
			return false, nil
		}
		return false, err
	}
	if ps == nil {
		return false, nil
	}
	return true, nil
}

// SaveProposerSettings saves the entire proposer setting overriding the existing settings
func (s *Store) SaveProposerSettings(ctx context.Context, settings *proposer.Settings) error {
	_, span := trace.StartSpan(ctx, "validator.db.SaveProposerSettings")
	defer span.End()
	// nothing to save
	if !settings.ShouldBeSaved() {
		log.Warn("proposer settings are empty, nothing has been saved")
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(proposerSettingsBucket)
		m, err := proto.Marshal(settings.ToConsensus())
		if err != nil {
			return errors.Wrap(err, "failed to marshal proposer settings")
		}
		return bkt.Put(proposerSettingsKey, m)
	})
}
