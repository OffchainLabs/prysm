package params

import (
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

// ForkDigestUsingConfig retrieves the fork digest from the current schedule determined
// by the provided epoch.
func ForkDigestUsingConfig(epoch primitives.Epoch, cfg *BeaconChainConfig) [4]byte {
	entry := cfg.networkSchedule.forEpoch(epoch)
	return entry.ForkDigest
}

func ForkDigest(epoch primitives.Epoch) [4]byte {
	return ForkDigestUsingConfig(epoch, BeaconConfig())
}

func computeForkDataRoot(version [fieldparams.VersionLength]byte, root [32]byte) ([32]byte, error) {
	r, err := (&ethpb.ForkData{
		CurrentVersion:        version[:],
		GenesisValidatorsRoot: root[:],
	}).HashTreeRoot()
	if err != nil {
		return [32]byte{}, err
	}
	return r, nil
}

// Fork returns the fork version for the given epoch.
func Fork(epoch primitives.Epoch) (*ethpb.Fork, error) {
	cfg := BeaconConfig()
	return ForkFromConfig(cfg, epoch), nil
}

func ForkFromConfig(cfg *BeaconChainConfig, epoch primitives.Epoch) *ethpb.Fork {
	current := cfg.networkSchedule.forEpoch(epoch)
	previous := current
	if current.Epoch > 0 {
		previous = cfg.networkSchedule.forEpoch(current.Epoch - 1)
	}
	return &ethpb.Fork{
		PreviousVersion: previous.ForkVersion[:],
		CurrentVersion:  current.ForkVersion[:],
		Epoch:           current.Epoch,
	}
}

// ForkDataFromDigest performs the inverse, where it tries to determine the fork version
// and epoch from a provided digest by looping through our current fork schedule.
func ForkDataFromDigest(digest [4]byte) ([fieldparams.VersionLength]byte, primitives.Epoch, error) {
	ns := BeaconConfig().networkSchedule
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	// Look up the digest in our map of digests to fork versions and epochs.
	entry, ok := ns.byDigest[digest]
	if !ok {
		return [fieldparams.VersionLength]byte{}, 0, errors.Errorf("no fork exists for a digest of %#x", digest)
	}
	return entry.ForkVersion, entry.Epoch, nil
}

// NextForkData retrieves the next fork data according to the
// provided current epoch.
func NextForkData(epoch primitives.Epoch) ([fieldparams.VersionLength]byte, primitives.Epoch) {
	entry := BeaconConfig().networkSchedule.Next(epoch)
	return entry.ForkVersion, entry.Epoch
}

func NextNetworkScheduleEntry(epoch primitives.Epoch) NetworkScheduleEntry {
	entry := BeaconConfig().networkSchedule.Next(epoch)
	return entry
}

func SortedNetworkScheduleEntries() []NetworkScheduleEntry {
	return BeaconConfig().networkSchedule.entries
}

func SortedForkSchedule() []NetworkScheduleEntry {
	entries := BeaconConfig().networkSchedule.entries
	schedule := make([]NetworkScheduleEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.isFork {
			schedule = append(schedule, entry)
		}
	}
	return schedule
}

// LastForkEpoch returns the last valid fork epoch that exists in our
// fork schedule.
func LastForkEpoch() primitives.Epoch {
	return BeaconConfig().networkSchedule.LastFork().Epoch
}

func LastNetworkScheduleEntry() NetworkScheduleEntry {
	return BeaconConfig().networkSchedule.LastEntry()
}

func GetNetworkScheduleEntry(epoch primitives.Epoch) NetworkScheduleEntry {
	entry := BeaconConfig().networkSchedule.forEpoch(epoch)
	return entry
}

func genesisNetworkScheduleEntry() NetworkScheduleEntry {
	b := BeaconConfig()
	// TODO: note this has a zero digest, but we would never hit this fallback condition on
	// a properly initialized fork schedule.
	return NetworkScheduleEntry{Epoch: b.GenesisEpoch, isFork: true, ForkVersion: to4(b.GenesisForkVersion), VersionEnum: version.Phase0}
}
