package ai

type ReadContentState string

const (
	ReadContentStateAvailable          ReadContentState = "available"
	ReadContentStateModelViewTruncated ReadContentState = "model_view_truncated"
	ReadContentStateSourceTruncated    ReadContentState = "source_truncated"
	ReadContentStateStaleAfterMutation ReadContentState = "stale_after_mutation"
	ReadContentStateMetadataOnly       ReadContentState = "metadata_only"
)
