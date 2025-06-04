package api_test

import (
	"net/http"
	"net/http/httptest"
	"context"
	"encoding/json"
	"testing"
"fmt"
// ssz "github.com/prysmaticlabs/fastssz"
	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/OffchainLabs/prysm/v6/config/params"
	// "github.com/OffchainLabs/prysm/v6/encoding/ssz"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/stretchr/testify/require"

	beaconcli "github.com/OffchainLabs/prysm/v6/api/client/beacon"
	// "github.com/OffchainLabs/prysm/v6/api"
	// "github.com/OffchainLabs/prysm/v6/config/params"
	//  "github.com/OffchainLabs/prysm/v6/encoding/ssz"
 "encoding/binary"
)

type payload struct{ Value uint64 }
func (p *payload) SizeSSZ() int { return 8 }

func (p *payload) MarshalSSZTo(buf []byte) ([]byte, error) {
	if len(buf) < 8 {
		buf = make([]byte, 8)
	}
	binary.LittleEndian.PutUint64(buf, p.Value)
	return buf[:8], nil
}

func (p *payload) MarshalSSZ() ([]byte, error) {
	out := make([]byte, 8)
	return p.MarshalSSZTo(out)
}
func (p *payload) UnmarshalSSZ(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("payload SSZ: expected 8 bytes, got %d", len(data))
	}
	p.Value = binary.LittleEndian.Uint64(data)
	return nil
}
func handler(w http.ResponseWriter, r *http.Request) {
	resp:= payload{Value: 0xBEEF}
		if middleware.PreferSSZ(r) {
		// Encode SSZ; if it fails, reply 500.
    if middleware.PreferSSZ(r) {
        // One-liner: sets the header and writes SSZ.
       data, err := resp.MarshalSSZ()        // or ssz.MarshalSSZ(&resp)
		if err != nil {
			http.Error(w, "SSZ marshal failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 2) Stream bytes & set headers
		httputil.WriteSsz(w, data)            // header + body, no returned error
		return
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	
}
}

func makeBeaconServer(enc params.HTTPEncoding) *httptest.Server {
	features.Init(&features.Flags{HTTPEncoding: enc})

	h := middleware.AcceptHeaderHandler(
		[]string{api.JsonMediaType, api.OctetStreamMediaType},
	)(
		middleware.Negotiator(http.HandlerFunc(handler)),
	)
	return httptest.NewServer(h)
}
func TestRealBeacon_Client(t *testing.T){
	type pair struct {
		srv params.HTTPEncoding
		cli params.HTTPEncoding
		okssz bool

	}
		cases := []pair{
		{params.EncodingJSON,  params.EncodingSSZ,  false},
		{params.EncodingAuto,  params.EncodingSSZ,  true},
		{params.EncodingSSZ,   params.EncodingJSON, true},
	}
	for _, tc  := range cases {
		 	 	name := tc.srv.String() + "_srv__" + tc.cli.String() + "_cli"
		t.Run(name, func(t *testing.T) {
			/* --- spin server with its flag --- */
			srv := makeBeaconServer(tc.srv)
			defer srv.Close()

			/* --- build *real* Beacon client with its flag --- */
			features.Init(&features.Flags{HTTPEncoding: tc.cli})
			bc, err := beaconcli.NewClient(srv.URL, http.DefaultClient) // real constructor
			require.NoError(t, err)

			/* --- call the endpoint -------------------------- */
			var got payload
			// we use the generic GET helper of the beacon client
			// _, err = bc.Get(context.Background(), "/data", &got)
			// require.NoError(t, err)
			// require.Equal(t, uint64(0xBEEF), got.Value)

			// /* --- confirm encoding by inspecting the last response --- */
			// lastHdr := bc.LastResponseHeader() // method exposed by client
			// if tc.okSsz {
			// 	require.Equal(t, api.OctetStreamMediaType,
			// 		lastHdr.Get("Content-Type"))
			// } else {
			// 	require.Equal(t, api.JsonMediaType,
			// 		lastHdr.Get("Content-Type"))
			// }

			// /* --- decode once more directly to ensure header matched body --- */
			// if tc.okSsz {
			// 	var tmp payload
			// 	require.NoError(t, UnmarshalSSZ(bc.LastResponseBody(), &tmp))
			// 	require.Equal(t, got, tmp)
			// } else {
			// 	var tmp payload
			// 	require.NoError(t, json.Unmarshal(bc.LastResponseBody(), &tmp))
			// 	require.Equal(t, got, tmp)
			// }
			body, err := bc.Get(context.Background(), "/")
			require.NoError(t, err)

			/* --- decode body to verify negotiated codec ----------- */
			// var got payload
			if tc.okssz { // expecting SSZ
				require.NoError(t, got.UnmarshalSSZ(body))
			} else { // expecting JSON
				require.NoError(t, json.Unmarshal(body, &got))
			}
			require.Equal(t, uint64(0xdeadbeef), got.Value)
		})
	
	}
}