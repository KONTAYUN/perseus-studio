package workspace

import (
	"bytes"
	"sort"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTMLFragmentsSemanticallyEqual compares parsed fragment structure so harmless
// serializer differences cannot create a second file version.
func HTMLFragmentsSemanticallyEqual(left, right string) bool {
	return htmlFragmentFingerprint(left) == htmlFragmentFingerprint(right)
}

func htmlFragmentFingerprint(fragment string) string {
	context := &nethtml.Node{Type: nethtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := nethtml.ParseFragment(strings.NewReader(SanitizeHTMLFragment(fragment)), context)
	if err != nil {
		return "\x00" + fragment
	}
	var output bytes.Buffer
	for _, node := range nodes {
		writeHTMLNodeFingerprint(&output, node)
	}
	return output.String()
}

func writeHTMLNodeFingerprint(output *bytes.Buffer, node *nethtml.Node) {
	if node == nil {
		return
	}
	switch node.Type {
	case nethtml.TextNode:
		output.WriteString("T:")
		output.WriteString(node.Data)
	case nethtml.ElementNode:
		output.WriteString("E:")
		output.WriteString(strings.ToLower(node.Data))
		attrs := append([]nethtml.Attribute(nil), node.Attr...)
		sort.Slice(attrs, func(i, j int) bool {
			if attrs[i].Key == attrs[j].Key {
				return attrs[i].Val < attrs[j].Val
			}
			return attrs[i].Key < attrs[j].Key
		})
		for _, attr := range attrs {
			output.WriteByte('|')
			output.WriteString(strings.ToLower(attr.Key))
			output.WriteByte('=')
			output.WriteString(attr.Val)
		}
		output.WriteByte('{')
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeHTMLNodeFingerprint(output, child)
		}
		output.WriteByte('}')
	case nethtml.CommentNode:
		output.WriteString("C:")
		output.WriteString(node.Data)
	}
}
