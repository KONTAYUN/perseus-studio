package ai

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"testing"
)

// A provider-visible tool must have the same input contract in both adapters,
// a manifest entry, and a real dispatch route. Detect drift at the boundary.
func TestProviderToolContractsMatchRuntime(t *testing.T) {
	fset := token.NewFileSet()
	dispatched := map[string]bool{}
	for _, path := range []string{"tool_registry.go", "agent_tool_loop.go"} {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if comparison, ok := node.(*ast.BinaryExpr); ok && comparison.Op == token.EQL {
				if field, ok := comparison.X.(*ast.SelectorExpr); ok && field.Sel.Name == "Name" {
					if lit, ok := comparison.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						name, _ := strconv.Unquote(lit.Value)
						dispatched[name] = true
					}
				}
			}
			if clause, ok := node.(*ast.CaseClause); ok {
				for _, expr := range clause.List {
					if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						name, _ := strconv.Unquote(lit.Value)
						dispatched[name] = true
					}
				}
			}
			return true
		})
	}
	for _, role := range []string{"admin", "user"} {
		loaded := allAgentToolsets(true)
		openai := openAIToolsForLoadedToolsets(loaded, true, role)
		anthropic := anthropicToolsForLoadedToolsets(loaded, true, role)
		for _, name := range providerVisibleToolNames(loaded, true, role) {
			left, right := toolInputSchema(name, openai), toolInputSchema(name, anthropic)
			if len(left) == 0 || !reflect.DeepEqual(left, right) {
				t.Errorf("%s/%s: missing or divergent schema", role, name)
			}
			if _, ok := toolManifestEntry(name); !ok {
				t.Errorf("missing manifest: %s", name)
			}
			if !dispatched[name] {
				t.Errorf("missing dispatch: %s", name)
			}
		}
	}
}
