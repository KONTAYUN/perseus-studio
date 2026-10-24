package document

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Dollar display math follows micromark-extension-math's fenced block rule;
// Goldmark continues to own nesting, indentation and all surrounding blocks.
type mathFence struct {
	ast.BaseBlock
	length int
}

var kindMathFence = ast.NewNodeKind("DocumentBlockMath")

func (n *mathFence) Kind() ast.NodeKind            { return kindMathFence }
func (n *mathFence) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type mathFenceParser struct{}

func (mathFenceParser) Trigger() []byte             { return []byte{'$'} }
func (mathFenceParser) CanInterruptParagraph() bool { return true }
func (mathFenceParser) CanAcceptIndentedLine() bool { return false }
func (mathFenceParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockIndent()
	if pos < 0 || pos > 3 || pos >= len(line) {
		return nil, parser.NoChildren
	}
	count := 0
	for pos+count < len(line) && line[pos+count] == '$' {
		count++
	}
	// Inline $$formula$$ stays inline; a display opener may carry metadata,
	// but metadata itself cannot contain another dollar sign.
	if count < 2 || bytes.ContainsRune(line[pos+count:], '$') {
		return nil, parser.NoChildren
	}
	return &mathFence{length: count}, parser.NoChildren
}
func (mathFenceParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	count := 0
	for count < len(trimmed) && trimmed[count] == '$' {
		count++
	}
	if indent <= 3 && count >= node.(*mathFence).length && len(bytes.TrimSpace(trimmed[count:])) == 0 {
		reader.AdvanceToEOL()
		return parser.Close
	}
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}
func (mathFenceParser) Close(ast.Node, text.Reader, parser.Context) {}
