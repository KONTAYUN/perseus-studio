package ai

func toolFailureRecoveryHint(err error) string {
	if err == nil {
		return ""
	}
	return normalizeToolError(err).RecommendedAction
}
