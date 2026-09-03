package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGloasProgressiveStateSchema(t *testing.T) {
	schema, ok := ProgressiveStateSchemaForVersion(version.Gloas)
	require.Equal(t, true, ok)
	require.Equal(t, params.BeaconConfig().BeaconStateGloasFieldCount, len(schema.fields))
	require.Equal(t, len(schema.fields), len(schema.activeFields))
	require.Equal(t, len(schema.fields), len(schema.fieldIndex))
	require.DeepEqual(t, gloasFields, schema.fields)

	for i, field := range schema.fields {
		require.Equal(t, true, schema.activeFields[i], "field %s should be active", field.String())
		require.Equal(t, i, field.RealPosition(), "unexpected Gloas position for field %s", field.String())

		index, found := schema.GetFieldIndex(field)
		require.Equal(t, true, found, "field %s should have an index", field.String())
		require.Equal(t, i, index, "unexpected index for field %s", field.String())
	}
}

func TestNewProgressiveStateFieldsSchema(t *testing.T) {
	t.Run("MaximumFields", func(t *testing.T) {
		allFields := make([]types.FieldIndex, ssz.MaxProgressiveActiveFields)
		for i := range allFields {
			allFields[i] = types.FieldIndex(i)
		}

		schema, err := newProgressiveStateFieldsSchema(allFields, nil, len(allFields))
		require.NoError(t, err)
		require.Equal(t, ssz.MaxProgressiveActiveFields, len(schema.activeFields))
		require.Equal(t, ssz.MaxProgressiveActiveFields, len(schema.fields))
		require.Equal(t, ssz.MaxProgressiveActiveFields, len(schema.fieldIndex))
		for i, active := range schema.activeFields {
			require.Equal(t, true, active, "field position %d should be active", i)
		}
	})

	t.Run("InactiveFields", func(t *testing.T) {
		allFields := []types.FieldIndex{
			types.GenesisTime,
			types.Slot,
			types.Validators,
			types.Balances,
			types.PTCWindow,
		}
		inactiveFields := []types.FieldIndex{
			types.Slot,
			types.Validators,
		}

		schema, err := newProgressiveStateFieldsSchema(allFields, inactiveFields, 3)
		require.NoError(t, err)
		require.DeepEqual(t, []bool{true, false, false, true, true}, schema.activeFields)
		require.DeepEqual(t, []types.FieldIndex{
			types.GenesisTime,
			types.Balances,
			types.PTCWindow,
		}, schema.fields)
		require.Equal(t, 3, len(schema.fieldIndex))

		tests := []struct {
			field types.FieldIndex
			index int
			found bool
		}{
			{field: types.GenesisTime, index: 0, found: true},
			{field: types.Slot, index: 0, found: false},
			{field: types.Validators, index: 0, found: false},
			{field: types.Balances, index: 1, found: true},
			{field: types.PTCWindow, index: 2, found: true},
		}
		for _, tt := range tests {
			t.Run(tt.field.String(), func(t *testing.T) {
				index, found := schema.GetFieldIndex(tt.field)
				require.Equal(t, tt.found, found)
				require.Equal(t, tt.index, index)
			})
		}
	})

	t.Run("CopiesInputs", func(t *testing.T) {
		allFields := []types.FieldIndex{types.GenesisTime, types.Slot, types.Balances}
		inactiveFields := []types.FieldIndex{types.Slot}

		schema, err := newProgressiveStateFieldsSchema(allFields, inactiveFields, 2)
		require.NoError(t, err)

		allFields[0] = types.Fork
		inactiveFields[0] = types.Balances

		require.DeepEqual(t, []bool{true, false, true}, schema.activeFields)
		require.DeepEqual(t, []types.FieldIndex{types.GenesisTime, types.Balances}, schema.fields)
		index, found := schema.GetFieldIndex(types.GenesisTime)
		require.Equal(t, true, found)
		require.Equal(t, 0, index)
	})

	t.Run("Invalid", func(t *testing.T) {
		tooManyFields := make([]types.FieldIndex, ssz.MaxProgressiveActiveFields+1)

		tests := []struct {
			name           string
			allFields      []types.FieldIndex
			inactiveFields []types.FieldIndex
			forkFieldCount int
			want           string
		}{
			{
				name: "NoFields",
				want: "requires at least one field",
			},
			{
				name:           "TooManyFields",
				allFields:      tooManyFields,
				forkFieldCount: len(tooManyFields),
				want:           "maximum is 256",
			},
			{
				name:           "DuplicateField",
				allFields:      []types.FieldIndex{types.GenesisTime, types.Slot, types.Slot},
				forkFieldCount: 3,
				want:           "duplicate field slot",
			},
			{
				name:           "UnknownInactiveField",
				allFields:      []types.FieldIndex{types.GenesisTime, types.Slot},
				inactiveFields: []types.FieldIndex{types.Validators},
				forkFieldCount: 2,
				want:           "inactive field validators is not present",
			},
			{
				name:           "DuplicateInactiveField",
				allFields:      []types.FieldIndex{types.GenesisTime, types.Slot, types.Balances},
				inactiveFields: []types.FieldIndex{types.Slot, types.Slot},
				forkFieldCount: 2,
				want:           "duplicate inactive field slot",
			},
			{
				name:           "TrailingInactiveField",
				allFields:      []types.FieldIndex{types.GenesisTime, types.Slot},
				inactiveFields: []types.FieldIndex{types.Slot},
				forkFieldCount: 1,
				want:           "must not end with an inactive field",
			},
			{
				name:           "FieldCountMismatch",
				allFields:      []types.FieldIndex{types.GenesisTime, types.Slot},
				forkFieldCount: 1,
				want:           "fields count does not match expected count: got 2, expected 1",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				schema, err := newProgressiveStateFieldsSchema(tt.allFields, tt.inactiveFields, tt.forkFieldCount)
				require.IsNil(t, schema)
				require.ErrorContains(t, tt.want, err)
			})
		}
	})
}

func TestProgressiveStateSchemaForVersion_Unsupported(t *testing.T) {
	schema, ok := ProgressiveStateSchemaForVersion(version.Gloas + 1)
	require.Equal(t, false, ok)
	require.IsNil(t, schema)
}
