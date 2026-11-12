package shared

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	syncmock "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestIsSyncing(t *testing.T) {
	tests := []struct {
		name         string
		headSlot     primitives.Slot
		syncDistance string
	}{
		{name: "head behind current slot", headSlot: 100, syncDistance: "10"},
		{name: "head at current slot", headSlot: 110, syncDistance: "0"},
		{name: "head ahead of current slot", headSlot: 111, syncDistance: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSlot := primitives.Slot(110)
			chainService := &mock.ChainService{Slot: &currentSlot, MockHeadSlot: &tt.headSlot, Optimistic: true}
			writer := httptest.NewRecorder()

			assert.Equal(t, true, IsSyncing(context.Background(), writer, &syncmock.Sync{IsSyncing: true}, chainService, chainService, chainService))
			assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
			resp := &httputil.DefaultJsonError{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
			_, details, ok := strings.Cut(resp.Message, " Details: ")
			require.Equal(t, true, ok)
			syncDetails := &structs.SyncDetailsContainer{}
			require.NoError(t, json.Unmarshal([]byte(details), syncDetails))
			require.NotNil(t, syncDetails.Data)
			assert.Equal(t, strconv.FormatUint(uint64(tt.headSlot), 10), syncDetails.Data.HeadSlot)
			assert.Equal(t, tt.syncDistance, syncDetails.Data.SyncDistance)
			assert.Equal(t, true, syncDetails.Data.IsSyncing)
			assert.Equal(t, true, syncDetails.Data.IsOptimistic)
		})
	}
}

func TestUintFromQuery_BuilderBoostFactor(t *testing.T) {
	type args struct {
		raw string
	}
	tests := []struct {
		name      string
		args      args
		wantRaw   string
		wantValue uint64
		wantOK    bool
	}{
		{
			name: "builder boost factor of 0 returns 0",
			args: args{
				raw: "0",
			},
			wantRaw:   "0",
			wantValue: 0,
			wantOK:    true,
		},
		{
			name:      "builder boost factor of the default, 100 returns 100",
			args:      args{raw: "100"},
			wantRaw:   "100",
			wantValue: 100,
			wantOK:    true,
		},
		{
			name:      "builder boost factor max uint64 returns max uint64",
			args:      args{raw: "18446744073709551615"},
			wantRaw:   "18446744073709551615",
			wantValue: math.MaxUint64,
			wantOK:    true,
		},
		{
			name:      "builder boost factor as a percentage returns error",
			args:      args{raw: "0.30"},
			wantRaw:   "",
			wantValue: 0,
			wantOK:    false,
		},
		{
			name:      "builder boost factor negative int returns error",
			args:      args{raw: "-100"},
			wantRaw:   "",
			wantValue: 0,
			wantOK:    false,
		},
		{
			name:      "builder boost factor max uint64 +1 returns error",
			args:      args{raw: "18446744073709551616"},
			wantRaw:   "",
			wantValue: 0,
			wantOK:    false,
		},
		{
			name:      "builder boost factor of invalid string returns error",
			args:      args{raw: "asdf"},
			wantRaw:   "",
			wantValue: 0,
			wantOK:    false,
		},
		{
			name:      "builder boost factor of number bigger than uint64 string returns error",
			args:      args{raw: "9871398721983721908372190837219837129803721983719283798217390821739081273918273918273918273981273982139812739821"},
			wantRaw:   "",
			wantValue: 0,
			wantOK:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := "builder_boost_factor"
			bbreq := httptest.NewRequest("GET", "/eth/v3/validator/blocks/{slot}?builder_boost_factor="+tt.args.raw, nil)
			w := httptest.NewRecorder()
			got, got1, got2 := UintFromQuery(w, bbreq, query, false)
			if got != tt.wantRaw {
				t.Errorf("UintFromQuery() got = %v, wantRaw %v", got, tt.wantRaw)
			}
			if got1 != tt.wantValue {
				t.Errorf("UintFromQuery() got1 = %v, wantRaw %v", got1, tt.wantValue)
			}
			if got2 != tt.wantOK {
				t.Errorf("UintFromQuery() got2 = %v, wantRaw %v", got2, tt.wantOK)
			}
		})
	}
}
