package document

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

//go:embed profile.json
var profileJSON []byte

const Version = "perseus-markdown-v1"
const MaxBytes = 8 << 20

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}
type Result struct {
	Profile  string         `json:"profile"`
	Category string         `json:"category"`
	Hash     string         `json:"hash"`
	Issues   []Issue        `json:"issues"`
	Nodes    map[string]int `json:"nodes"`
}
type ValidationError struct{ Result Result }

func (e *ValidationError) Error() string {
	if len(e.Result.Issues) > 0 {
		return e.Result.Issues[0].Message
	}
	return "文稿格式检查失败"
}
func Profile() json.RawMessage { return append(json.RawMessage{}, profileJSON...) }
func IsMarkdown(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown") || strings.HasSuffix(p, ".mdown")
}

func Validate(source []byte, requestedProfile string) (Result, error) {
	sum := sha256.Sum256(source)
	result := Result{Profile: Version, Category: "compatible", Hash: hex.EncodeToString(sum[:]), Issues: []Issue{}, Nodes: map[string]int{}}
	reject := func(code, message string) (Result, error) {
		result.Category = "format_issue"
		result.Issues = append(result.Issues, Issue{Code: code, Message: message})
		return result, &ValidationError{Result: result}
	}
	if requestedProfile != "" && requestedProfile != Version {
		return reject("profile_mismatch", "文稿格式版本已更新，请重新打开后保存")
	}
	if len(source) > MaxBytes {
		result.Category = "editor_failure"
		result.Issues = append(result.Issues, Issue{Code: "resource_limit", Message: "文稿超过 8 MiB 文本处理上限，请保留原稿后拆分副本"})
		return result, &ValidationError{Result: result}
	}
	if !utf8.Valid(source) {
		return reject("invalid_utf8", "文稿包含无效 UTF-8 编码，原文件未修改")
	}
	if strings.ContainsRune(string(source), 0) {
		return reject("nul_character", "文稿包含不可保存的空字符，原文件未修改")
	}
	// Markdown is permissive. A lone delimiter or unknown extension is data,
	// never proof of corruption. Goldmark owns CommonMark/GFM token boundaries.
	parser := documentEngine(source)
	parseSource := []byte(strings.TrimPrefix(string(source), "\ufeff"))
	tree := parser.Parser().Parse(text.NewReader(parseSource))
	err := ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		result.Nodes[node.Kind().String()]++
		if (node.Kind() == ast.KindHTMLBlock || node.Kind() == ast.KindRawHTML) && !isNativeHTML(node, parseSource) {
			result.Category = "preserved"
		}
		return ast.WalkContinue, nil
	})
	collectContractNodes(tree, parseSource, &result)
	if err != nil {
		return reject("parser_failure", fmt.Sprintf("无法完成文稿结构检查：%v", err))
	}
	return result, nil
}
