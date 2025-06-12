package params

import (
	"bytes"
	"sort"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// IsForkNextEpoch checks if an allotted fork is in the following epoch.
func IsForkNextEpoch(currentEpoch primitives.Epoch) bool {
	entry, ok := BeaconConfig().networkSchedule.activatedAt(currentEpoch + 1)
	return ok && entry.isFork
}

// ForkDigestFromEpoch retrieves the fork digest from the current schedule determined
// by the provided epoch.
func ForkDigestFromEpoch(epoch primitives.Epoch) [4]byte {
	return BeaconConfig().networkSchedule.ForEpoch(epoch).ForkDigest
}

// CreateForkDigest creates a fork digest from a genesis time and genesis
// validators root, utilizing the current slot to determine
// the active fork version in the node.
func ForkDigest(epoch primitives.Epoch) [4]byte {
	return BeaconConfig().networkSchedule.ForEpoch(epoch).ForkDigest
}

func computeForkDataRoot(version [4]byte, root [32]byte) ([32]byte, error) {
	r, err := (&ethpb.ForkData{
		CurrentVersion:        version[:],
		GenesisValidatorsRoot: root[:],
	}).HashTreeRoot()
	if err != nil {
		return [32]byte{}, nil
	}
	return r, nil
}

// Fork returns the fork version for the given epoch.
func Fork(epoch primitives.Epoch) (*ethpb.Fork, error) {
	cfg := BeaconConfig()
	current := cfg.networkSchedule.ForEpoch(epoch)
	previous := current
	if current.Epoch > 0 {
		previous = BeaconConfig().networkSchedule.ForEpoch(current.Epoch - 1)
	}
	return &ethpb.Fork{
		PreviousVersion: previous.ForkVersion[:],
		CurrentVersion:  current.ForkVersion[:],
		Epoch:           current.Epoch,
	}, nil
}

// RetrieveForkDataFromDigest performs the inverse, where it tries to determine the fork version
// and epoch from a provided digest by looping through our current fork schedule.
func ForkDataFromDigest(digest [4]byte) ([4]byte, primitives.Epoch, error) {
	cfg := BeaconConfig()
	entry, ok := cfg.networkSchedule.byDigest[digest]
	if !ok {
		return [4]byte{}, 0, errors.Errorf("no fork exists for a digest of %#x", digest)
	}
	return entry.ForkVersion, entry.Epoch, nil
}

// NextForkData retrieves the next fork data according to the
// provided current epoch.
func NextForkData(epoch primitives.Epoch) ([4]byte, primitives.Epoch) {
	entry := BeaconConfig().networkSchedule.Next(epoch)
	return entry.ForkVersion, entry.Epoch
}

// SortedForkVersions sorts the provided fork schedule in ascending order
// by epoch.
func SortedForkVersions() [][4]byte {
	forkSchedule := BeaconConfig().ForkVersionSchedule
	sortedVersions := make([][4]byte, len(forkSchedule))
	i := 0
	for k := range forkSchedule {
		sortedVersions[i] = k
		i++
	}
	sort.Slice(sortedVersions, func(a, b int) bool {
		// va == "version" a, ie the [4]byte version id
		va, vb := sortedVersions[a], sortedVersions[b]
		// ea == "epoch" a, ie the types.Epoch corresponding to va
		ea, eb := forkSchedule[va], forkSchedule[vb]
		// Try to sort by epochs first, which works fine when epochs are all distinct.
		// in the case of testnets starting from a given fork, all epochs leading to the fork will be zero.
		if ea != eb {
			return ea < eb
		}
		// If the epochs are equal, break the tie with a lexicographic comparison of the fork version bytes.
		// eg 2 versions both with a fork epoch of 0, 0x00000000 would come before 0x01000000.
		// sort.Slice takes a 'less' func, ie `return a < b`, and when va < vb, bytes.Compare will return -1
		return bytes.Compare(va[:], vb[:]) < 0
	})
	return sortedVersions
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
