package config

import (
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
)

// GetDepositContract retrieves deposit contract address and genesis fork version.
func GetDepositContract(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "config.GetDepositContract")
	defer span.End()

	httputil.WriteJson(w, &structs.GetDepositContractResponse{
		Data: &structs.DepositContractData{
			ChainId: strconv.FormatUint(params.BeaconConfig().DepositChainID, 10),
			Address: params.BeaconConfig().DepositContractAddress,
		},
	})
}

// GetForkSchedule retrieve all scheduled upcoming forks this node is aware of.
func GetForkSchedule(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "config.GetForkSchedule")
	defer span.End()

	schedule := params.SortedForkSchedule()
	data := make([]*structs.Fork, 0, len(schedule))
	if len(schedule) == 0 {
		httputil.WriteJson(w, &structs.GetForkScheduleResponse{
			Data: data,
		})
	}
	previous := schedule[0]
	for _, entry := range schedule {
		data = append(data, &structs.Fork{
			PreviousVersion: hexutil.Encode(previous.ForkVersion[:]),
			CurrentVersion:  hexutil.Encode(entry.ForkVersion[:]),
			Epoch:           fmt.Sprintf("%d", entry.Epoch),
		})
		previous = entry
	}

	httputil.WriteJson(w, &structs.GetForkScheduleResponse{
		Data: data,
	})
}

// GetSpec retrieves specification configuration (without Phase 1 params) used on this node. Specification params list
// Values are returned with following format:
// - any value starting with 0x in the spec is returned as a hex string.
// - all other values are returned as number.
func GetSpec(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "config.GetSpec")
	defer span.End()

	data, err := prepareConfigSpec()
	if err != nil {
		httputil.HandleError(w, "Could not prepare config spec: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetSpecResponse{Data: data})
}

func convertValueForJSON(v reflect.Value, tag string) interface{} {
	// Unwrap pointers / interfaces
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	// ===== Single byte → 0xAB =====
	case reflect.Uint8:
		return hexutil.Encode([]byte{uint8(v.Uint())})

	// ===== Other unsigned numbers → "123" =====
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)

	// ===== Signed numbers → "123" =====
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)

	// ===== Raw bytes – encode to hex =====
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return hexutil.Encode(v.Bytes())
		}
		fallthrough
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			// Need a copy because v.Slice is illegal on arrays directly
			tmp := make([]byte, v.Len())
			reflect.Copy(reflect.ValueOf(tmp), v)
			return hexutil.Encode(tmp)
		}
		// Generic slice/array handling
		n := v.Len()
		out := make([]interface{}, n)
		for i := 0; i < n; i++ {
			out[i] = convertValueForJSON(v.Index(i), tag)
		}
		return out

	// ===== Struct =====
	case reflect.Struct:
		t := v.Type()
		m := make(map[string]interface{}, v.NumField())
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !v.Field(i).CanInterface() {
				continue // unexported
			}
			jsonTag := f.Tag.Get("json")
			if jsonTag == "-" {
				continue // skip fields with json:"-"
			}

			// Parse JSON tag options (e.g., "fieldname,omitempty")
			parts := strings.Split(jsonTag, ",")
			key := parts[0]
			hasOmitEmpty := len(parts) > 1 && strings.Contains(strings.Join(parts[1:], ","), "omitempty")

			if key == "" {
				key = f.Name
			}

			// Check if field should be omitted before conversion
			if hasOmitEmpty && isEmptyValueReflect(v.Field(i)) {
				continue
			}

			fieldValue := convertValueForJSON(v.Field(i), tag)
			m[key] = fieldValue
		}
		return m

	// ===== String =====
	case reflect.String:
		return v.String()

	// ===== Default =====
	default:
		log.WithFields(log.Fields{
			"fn":   "prepareConfigSpec",
			"tag":  tag,
			"kind": v.Kind().String(),
			"type": v.Type().String(),
		}).Error("Unsupported config field kind; value forwarded verbatim")
		return v.Interface()
	}
}

// isEmptyValueReflect checks if a reflect.Value should be considered empty for omitempty
func isEmptyValueReflect(v reflect.Value) bool {
	// Unwrap pointers / interfaces
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.Map:
		return v.Len() == 0
	}
	return false
}

// isEmptyValue checks if a value should be considered empty for omitempty
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	default:
		// For numbers, check if they're zero
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return rv.Int() == 0
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return rv.Uint() == 0
		case reflect.Float32, reflect.Float64:
			return rv.Float() == 0
		case reflect.Bool:
			return !rv.Bool()
		case reflect.Slice, reflect.Array:
			return rv.Len() == 0
		case reflect.Map:
			return rv.Len() == 0
		case reflect.Ptr, reflect.Interface:
			return rv.IsNil()
		}
	}
	return false
}

func prepareConfigSpec() (map[string]interface{}, error) {
	data := make(map[string]interface{})
	config := *params.BeaconConfig()

	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)

	for i := 0; i < t.NumField(); i++ {
		tField := t.Field(i)
		_, isSpec := tField.Tag.Lookup("spec")
		if !isSpec {
			continue
		}
		if shouldSkip(tField) {
			continue
		}

		tag := strings.ToUpper(tField.Tag.Get("yaml"))
		val := v.Field(i)
		data[tag] = convertValueForJSON(val, tag)
	}

	return data, nil
}

func shouldSkip(tField reflect.StructField) bool {
	// Dynamically skip blob schedule if Fulu is not yet scheduled.
	if params.BeaconConfig().FuluForkEpoch == math.MaxUint64 &&
		tField.Type == reflect.TypeOf(params.BeaconConfig().BlobSchedule) {
		return true
	}
	return false
}
