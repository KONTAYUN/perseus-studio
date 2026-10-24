package document

import (
	"bytes"
	"regexp"

	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark/ast"
)

var nativeBreak = regexp.MustCompile(`(?i)^<br[ \t]*/?>$`)
var alertMarker = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\](?:\r?\n|$)`)
var platformMarker = regexp.MustCompile(`(?:^:{3,}[^\r\n]*|!?\[\[[^\]\r\n]+\]\])`)

func isNativeHTML(node ast.Node, source []byte) bool {
	if raw, ok := node.(*ast.RawHTML); ok {
		var value []byte
		for i := 0; i < raw.Segments.Len(); i++ {
			segment := raw.Segments.At(i)
			value = append(value, segment.Value(source)...)
		}
		return nativeBreak.Match(value)
	}
	if block, ok := node.(*ast.HTMLBlock); ok {
		return nativeBreak.Match(bytes.TrimSpace(block.Lines().Value(source)))
	}
	return false
}

// Contract node names are independent of each parser's internal AST names.
func collectContractNodes(tree ast.Node, source []byte, result *Result) {
	if len(metadataSource(source)) > 0 {
		result.Nodes["documentMetadata"]++
	}
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		name := map[string]string{"Paragraph": "paragraph", "TextBlock": "paragraph", "Heading": "heading", "Emphasis": "emphasis", "Strikethrough": "strike", "CodeSpan": "code", "FencedCodeBlock": "codeBlock", "CodeBlock": "codeBlock", "Blockquote": "blockquote", "ThematicBreak": "horizontalRule", "Link": "link", "AutoLink": "link", "Image": "image", "List": "list", "TaskCheckBox": "taskItem", "Table": "table", "FootnoteLink": "footnoteReference", "Footnote": "footnoteDefinition", "DocumentBlockMath": "blockMath"}[node.Kind().String()]
		if _, ok := node.(*passthrough.PassthroughInline); ok {
			name = "inlineMath"
		}
		if name != "" {
			result.Nodes[name]++
		}
		if node.Kind() == ast.KindCodeSpan || node.Kind() == ast.KindFencedCodeBlock || node.Kind() == ast.KindCodeBlock {
			return ast.WalkSkipChildren, nil
		}
		if node.Kind() == ast.KindParagraph || node.Kind() == ast.KindTextBlock {
			var prose []byte
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if value, ok := child.(*ast.Text); ok {
					prose = append(prose, value.Segment.Value(source)...)
				} else {
					prose = append(prose, 0)
				}
			}
			if platformMarker.Match(prose) {
				result.Category = "preserved"
			}
		}
		if text, ok := node.(*ast.Text); ok {
			if text.HardLineBreak() {
				result.Nodes["hardBreak"]++
			}
			// Classification of known platform islands never marks damage. Code
			// spans and escaped text retain Goldmark's own token boundaries.
			if platformMarker.Match(text.Segment.Value(source)) {
				result.Category = "preserved"
			}
		}
		if isNativeHTML(node, source) {
			result.Nodes["hardBreak"]++
		}
		if node.Kind() == ast.KindBlockquote && alertMarker.Match(node.Text(source)) {
			result.Nodes["callout"]++
		}
		return ast.WalkContinue, nil
	})
}
