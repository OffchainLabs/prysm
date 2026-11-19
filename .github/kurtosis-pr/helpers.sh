# Sourced into `/kurtosis` check scripts. $ENCLAVE names the running enclave.

# port <service> [port id]: prints the host URL of a service port.
port() { kurtosis port print "$ENCLAVE" "$1" "${2:-http}"; }

# api <service> <path>: GETs a beacon API path from a CL service.
api() { curl -sf --max-time 10 "$(port "$1")$2"; }

# head_slot <service>: prints the head slot of a CL service.
head_slot() { api "$1" /eth/v1/beacon/headers/head | jq -r .data.header.message.slot; }

# wait_slot <slot> [service]: blocks until the service (default cl-1) reaches the slot.
wait_slot() {
  local svc=${2:-$(kurtosis enclave inspect "$ENCLAVE" | awk '$2 ~ /^cl-1-/ { print $2; exit }')}
  local slot
  until slot=$(head_slot "$svc" 2>/dev/null) && [[ $slot =~ ^[0-9]+$ ]] && [ "$slot" -ge "$1" ]; do sleep 1; done
}
