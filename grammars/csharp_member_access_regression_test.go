package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func findFirstNamedDescendantWhere(node *gotreesitter.Node, lang *gotreesitter.Language, typ string, pred func(*gotreesitter.Node) bool) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.IsNamed() && node.Type(lang) == typ && pred(node) {
		return node
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if found := findFirstNamedDescendantWhere(node.NamedChild(i), lang, typ, pred); found != nil {
			return found
		}
	}
	return nil
}

func assertCSharpReadToEndMemberAccessShape(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, src []byte) {
	t.Helper()

	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected parse without syntax errors, got %s", root.SExpr(lang))
	}

	invocation := findFirstNamedDescendantWhere(root, lang, "invocation_expression", func(node *gotreesitter.Node) bool {
		return strings.Contains(node.Text(src), "process.StandardOutput.ReadToEnd()")
	})
	if invocation == nil {
		t.Fatalf("missing ReadToEnd invocation in tree: %s", root.SExpr(lang))
	}

	function := invocation.ChildByFieldName("function", lang)
	if function == nil {
		t.Fatalf("invocation missing function field: %s", invocation.SExpr(lang))
	}
	if got := function.Type(lang); got != "member_access_expression" {
		t.Fatalf("function type = %q, want member_access_expression: %s", got, invocation.SExpr(lang))
	}

	expression := function.ChildByFieldName("expression", lang)
	if expression == nil {
		t.Fatalf("member access missing expression field: %s", function.SExpr(lang))
	}
	if got := expression.Type(lang); got != "member_access_expression" {
		t.Fatalf("expression type = %q, want member_access_expression: %s", got, function.SExpr(lang))
	}
	if got := expression.Text(src); got != "process.StandardOutput" {
		t.Fatalf("expression text = %q, want %q", got, "process.StandardOutput")
	}
}

func assertCSharpInvocationStatementShape(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, src []byte, targetCall string) {
	t.Helper()

	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected parse without syntax errors, got %s", root.SExpr(lang))
	}

	badDecl := findFirstNamedDescendantWhere(root, lang, "local_declaration_statement", func(node *gotreesitter.Node) bool {
		return strings.Contains(node.Text(src), targetCall)
	})
	if badDecl != nil {
		t.Fatalf("target call parsed as local declaration unexpectedly: %s", badDecl.SExpr(lang))
	}

	stmt := findFirstNamedDescendantWhere(root, lang, "expression_statement", func(node *gotreesitter.Node) bool {
		return strings.Contains(node.Text(src), targetCall)
	})
	if stmt == nil {
		t.Fatalf("missing expression_statement for %q in tree: %s", targetCall, root.SExpr(lang))
	}

	invocation := stmt.NamedChild(0)
	if invocation == nil {
		t.Fatalf("expression_statement missing invocation child: %s", stmt.SExpr(lang))
	}
	if got := invocation.Type(lang); got != "invocation_expression" {
		t.Fatalf("expression_statement child type = %q, want invocation_expression: %s", got, stmt.SExpr(lang))
	}

	function := invocation.ChildByFieldName("function", lang)
	if function == nil {
		t.Fatalf("invocation missing function field: %s", invocation.SExpr(lang))
	}
	if got := function.Type(lang); got != "member_access_expression" {
		t.Fatalf("function type = %q, want member_access_expression: %s", got, invocation.SExpr(lang))
	}
	if got := function.Text(src); got != "newLines.Add" {
		t.Fatalf("function text = %q, want %q", got, "newLines.Add")
	}

	expression := function.ChildByFieldName("expression", lang)
	if expression == nil {
		t.Fatalf("member access missing expression field: %s", function.SExpr(lang))
	}
	if got := expression.Type(lang); got != "identifier" {
		t.Fatalf("expression type = %q, want identifier: %s", got, function.SExpr(lang))
	}
	if got := expression.Text(src); got != "newLines" {
		t.Fatalf("expression text = %q, want %q", got, "newLines")
	}
}

func TestCSharpMemberAccessRegression(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`using System.Diagnostics;

string GetOutput()
{
    var process = new Process();
    process.Start();
    var output = process.StandardOutput.ReadToEnd();
    return output;
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertCSharpReadToEndMemberAccessShape(t, tree, lang, src)
}

func TestCSharpMemberAccessRegressionWithTopLevelVar(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`using System.Diagnostics;

var filePath = "";

string GetOutput()
{
    var process = new Process
    {
        StartInfo = new ProcessStartInfo
        {
            Arguments = $"test --filter skip-all-corpus-tests",

        }
    };
    var output = process.StandardOutput.ReadToEnd();
    process.WaitForExit();
    return output;
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertCSharpReadToEndMemberAccessShape(t, tree, lang, src)
}

func TestCSharpInvocationStatementRegression(t *testing.T) {
	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class C
{
    void F()
    {
        newLines.Add(line);
    }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertCSharpInvocationStatementShape(t, tree, lang, src, "newLines.Add(line)")
}
