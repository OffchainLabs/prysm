// Package forks contains useful helpers for Ethereum consensus fork-related functionality.
package forks

import (
	"bytes"
	"sort"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// Fork given a target epoch,
// returns the active fork version during this epoch.
func Fork(
	targetEpoch primitives.Epoch,
) (*ethpb.Fork, error) {
	currentForkVersion := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)
	previousForkVersion := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)
	fSchedule := params.BeaconConfig().ForkVersionSchedule
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
