package apiutil

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestParseAccept(t *testing.T) {
	type want struct {
		mt   string
		q    float64
		spec int
	}

	cases := []struct {
		name   string
		header string
		want   []want
	}{
		{
			name:   "empty header becomes */*;q=1",
			header: "",
			want:   []want{{mt: "*/*", q: 1, spec: 0}},
		},
		{
			name:   "quality ordering then specificity",
			header: "application/json;q=0.2, */*;q=0.1, application/xml;q=0.5, text/*;q=0.5",
			want: []want{
				{mt: "application/xml", q: 0.5, spec: 2},
				{mt: "text/*", q: 0.5, spec: 1},
				{mt: "application/json", q: 0.2, spec: 2},
				{mt: "*/*", q: 0.1, spec: 0},
			},
		},
		{
			name:   "invalid pieces are skipped",
			header: "text/plain; q=boom, application/json",
			want:   []want{{mt: "application/json", q: 1, spec: 2}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAccept(tc.header)
			gotProjected := make([]want, len(got))
			for i, g := range got {
				gotProjected[i] = want{mt: g.mt, q: g.q, spec: g.spec}
			}
			require.DeepEqual(t, gotProjected, tc.want)
		})
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		name    string
		accept  string
		ct      string
		matches bool
	}{
		{"exact match", "application/json", "application/json", true},
		{"type wildcard", "application/*;q=0.8", "application/xml", true},
		{"global wildcard", "*/*;q=0.1", "image/png", true},
		{"explicitly unacceptable (q=0)", "text/*;q=0", "text/plain", false},
		{"no match", "image/png", "application/json", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Matches(tc.accept, tc.ct)
			require.Equal(t, tc.matches, got)
		})
	}
}

func TestNegotiate(t *testing.T) {
	cases := []struct {
		name        string
		accept      string
		serverTypes []string
		wantType    string
		ok          bool
	}{
		{
			name:        "highest quality wins",
			accept:      "application/json;q=0.8,application/xml;q=0.9",
			serverTypes: []string{"application/json", "application/xml"},
			wantType:    "application/xml",
			ok:          true,
		},
		{
			name:        "wildcard matches first server type",
			accept:      "*/*;q=0.5",
			serverTypes: []string{"application/octet-stream", "application/json"},
			wantType:    "application/octet-stream",
			ok:          true,
		},
		{
			name:        "no acceptable type",
			accept:      "image/png",
			serverTypes: []string{"application/json"},
			wantType:    "",
			ok:          false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Negotiate(tc.accept, tc.serverTypes)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.wantType, got)
		})
	}
}

func TestPrimaryAcceptMatches(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		produced string
		expect   bool
	}{
		{
			name:     "prefers json",
			accept:   "application/json;q=0.9,application/xml",
			produced: "application/json",
			expect:   true,
		},
		{
			name:     "wildcard application beats other wildcard",
			accept:   "application/*;q=0.2,*/*;q=0.1",
			produced: "application/xml",
			expect:   true,
		},
		{
			name:     "json wins",
			accept:   "application/xml;q=0.8,application/json;q=0.9",
			produced: "application/json",
			expect:   true,
		},
		{
			name:     "json loses",
			accept:   "application/xml;q=0.8,application/json;q=0.9,application/octet-stream;q=0.99",
			produced: "application/json",
			expect:   false,
		},
		{
			name:     "json wins with non q option",
			accept:   "application/xml;q=0.8,image/png,application/json;q=0.9",
			produced: "application/json",
			expect:   true,
		},
		{
			name:     "json not primary",
			accept:   "image/png,application/json",
			produced: "application/json",
			expect:   false,
		},
		{
			name:     "absent header",
			accept:   "",
			produced: "text/plain",
			expect:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PrimaryAcceptMatches(tc.accept, tc.produced)
			require.Equal(t, got, tc.expect)
		})
	}
}
