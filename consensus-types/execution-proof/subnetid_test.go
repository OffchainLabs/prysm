package executionproof

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestValidSubnetIds checks that valid IDs are created successfully.
func TestValidSubnetIds(t *testing.T) {
	for id := range EXECUTION_PROOF_SUBNET_COUNT {
		subnetId, err := NewExecutionProofSubnetId(id)
		if err != nil {
			t.Errorf("Expected valid subnet ID for %d, but got error: %v", id, err)
		}
		if subnetId.AsU8() != id {
			t.Errorf("Expected subnet ID %d, but got %d", id, subnetId.AsU8())
		}
	}
}

// TestInvalidSubnetIds checks that invalid IDs return an error.
func TestInvalidSubnetIds(t *testing.T) {
	invalidId := EXECUTION_PROOF_SUBNET_COUNT
	_, err := NewExecutionProofSubnetId(invalidId)
	if err == nil {
		t.Errorf("Expected error for invalid subnet ID %d, but got nil", invalidId)
	}

	// Test a higher value as well
	_, err = NewExecutionProofSubnetId(invalidId + 10)
	if err == nil {
		t.Errorf("Expected error for invalid subnet ID %d, but got nil", invalidId+10)
	}
}

// TestAllSubnetIds checks the 'All' function.
func TestAllSubnetIds(t *testing.T) {
	all := All()
	if len(all) != int(EXECUTION_PROOF_SUBNET_COUNT) {
		t.Errorf("Expected %d subnet IDs from All(), but got %d", EXECUTION_PROOF_SUBNET_COUNT, len(all))
	}

	for idx, subnetId := range all {
		if subnetId.AsUsize() != idx {
			t.Errorf("Expected subnet ID at index %d to be %d, but got %d", idx, idx, subnetId.AsUsize())
		}
	}
}

// TestAsUsize checks the AsUsize method.
func TestAsUsize(t *testing.T) {
	id, _ := NewExecutionProofSubnetId(5)
	if id.AsUsize() != 5 {
		t.Errorf("Expected AsUsize() to return 5, but got %d", id.AsUsize())
	}
}

// TestString checks the fmt.Stringer implementation.
func TestString(t *testing.T) {
	id, _ := NewExecutionProofSubnetId(7)
	expected := "7"
	if id.String() != expected {
		t.Errorf("Expected string '%s', but got '%s'", expected, id.String())
	}
}

// TestJSONMarshaling checks Serde equivalent.
func TestJSONMarshaling(t *testing.T) {
	id, _ := NewExecutionProofSubnetId(3)

	// Test Marshal
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	expectedJSON := `3`
	if string(data) != expectedJSON {
		t.Errorf("Expected marshaled JSON '%s', but got '%s'", expectedJSON, string(data))
	}

	// Test Unmarshal (Valid)
	var unmarshaledId ExecutionProofSubnetId
	if err := json.Unmarshal(data, &unmarshaledId); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if unmarshaledId != id {
		t.Errorf("Expected unmarshaled ID %d, but got %d", id, unmarshaledId)
	}

	// Test Unmarshal (Invalid)
	invalidJSON := `8` // Assuming EXECUTION_PROOF_SUBNET_COUNT is 8
	var invalidId ExecutionProofSubnetId
	err = json.Unmarshal([]byte(invalidJSON), &invalidId)
	if err == nil {
		t.Errorf("Expected error when unmarshaling invalid ID %s, but got nil", invalidJSON)
	}
}

// TestSSZMarshaling checks the SSZ implementation.
func TestSSZMarshaling(t *testing.T) {
	id, _ := NewExecutionProofSubnetId(5)

	// Test Marshal
	sszBytes, err := id.MarshalSSZ()
	if err != nil {
		t.Fatalf("Failed to marshal SSZ: %v", err)
	}

	expectedBytes := []byte{0x05}
	if !bytes.Equal(sszBytes, expectedBytes) {
		t.Errorf("Expected SSZ bytes %v, but got %v", expectedBytes, sszBytes)
	}

	// Test Unmarshal (Valid)
	var unmarshaledId ExecutionProofSubnetId
	err = unmarshaledId.UnmarshalSSZ(sszBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal SSZ: %v", err)
	}

	if unmarshaledId != id {
		t.Errorf("Expected unmarshaled SSZ ID %d, but got %d", id, unmarshaledId)
	}

	// Test Unmarshal (Invalid)
	invalidBytes := []byte{0x08} // Assuming EXECUTION_PROOF_SUBNET_COUNT is 8
	var invalidId ExecutionProofSubnetId
	err = invalidId.UnmarshalSSZ(invalidBytes)
	if err == nil {
		t.Errorf("Expected error when unmarshaling invalid SSZ ID %v, but got nil", invalidBytes)
	}
}