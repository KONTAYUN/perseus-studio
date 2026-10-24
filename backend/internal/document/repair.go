package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"sort"
)

// This is a conservative formatting proof, never a natural-language judgement.
// Inert raw HTML is compared in memory, never exposed as trusted preview HTML.
func RepairEquivalent(before, after []byte) bool {
	projection := func(source []byte) (string, error) {
		engine := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote), goldmark.WithRendererOptions(html.WithUnsafe()))
		ctx := parser.NewContext()
		tree := engine.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))
		var output bytes.Buffer
		if err := engine.Renderer().Render(&output, source, tree); err != nil {
			return "", err
		}
		refs := []string{}
		for _, ref := range ctx.References() {
			raw, _ := json.Marshal([]string{string(ref.Label()), string(ref.Destination()), string(ref.Title())})
			refs = append(refs, string(raw))
		}
		sort.Strings(refs)
		raw, _ := json.Marshal(refs)
		return output.String() + string(raw), nil
	}
	a, err := projection(before)
	if err != nil {
		return false
	}
	b, err := projection(after)
	return err == nil && a == b
}
func ValidateRepair(before, after []byte) (Result, error) {
	result, err := Validate(after, Version)
	if err != nil {
		return result, err
	}
	if bytes.Equal(before, after) {
		return result, fmt.Errorf("修复候选没有实际修改，请结束或提供明确建议")
	}
	return result, nil
}
