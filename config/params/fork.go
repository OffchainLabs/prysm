package params

import (
	"bytes"
	"math"
	"sort"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// IsForkNextEpoch checks if an allotted fork is in the following epoch.
func IsForkNextEpoch(currentEpoch primitives.Epoch) (bool, error) {
	fSchedule := BeaconConfig().ForkVersionSchedule
	scheduledForks := SortedForkVersions(fSchedule)
	isForkEpoch := false
	for _, forkVersion := range scheduledForks {
		epoch := fSchedule[forkVersion]
		if currentEpoch+1 == epoch {
			isForkEpoch = true
			break
		}
	}
	return isForkEpoch, nil
}

// ForkDigestFromEpoch retrieves the fork digest from the current schedule determined
// by the provided epoch.
func ForkDigestFromEpoch(epoch primitives.Epoch) ([4]byte, error) {
	forkData, err := Fork(epoch)
	if err != nil {
		return [4]byte{}, err
	}
	version := bytesutil.ToBytes4(forkData.CurrentVersion)
	return computeForkDataRoot(version, BeaconConfig().GenesisValidatorsRoot)
}

// CreateForkDigest creates a fork digest from a genesis time and genesis
// validators root, utilizing the current slot to determine
// the active fork version in the node.
func CreateForkDigest(epoch primitives.Epoch) ([4]byte, error) {
	var version [4]byte
	digest, err := computeForkDataRoot(version, BeaconConfig().GenesisValidatorsRoot)
	if err != nil {
		return [4]byte{}, err
	}
	return digest, nil
}

func computeForkDataRoot(version [4]byte, root [32]byte) ([4]byte, error) {
	r, err := (&ethpb.ForkData{
		CurrentVersion:        version[:],
		GenesisValidatorsRoot: root[:],
	}).HashTreeRoot()
	if err != nil {
		return [4]byte{}, nil
	}
	return bytesutil.ToBytes4(r[:]), nil
}

// Fork given a target epoch,
// returns the active fork version during this epoch.
func Fork(
	targetEpoch primitives.Epoch,
) (*ethpb.Fork, error) {
	currentForkVersion := bytesutil.ToBytes4(BeaconConfig().GenesisForkVersion)
	previousForkVersion := bytesutil.ToBytes4(BeaconConfig().GenesisForkVersion)
	fSchedule := BeaconConfig().ForkVersionSchedule
	sortedForkVersions := SortedForkVersions(fSchedule)
	forkEpoch := primitives.Epoch(0)
	for _, forkVersion := range sortedForkVersions {
		epoch, ok := fSchedule[forkVersion]
		if !ok {
			return nil, errors.Errorf("fork version %x doesn't exist in schedule", forkVersion)
		}
		if targetEpoch >= epoch {
			previousForkVersion = currentForkVersion
			currentForkVersion = forkVersion
			forkEpoch = epoch
		}
	}
	return &ethpb.Fork{
		PreviousVersion: previousForkVersion[:],
		CurrentVersion:  currentForkVersion[:],
		Epoch:           forkEpoch,
	}, nil
}

// RetrieveForkDataFromDigest performs the inverse, where it tries to determine the fork version
// and epoch from a provided digest by looping through our current fork schedule.
func RetrieveForkDataFromDigest(digest [4]byte, genesisValidatorsRoot []byte) ([4]byte, primitives.Epoch, error) {
	fSchedule := BeaconConfig().ForkVersionSchedule
	gvr := BeaconConfig().GenesisValidatorsRoot
	for v, e := range fSchedule {
		rDigest, err := computeForkDataRoot(v, gvr)
		if err != nil {
			return [4]byte{}, 0, err
		}
		if rDigest == digest {
			return v, e, nil
		}
	}
	return [4]byte{}, 0, errors.Errorf("no fork exists for a digest of %#x", digest)
}

// NextForkData retrieves the next fork data according to the
// provided current epoch.
func NextForkData(currEpoch primitives.Epoch) ([4]byte, primitives.Epoch, error) {
	fSchedule := BeaconConfig().ForkVersionSchedule
	sortedForkVersions := SortedForkVersions(fSchedule)
	nextForkEpoch := primitives.Epoch(math.MaxUint64)
	var nextForkVersion [4]byte
	for _, forkVersion := range sortedForkVersions {
		epoch, ok := fSchedule[forkVersion]
		if !ok {
			return [4]byte{}, 0, errors.Errorf("fork version %x doesn't exist in schedule", forkVersion)
		}
		// If we get an epoch larger than out current epoch
		// we set this as our next fork epoch and exit the
		// loop.
		if epoch > currEpoch {
			nextForkEpoch = epoch
			nextForkVersion = forkVersion
			break
		}
		// In the event the retrieved epoch is less than
		// our current epoch, we mark the previous
		// fork's version as the next fork version.
		if epoch <= currEpoch {
			// The next fork version is updated to
			// always include the most current fork version.
			nextForkVersion = forkVersion
		}
	}
	return nextForkVersion, nextForkEpoch, nil
}

// SortedForkVersions sorts the provided fork schedule in ascending order
// by epoch.
func SortedForkVersions(forkSchedule map[[4]byte]primitives.Epoch) [][4]byte {
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

// LastForkEpoch returns the last valid fork epoch that exists in our
// fork schedule.
func LastForkEpoch() primitives.Epoch {
	fSchedule := BeaconConfig().ForkVersionSchedule
	sortedForkVersions := SortedForkVersions(fSchedule)
	lastValidEpoch := primitives.Epoch(0)
	numOfVersions := len(sortedForkVersions)
	for i := numOfVersions - 1; i >= 0; i-- {
		v := sortedForkVersions[i]
		fEpoch := fSchedule[v]
		if fEpoch != math.MaxUint64 {
			lastValidEpoch = fEpoch
			break
		}
	}
	return lastValidEpoch
}
