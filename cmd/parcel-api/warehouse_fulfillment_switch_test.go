package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 本文件证仓配部署开关在 parcel-api 里的接法（ADR-0375 决定四）：run 在开池之前读一次、写启动日志，值经 startupChannels
// 交到 endpointSources，特性只从 endpointSources 取。
//
// 这几条是对源文件的结构断言，不是行为断言：run 要开池才往下走，没有真库就跑不到那一步，而开关今天没有任何端点消费，
// 也就没有可观察的答复。开关读法与启动日志的行为用例在 cmd/internal/warehouseswitch。

const warehouseSwitchPackage = "warehouseswitch"

func parseProcessFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("解析 %s：%v", path, err)
	}
	return file
}

func processSourceFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, path := range matches {
		if !strings.HasSuffix(path, "_test.go") {
			sources = append(sources, path)
		}
	}
	if len(sources) == 0 {
		t.Fatal("没找到 parcel-api 的非测试源文件；本用例的扫描面已经失效")
	}
	return sources
}

func functionBody(t *testing.T, file *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, decl := range file.Decls {
		if function, isFunction := decl.(*ast.FuncDecl); isFunction && function.Recv == nil && function.Name.Name == name {
			return function.Body
		}
	}
	t.Fatalf("没找到函数 %s", name)
	return nil
}

func isCallOf(call *ast.CallExpr, qualifier, name string) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != name {
		return false
	}
	identifier, isIdent := selector.X.(*ast.Ident)
	return isIdent && identifier.Name == qualifier
}

func callsIn(node ast.Node, matches func(*ast.CallExpr) bool) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(node, func(visited ast.Node) bool {
		if call, isCall := visited.(*ast.CallExpr); isCall && matches(call) {
			found = append(found, call)
		}
		return true
	})
	return found
}

func isIdentifierNamed(expression ast.Expr, name string) bool {
	identifier, isIdent := expression.(*ast.Ident)
	return isIdent && identifier.Name == name
}

// literalFieldValue 取函数体里某个具名类型的复合字面量里某个字段键的值；找不到字面量或字段键都当场判红。
func literalFieldValue(t *testing.T, body *ast.BlockStmt, typeName, field string) ast.Expr {
	t.Helper()
	var literal *ast.CompositeLit
	ast.Inspect(body, func(visited ast.Node) bool {
		if candidate, isLiteral := visited.(*ast.CompositeLit); isLiteral && literal == nil && isIdentifierNamed(candidate.Type, typeName) {
			literal = candidate
		}
		return literal == nil
	})
	if literal == nil {
		t.Fatalf("函数体里没有 %s 的复合字面量", typeName)
	}
	for _, element := range literal.Elts {
		if pair, isPair := element.(*ast.KeyValueExpr); isPair && isIdentifierNamed(pair.Key, field) {
			return pair.Value
		}
	}
	t.Fatalf("%s 的字面量里没有字段 %s", typeName, field)
	return nil
}

// Covers: ADR-0375 决定四「各进程在启动时读一次、带启动日志」——整个 parcel-api 只有 run 里读一次（特性的 build 从
// endpointSources 取，不另读环境）；启动日志用读出的那个值写，读与写都排在开池之前。
func TestRunReadsTheWarehouseFulfillmentSwitchOnceAndLogsItBeforeOpeningThePool(t *testing.T) {
	isEnabledCall := func(call *ast.CallExpr) bool { return isCallOf(call, warehouseSwitchPackage, "Enabled") }

	reads := 0
	for _, path := range processSourceFiles(t) {
		reads += len(callsIn(parseProcessFile(t, path), isEnabledCall))
	}
	if reads != 1 {
		t.Fatalf("warehouseswitch.Enabled 在 parcel-api 的非测试源里被调用 %d 次，want 1：开关读一次，别处从 endpointSources 取", reads)
	}

	run := functionBody(t, parseProcessFile(t, "main.go"), "run")
	enabled := callsIn(run, isEnabledCall)
	if len(enabled) != 1 {
		t.Fatalf("读开关的那一次不在 run 里：run 内调用 %d 次", len(enabled))
	}
	if len(enabled[0].Args) != 1 || !isSelectorOf(enabled[0].Args[0], "os", "Getenv") {
		t.Fatal("开关该以 os.Getenv 为读法的入参，别的来源不是部署环境")
	}

	// 读出的值落在哪个变量上，启动日志与 startupChannels 就该用哪个变量。
	var variable string
	ast.Inspect(run, func(visited ast.Node) bool {
		assignment, isAssignment := visited.(*ast.AssignStmt)
		if !isAssignment || len(assignment.Rhs) != 1 || assignment.Rhs[0] != ast.Expr(enabled[0]) || len(assignment.Lhs) != 1 {
			return true
		}
		if identifier, isIdent := assignment.Lhs[0].(*ast.Ident); isIdent {
			variable = identifier.Name
		}
		return false
	})
	if variable == "" {
		t.Fatal("读出的开关值没有落进一个变量；启动日志与 startupChannels 就没法用同一个值")
	}

	logs := callsIn(run, func(call *ast.CallExpr) bool { return isCallOf(call, warehouseSwitchPackage, "LogStartup") })
	if len(logs) != 1 {
		t.Fatalf("run 里写仓配开关启动日志 %d 次，want 1", len(logs))
	}
	if len(logs[0].Args) != 2 || !isIdentifierNamed(logs[0].Args[0], "logger") || !isIdentifierNamed(logs[0].Args[1], variable) {
		t.Fatalf("启动日志该用 run 的 logger 与读出的值 %s", variable)
	}

	opens := callsIn(run, func(call *ast.CallExpr) bool { return isIdentifierNamed(call.Fun, "openDatabase") })
	if len(opens) != 1 {
		t.Fatalf("run 里开池 %d 次，本用例认不出开池那一步", len(opens))
	}
	if !(enabled[0].Pos() < logs[0].Pos() && logs[0].Pos() < opens[0].Pos()) {
		t.Fatal("读开关与写启动日志都要在开池之前：部署参数不合规或没配，要在连库之前就留下痕迹")
	}

	channels := literalFieldValue(t, run, "startupChannels", "warehouseFulfillment")
	if !isIdentifierNamed(channels, variable) {
		t.Fatalf("startupChannels.warehouseFulfillment 该取读出的值 %s", variable)
	}
}

func isSelectorOf(expression ast.Expr, qualifier, name string) bool {
	selector, isSelector := expression.(*ast.SelectorExpr)
	return isSelector && selector.Sel.Name == name && isIdentifierNamed(selector.X, qualifier)
}

// Covers: ADR-0375 决定四——值从 startupChannels 原样进 endpointSources，两处都是布尔、缺省为关；生产装配里 endpointSources
// 那一格取的就是 startup 的那一格，没有接到别的值上。
func TestTheWarehouseFulfillmentSwitchTravelsFromStartupChannelsIntoEndpointSources(t *testing.T) {
	for name, structure := range map[string]reflect.Type{
		"startupChannels": reflect.TypeOf(startupChannels{}),
		"endpointSources": reflect.TypeOf(endpointSources{}),
	} {
		field, found := structure.FieldByName("warehouseFulfillment")
		if !found {
			t.Fatalf("%s 没有 warehouseFulfillment 这一格", name)
		}
		if field.Type.Kind() != reflect.Bool {
			t.Fatalf("%s.warehouseFulfillment 是 %s，want bool", name, field.Type)
		}
	}
	if (startupChannels{}).warehouseFulfillment || (endpointSources{}).warehouseFulfillment {
		t.Fatal("零值读成了开：缺省必须是关")
	}

	assemble := functionBody(t, parseProcessFile(t, "main.go"), "assembleProductionEndpoints")
	value := literalFieldValue(t, assemble, "endpointSources", "warehouseFulfillment")
	if !isSelectorOf(value, "startup", "warehouseFulfillment") {
		t.Fatal("endpointSources.warehouseFulfillment 该取 startup.warehouseFulfillment")
	}
}
