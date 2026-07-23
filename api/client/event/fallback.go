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
