package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// 生产装配把结算账户登记册交进作用域源。把这一参换回 nil，本测试变红：
// 空册与没接目录在作用域结果上都是未形成，只看结果分不出装配有没有把目录留空。
func TestAcceptanceFinancialControlDoesNotPassANilAccountDirectory(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "assemble.go", nil, 0)
	if err != nil {
		t.Fatalf("parse assemble.go: %v", err)
	}
	var found bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "acceptanceFinancialControl" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "NewPolicyBackedControlScopeSource" {
				return true
			}
			found = true
			if len(call.Args) < 2 {
				t.Fatal("NewPolicyBackedControlScopeSource 少了目录参数")
				return false
			}
			if ident, ok := call.Args[1].(*ast.Ident); ok && ident.Name == "nil" {
				t.Fatal("acceptanceFinancialControl 把结算账户目录又交回了 nil")
			}
			return true
		})
	}
	if !found {
		t.Fatal("acceptanceFinancialControl 里没有 NewPolicyBackedControlScopeSource")
	}
}
