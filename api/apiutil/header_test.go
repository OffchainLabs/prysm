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
		accept   string
		produced string
		expect   bool
	}{
		{accept: "application/json;q=0.9,application/xml", produced: "application/json", expect: true},
		{accept: "application/*;q=0.2,*/*;q=0.1", produced: "application/xml", expect: true},
		{accept: "image/png,application/json", produced: "application/json", expect: false},
		{accept: "", produced: "text/plain", expect: true}, // header absent ⇒ */*
	}

	for _, tc := range tests {
		got := PrimaryAcceptMatches(tc.accept, tc.produced)
		require.Equal(t, got, tc.expect)
	}
}
