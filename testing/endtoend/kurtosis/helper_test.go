package kurtosis

import "testing"

func TestYamlToJSON(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    string
		wantErr bool
	}{
		{
			name: "scalar map sorts keys and keeps int as int",
			yaml: "foo: bar\nbaz: 1\n",
			want: `{"baz":1,"foo":"bar"}`,
		},
		{
			name: "nested map and list (ethereum-package shape)",
			yaml: "participants:\n  - cl_type: prysm\n    count: 2\nnetwork_params:\n  preset: minimal\n  seconds_per_slot: 6\n",
			want: `{"network_params":{"preset":"minimal","seconds_per_slot":6},"participants":[{"cl_type":"prysm","count":2}]}`,
		},
		{
			name: "bool and float preserved",
			yaml: "enabled: true\nratio: 1.5\n",
			want: `{"enabled":true,"ratio":1.5}`,
		},
		{
			// FAR_FUTURE_EPOCH (2^64-1) must survive without float rounding.
			name: "large uint64 keeps precision",
			yaml: "fulu_fork_epoch: 18446744073709551615\n",
			want: `{"fulu_fork_epoch":18446744073709551615}`,
		},
		{
			name:    "malformed yaml errors",
			yaml:    "foo: [unclosed\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := yamlToJSON([]byte(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (output %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("yamlToJSON mismatch:\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}
