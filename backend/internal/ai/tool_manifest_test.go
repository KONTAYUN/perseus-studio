package ai

import "testing"

func TestNormalizeToolManifestUsesMetadataInsteadOfName(t *testing.T) {
	mutation := normalizeToolManifestEntry(ToolManifestEntry{
		Name:     "publish_result",
		Mutation: true,
	})
	if !mutation.Mutation || mutation.ReadOnly {
		t.Fatalf("explicit mutation metadata was not preserved: %#v", mutation)
	}

	readOnly := normalizeToolManifestEntry(ToolManifestEntry{
		Name:  "write_report_preview",
		Group: ToolGroupRead,
	})
	if readOnly.Mutation || !readOnly.ReadOnly {
		t.Fatalf("tool name must not imply mutation behavior: %#v", readOnly)
	}
}
