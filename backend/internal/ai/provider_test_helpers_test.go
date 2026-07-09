package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func readJSONRequestPayload(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	defer r.Body.Close()

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid request payload: %v", err)
	}
	return payload
}
