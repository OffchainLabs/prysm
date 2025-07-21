package apiutil

import (
	"mime"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type mediaRange struct {
	mt   string  // canonicalised media‑type, e.g. "application/json"
	q    float64 // quality factor (0‑1)
	raw  string  // original string – useful for logging/debugging
	spec int     // 2=exact, 1=type/*, 0=*/*
}

// ParseAccept returns media ranges sorted by q (desc) then specificity.
func ParseAccept(header string) []mediaRange {
	if header == "" {
		return []mediaRange{{mt: "*/*", q: 1, spec: 0}}
	}

	var out []mediaRange
	for _, field := range strings.Split(header, ",") {
		field = strings.TrimSpace(field)
		mt, params, err := mime.ParseMediaType(field)
		if err != nil {
			log.WithField("error", err.Error()).Debug("Failed to parse header field")
			continue // skip malformed entry
		}

		q := 1.0
		if qs, ok := params["q"]; ok {
			v, err := strconv.ParseFloat(qs, 64)
			if err != nil || v < 0 || v > 1 {
				log.WithField("q", qs).Debug("Invalid quality factor (0-1)")
				continue // skip invalid q‑values
			}
			q = v
		}

		spec := 2
		switch {
		case mt == "*/*":
			spec = 0
		case strings.HasSuffix(mt, "/*"):
			spec = 1
		}

		out = append(out, mediaRange{mt: mt, q: q, raw: field, spec: spec})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].q == out[j].q {
			return out[i].spec > out[j].spec
		}
		return out[i].q > out[j].q
	})
	return out
}

// Matches reports whether content type is acceptable per the header.
func Matches(header, ct string) bool {
	for _, r := range ParseAccept(header) {
		switch {
		case r.q == 0:
			continue
		case r.mt == "*/*":
			return true
		case strings.HasSuffix(r.mt, "/*"):
			if strings.HasPrefix(ct, r.mt[:len(r.mt)-1]) {
				return true
			}
		case r.mt == ct:
			return true
		}
	}
	return false
}

// Negotiate selects the best server type according to the header.
// Returns the chosen type and true, or "", false when nothing matches.
func Negotiate(header string, serverTypes []string) (string, bool) {
	for _, r := range ParseAccept(header) {
		if r.q == 0 {
			continue
		}
		for _, s := range serverTypes {
			if Matches(r.mt, s) {
				return s, true
			}
		}
	}
	return "", false
}

// PrimaryAcceptMatches only checks the first accept maches
func PrimaryAcceptMatches(header, produced string) bool {
	if header == "" {
		return true
	}
	primary := strings.TrimSpace(strings.SplitN(header, ",", 2)[0])
	return Matches(primary, produced)
}
