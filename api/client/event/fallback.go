package event

// LegacyTopicFallback returns topics with any topics in the input
// that have a legacy equivalent replaced by their legacy topic.
// Boolean return value indicates whether any replacements were made.
func LegacyTopicFallback(topics []string) ([]string, bool) {
	out := make([]string, len(topics))
	replaced := false
	for i, t := range topics {
		if replacement, ok := LegacyEventTopicMapping[t]; ok {
			out[i] = replacement
			replaced = true
		} else {
			out[i] = t
		}
	}
	return out, replaced
}

// DropOptionalTopics returns topics with the optional ones removed, and whether any were dropped.
func DropOptionalTopics(topics []string) ([]string, bool) {
	out := make([]string, 0, len(topics))
	dropped := false
	for _, t := range topics {
		if OptionalEventTopics[t] {
			dropped = true
			continue
		}
		out = append(out, t)
	}
	return out, dropped
}
