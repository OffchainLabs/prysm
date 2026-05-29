package main

// AddExclusion adds an exclusion to the Configs, if they do not exist already.
func (c Configs) AddExclusion(check string, exclusions []string) {
	if c == nil {
		return
	}
	for _, e := range exclusions {
		cfg := c[check]
		if cfg.ExcludeFiles == nil {
			cfg.ExcludeFiles = make(map[string]string)
		}
		if _, ok := cfg.ExcludeFiles[e]; !ok {
			cfg.ExcludeFiles[e] = exclusionMessage
		}
		c[check] = cfg
	}
}
