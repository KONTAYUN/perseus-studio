package api

import "encoding/json"

func completionMessageMetaJSON(outcome string, index int, runID string) string {
	var meta map[string]any
	_ = json.Unmarshal([]byte(messageMetaJSON("assistant_segment", index, runID)), &meta)
	meta["outcome"] = outcome
	raw, _ := json.Marshal(meta)
	return string(raw)
}
