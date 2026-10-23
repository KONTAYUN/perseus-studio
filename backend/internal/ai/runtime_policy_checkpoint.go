package ai

func cloneScopePaths(paths []string) []string {
	if paths == nil {
		return nil
	}
	return append([]string{}, paths...)
}

// Resume can narrow an explicit scope; it must never silently broaden it.
func intersectWorkspaceScopes(requested, persisted []string) []string {
	if persisted == nil {
		return cloneScopePaths(requested)
	}
	if requested == nil {
		return cloneScopePaths(persisted)
	}
	allowed := map[string]bool{}
	for _, p := range persisted {
		if clean := normalizeScopePath(p); clean != "" {
			allowed[clean] = true
		}
	}
	result := []string{}
	for _, p := range requested {
		if clean := normalizeScopePath(p); clean != "" && allowed[clean] {
			result = append(result, p)
		}
	}
	return result
}
func cloneLoopGuard(guard AgentLoopGuard) AgentLoopGuard {
	if guard.Observations != nil {
		next := map[string]string{}
		for k, v := range guard.Observations {
			next[k] = v
		}
		guard.Observations = next
	}
	return guard
}
