package ssz_static

import (
	"encoding/hex"
	"errors"
	"path"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/golang/snappy"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// RunSSZStaticTests executes "ssz_static" tests for the given fork of phase using the provided
// unmarshaller to hydrate serialized test data into go struct pointers and also applies any custom
// HTR methods via the customHTR callback.
func RunSSZStaticTests(t *testing.T, config, forkOrPhase string, unmarshaller Unmarshaller, customHtr CustomHTRAdder) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, _ := utils.TestFolders(t, config, forkOrPhase, "ssz_static")

	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s", config, forkOrPhase)
	}

	for _, folder := range testFolders {
		modePath := path.Join("ssz_static", folder.Name())
		modeFolders, _ := utils.TestFolders(t, config, forkOrPhase, modePath)

		if len(modeFolders) == 0 {
			t.Fatalf("No test folders found for %s/%s/%s", config, forkOrPhase, folder.Name())
		}

		for _, modeFolder := range modeFolders {
			innerPath := path.Join(modePath, modeFolder.Name())
			innerTestFolders, innerTestsFolderPath := utils.TestFolders(t, config, forkOrPhase, innerPath)

			if len(innerTestFolders) == 0 {
				t.Fatalf("No test folders found for %s/%s/%s/%s", config, forkOrPhase, folder.Name(), modeFolder.Name())
			}

			for _, innerFolder := range innerTestFolders {
				t.Run(path.Join(modeFolder.Name(), folder.Name(), innerFolder.Name()), func(t *testing.T) {
					serializedBytes, err := util.BazelFileBytes(innerTestsFolderPath, innerFolder.Name(), "serialized.ssz_snappy")
					require.NoError(t, err)
					serializedSSZ, err := snappy.Decode(nil /* dst */, serializedBytes)
					require.NoError(t, err, "Failed to decompress")
					object, err := unmarshaller(t, serializedSSZ, folder.Name())
					require.NoError(t, err, "Could not unmarshall serialized SSZ")

					rootsYamlFile, err := util.BazelFileBytes(innerTestsFolderPath, innerFolder.Name(), "roots.yaml")
					require.NoError(t, err)
					rootsYaml := &SSZRoots{}
					require.NoError(t, utils.UnmarshalYaml(rootsYamlFile, rootsYaml), "Failed to Unmarshal")

					// All types support generated code, but may also include a custom HTR method.
					var htrs []HTR
					htrs = append(htrs, func(s any) ([32]byte, error) {
						sszObj, ok := s.(ssz.HashRoot)
						if !ok {
							return [32]byte{}, errors.New("could not get hash root, not compatible object")
						}
						return sszObj.HashTreeRoot()
					})

					// Apply custom HTR methods, if any.
					if customHtr != nil {
						htrs = customHtr(t, htrs, object)
					}

					if len(htrs) == 0 {
						t.Fatal("no HTRs to run")
					}

					for i, htr := range htrs {
						var testName string
						if i == 0 { // First HTR test is generated code.
							testName = "generated"
						} else {
							testName = "custom"
						}
						t.Run(testName, func(t *testing.T) {
							root, err := htr(object)
							require.NoError(t, err)
							rootBytes, err := hex.DecodeString(rootsYaml.Root[2:])
							require.NoError(t, err)
							expected := bytesutil.ToBytes32(rootBytes)
							require.Equal(t, expected, root, "Did not receive expected hash tree root")

							if rootsYaml.SigningRoot == "" {
								return
							}

							var signingRoot [32]byte
							if v, ok := object.(ssz.HashRoot); ok {
								signingRoot, err = v.HashTreeRoot()
							} else {
								t.Fatal("object does not meet ssz.HashRoot")
							}

							require.NoError(t, err)
							signingRootBytes, err := hex.DecodeString(rootsYaml.SigningRoot[2:])
							require.NoError(t, err)
							require.DeepEqual(t, signingRootBytes, signingRoot[:], "Did not receive expected signing root")
						})
					}
				})
			}
		}
	}
}
