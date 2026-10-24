package document

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSharedDocumentContract(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/document-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID        string   `json:"id"`
		Source    string   `json:"source"`
		Preserved bool     `json:"preserved"`
		Nodes     []string `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			r, err := Validate([]byte(c.Source), Version)
			if err != nil {
				t.Fatal(err)
			}
			if (r.Category == "preserved") != c.Preserved {
				t.Fatalf("category=%s", r.Category)
			}
			if len(r.Hash) != 64 || len(r.Issues) != 0 {
				t.Fatal(r)
			}
			for _, name := range c.Nodes {
				if r.Nodes[name] == 0 {
					t.Fatalf("missing contract node %s: %v", name, r.Nodes)
				}
			}
		})
	}
}
func TestInvalidCandidatesFailWithoutTreatingPermissiveMarkdownAsDamage(t *testing.T) {
	for _, raw := range [][]byte{{0xff}, []byte("正文\x00"), []byte(strings.Repeat("x", MaxBytes+1))} {
		if _, err := Validate(raw, Version); err == nil {
			t.Fatal("accepted invalid candidate")
		}
	}
	if _, err := Validate([]byte("正文"), "future-profile"); err == nil {
		t.Fatal("accepted unknown profile")
	}
	for _, path := range []string{"a.MD", "a.markdown", "a.mdown"} {
		if !IsMarkdown(path) {
			t.Fatal(path)
		}
	}
}
