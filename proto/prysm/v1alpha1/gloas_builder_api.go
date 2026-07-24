package eth

// neutralBuilderBoostFactorSSZ is the wire sentinel for an unset boost factor.
const neutralBuilderBoostFactorSSZ = 100

const blsPubkeyLengthSSZ = 48

// BuilderPreferencesToSSZ converts inline builder preferences to the SSZ
// produce-body form. Absent optionals collapse to sentinels: min_bid 0 (no floor),
// boost 100 (neutral), all-zero pubkey (no binding).
func BuilderPreferencesToSSZ(prefs []*BuilderPreferenceV1) *ProduceBuilderEntryListV1 {
	entries := make([]*ProduceBuilderEntryV1, 0, len(prefs))
	for _, p := range prefs {
		if p == nil {
			continue
		}
		e := &ProduceBuilderEntryV1{
			Url:                []byte(p.Url),
			Request:            p.Request,
			MinBid:             p.GetMinBid(),
			BuilderBoostFactor: neutralBuilderBoostFactorSSZ,
			Pubkey:             make([]byte, blsPubkeyLengthSSZ),
		}
		if p.BuilderBoostFactor != nil {
			e.BuilderBoostFactor = p.GetBuilderBoostFactor()
		}
		if len(p.Pubkey) == blsPubkeyLengthSSZ {
			copy(e.Pubkey, p.Pubkey)
		}
		entries = append(entries, e)
	}
	return &ProduceBuilderEntryListV1{Entries: entries}
}

// BuilderPreferencesFromSSZ converts the SSZ produce-body form back to inline
// preferences; an all-zero pubkey maps to no binding.
func BuilderPreferencesFromSSZ(list *ProduceBuilderEntryListV1) []*BuilderPreferenceV1 {
	if list == nil {
		return nil
	}
	out := make([]*BuilderPreferenceV1, 0, len(list.Entries))
	for _, e := range list.Entries {
		if e == nil {
			continue
		}
		minBid := e.MinBid
		boost := e.BuilderBoostFactor
		p := &BuilderPreferenceV1{
			Url:                string(e.Url),
			Request:            e.Request,
			MinBid:             &minBid,
			BuilderBoostFactor: &boost,
		}
		if !allZeroSSZ(e.Pubkey) {
			p.Pubkey = e.Pubkey
		}
		out = append(out, p)
	}
	return out
}

func allZeroSSZ(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
