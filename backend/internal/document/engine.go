package document

import (
	"bytes"
	"strings"

	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/frontmatter"
)

// Goldmark owns Markdown grammar. Explicit adapters cover the same dollar math
// and closed, document-leading metadata accepted by remark. Unknown syntax is
// still accepted as data, never rejected because this renderer lacks a feature.
func documentEngine(source []byte) goldmark.Markdown {
	extensions := []goldmark.Extender{extension.GFM, extension.Footnote,
		passthrough.New(passthrough.Config{InlineDelimiters: []passthrough.Delimiters{{Open: "$$", Close: "$$"}, {Open: "$", Close: "$"}}}),
	}
	if len(metadataSource(source)) > 0 {
		extensions = append(extensions, &frontmatter.Extender{})
	}
	return goldmark.New(goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parser.WithBlockParsers(util.Prioritized(mathFenceParser{}, 650))))
}

// The upstream frontmatter parser permits unterminated and 4+ character fences.
// Restrict activation to remark-frontmatter's closed three-character form.
func metadataSource(source []byte) []byte {
	source = bytes.TrimPrefix(source, []byte("\xef\xbb\xbf"))
	lines := bytes.SplitAfter(source, []byte("\n"))
	if len(lines) < 2 {
		return nil
	}
	opening := strings.TrimRight(string(lines[0]), " \t\r\n")
	if opening != "---" && opening != "+++" {
		return nil
	}
	length := len(lines[0])
	for _, line := range lines[1:] {
		length += len(line)
		if strings.TrimRight(string(line), " \t\r\n") == opening {
			return source[:length]
		}
	}
	return nil
}
