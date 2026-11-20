package structs

import "encoding/json"

func (s *SignedBeaconBlockPQ) MessageRawJson() ([]byte, error) {
	return json.Marshal(s.Message)
}

func (s *SignedBeaconBlockPQ) SigString() string {
	return s.Signature
}
