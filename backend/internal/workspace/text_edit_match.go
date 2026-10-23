package workspace

import (
	"fmt"
	"strings"
	"webide-backend/internal/failure"
)

type textReplacementPlan struct {
	content      string
	replacements int
	matchKind    TextEditMatchKind
}

func planTextReplacement(content, oldText, newText string, replaceAll bool) (textReplacementPlan, error) {
	if count := strings.Count(content, oldText); count > 0 {
		if !replaceAll && count > 1 {
			return textReplacementPlan{}, failure.New("workspace_exact_edit_unstable", fmt.Sprintf("old_text matched %d times; set replace_all=true or provide a more specific old_text", count), nil)
		}
		limit := 1
		if replaceAll {
			limit = -1
		}
		return textReplacementPlan{
			content:      strings.Replace(content, oldText, newText, limit),
			replacements: count,
			matchKind:    TextEditMatchExact,
		}, nil
	}
	return textReplacementPlan{}, failure.New("workspace_exact_edit_unstable", "old_text was not found; reread the file and provide exact text including whitespace", nil)
}
