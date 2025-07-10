package config

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/forks"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

	schedule := params.BeaconConfig().ForkVersionSchedule
	if len(schedule) == 0 {
		httputil.WriteJson(w, &structs.GetForkScheduleResponse{
			Data: make([]*structs.Fork, 0),
		})
		return
	}

	versions := forks.SortedForkVersions(schedule)
	chainForks := make([]*structs.Fork, len(schedule))
	var previous, current []byte
	for i, v := range versions {
		if i == 0 {
			previous = params.BeaconConfig().GenesisForkVersion
		} else {
			previous = current
		}
		copyV := v
		current = copyV[:]
		chainForks[i] = &structs.Fork{
			PreviousVersion: hexutil.Encode(previous),
			CurrentVersion:  hexutil.Encode(current),
			Epoch:           fmt.Sprintf("%d", schedule[v]),
		}
	}

	httputil.WriteJson(w, &structs.GetForkScheduleResponse{
		Data: chainForks,
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

func prepareConfigSpec() (map[string]interface{}, error) {
	data := make(map[string]interface{})
	config := *params.BeaconConfig()
	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)

	for i := 0; i < t.NumField(); i++ {
		tField := t.Field(i)
		_, isSpecField := tField.Tag.Lookup("spec")
		if !isSpecField {
			// Field should not be returned from API.
			continue
		}

		tagValue := strings.ToUpper(tField.Tag.Get("yaml"))
		vField := v.Field(i)
		switch vField.Kind() {
		case reflect.Int:
			data[tagValue] = strconv.FormatInt(vField.Int(), 10)
		case reflect.Uint64:
			data[tagValue] = strconv.FormatUint(vField.Uint(), 10)
		case reflect.Slice:
			// Handle byte slices with hexutil.Encode
			if vField.Type().Elem().Kind() == reflect.Uint8 {
				data[tagValue] = hexutil.Encode(vField.Bytes())
			} else if vField.Type().Elem().Kind() == reflect.Struct {
				// Handle struct slices - convert numeric fields to strings for consistent JSON output
				data[tagValue] = convertStructSliceForJSON(vField)
			} else {
				// Handle other slice types - return as interface{} for JSON serialization
				data[tagValue] = vField.Interface()
			}
		case reflect.Array:
			data[tagValue] = hexutil.Encode(reflect.ValueOf(&config).Elem().Field(i).Slice(0, vField.Len()).Bytes())
		case reflect.String:
			data[tagValue] = vField.String()
		case reflect.Uint8:
			data[tagValue] = hexutil.Encode([]byte{uint8(vField.Uint())})
		default:
			return nil, fmt.Errorf("unsupported config field type: %s", vField.Kind().String())
		}
	}

	return data, nil
}

// convertStructSliceForJSON converts struct slices to ensure numeric fields are strings
func convertStructSliceForJSON(sliceValue reflect.Value) []map[string]interface{} {
	length := sliceValue.Len()
	result := make([]map[string]interface{}, length)

	for i := 0; i < length; i++ {
		elem := sliceValue.Index(i)
		elemType := elem.Type()
		elemMap := make(map[string]interface{})

		for j := 0; j < elem.NumField(); j++ {
			field := elem.Field(j)
			fieldType := elemType.Field(j)

			// Skip unexported fields
			if !field.CanInterface() {
				continue
			}

			// Get JSON tag name (fallback to field name if no JSON tag)
			jsonTag := fieldType.Tag.Get("json")
			if jsonTag == "" || jsonTag == "-" {
				jsonTag = fieldType.Name
			}

			// Convert numeric types to strings for consistent JSON output
			switch field.Kind() {
			case reflect.Uint64, reflect.Uint, reflect.Uint32, reflect.Uint16, reflect.Uint8:
				elemMap[jsonTag] = strconv.FormatUint(field.Uint(), 10)
			case reflect.Int64, reflect.Int, reflect.Int32, reflect.Int16, reflect.Int8:
				elemMap[jsonTag] = strconv.FormatInt(field.Int(), 10)
			default:
				elemMap[jsonTag] = field.Interface()
			}
		}

		result[i] = elemMap
	}

	return result
}
