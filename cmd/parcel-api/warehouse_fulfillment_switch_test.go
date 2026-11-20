package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证仓配部署开关在 parcel-api 里的接法（ADR-0375 决定四）：run 在开池之前读一次、写启动日志，值经 startupChannels
// 交到 endpointSources，特性只从 endpointSources 取。
//
// 「run 里读一次、在开池之前」是对源文件的结构断言：run 要开池才往下走，没有真库就跑不到那一步，只能对着语法树核。值从
// startupChannels 传到仓配各行是行为用例：经生产装配对真库装配，看开关为真与为假时仓配的行各答什么。开关读法与启动日志的
// 行为用例在 cmd/internal/warehouseswitch。

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

// Covers: ADR-0375 决定四、AT-WF-012（端点那一半）——开关值从 startupChannels 经 endpointSources 进到仓配的各行：经
// assembleProductionEndpoints 对真库装配，startupChannels.warehouseFulfillment 为真，打仓配的操作者行答「接入渠道未配置」（渠道配了时
// 答真渠道的答复：核验方放行、操作者册里没有这个操作者，答 OPERATOR_NOT_GRANTED）；货主行自建首方 Intake，本测试的客户发行方未设，
// 开关开着时它们仍是「接入渠道未配置」，不吃操作者渠道。为假，不论渠道配没配，各行都答 WAREHOUSE_FULFILLMENT_NOT_ENABLED。
// 零值就是关：缺省不提供仓配的业务答复。
func TestTheWarehouseFulfillmentSwitchTravelsFromStartupChannelsIntoTheWarehouseLines(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	unset := fakeGetenv(nil)
	unsetOperator, _, err := buildOperatorCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	integrationClientVerifier, _, err := buildIntegrationClientCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	customerClientVerifier, _, err := buildCustomerClientCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	assemble := func(startup startupChannels) http.Handler {
		t.Helper()
		startup.integrationClientVerifier, startup.customerClientVerifier = integrationClientVerifier, customerClientVerifier
		endpoints, err := assembleProductionEndpoints(db, startup)
		if err != nil {
			t.Fatalf("生产装配：%v", err)
		}
		return httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
	}

	cases := []struct {
		name    string
		startup startupChannels
		code    string
	}{
		{"switch on, channel unconfigured", startupChannels{warehouseFulfillment: true, operatorVerifier: unsetOperator}, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"switch on, channel configured", startupChannels{warehouseFulfillment: true, operatorVerifier: acceptingVerifier{}}, "OPERATOR_NOT_GRANTED"},
		{"switch off, channel unconfigured", startupChannels{warehouseFulfillment: false, operatorVerifier: unsetOperator}, "WAREHOUSE_FULFILLMENT_NOT_ENABLED"},
		{"switch off, channel configured", startupChannels{warehouseFulfillment: false, operatorVerifier: acceptingVerifier{}}, "WAREHOUSE_FULFILLMENT_NOT_ENABLED"},
		{"switch never set", startupChannels{operatorVerifier: acceptingVerifier{}}, "WAREHOUSE_FULFILLMENT_NOT_ENABLED"},
	}
	for _, testCase := range cases {
		router := assemble(testCase.startup)
		for _, pattern := range warehouseFulfillmentLines() {
			probe := businessEndpointProbes()[pattern]
			body := `{}`
			switch pattern {
			case receiptConfirmationsLine:
				body = receiptConfirmationBody()
			case warehouseMeasurementsLine:
				body = warehouseMeasurementBody()
			case unclaimedArrivalRegLine:
				body = unclaimedArrivalBody()
			}
			request := httptest.NewRequest(probe.method, probe.target, strings.NewReader(body))
			if pattern == receiptConfirmationsLine || pattern == warehouseMeasurementsLine || pattern == unclaimedArrivalRegLine ||
				pattern == internalMovesLine || pattern == formConversionsLine || pattern == conditionConversionsLine || pattern == shortageRegistrationsLine ||
				pattern == processingExecutionsLine || pattern == relabelExecutionsLine || pattern == unboxingsLine || pattern == countEntriesLine {
				request.Header.Set("Parcel-Work-Scope-Kind", "WAREHOUSE")
				request.Header.Set("Parcel-Work-Scope-Reference", "SYN-WH-01")
				request.Header.Set("Parcel-Device-Id", "SYN-DEVICE-NODE")
			}
			request.Header.Set("Authorization", "Bearer "+presentedOperatorToken)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			want := testCase.code
			if declaredEndpointLines()[pattern].face.family == faceCustomerClient && testCase.startup.warehouseFulfillment {
				want = "ACCESS_CHANNEL_NOT_CONFIGURED"
			}
			if response.Code != http.StatusForbidden || problemCode(t, response) != want {
				t.Fatalf("%s：%s 答 %d %s，want 403 %s", testCase.name, pattern, response.Code, response.Body.String(), want)
			}
			assertNoOutcome(t, response, pattern)
		}
	}
	// 同一份生产装配里别的行与开关无关：一行不属于仓配的登记写行，两种开关下都从「接入渠道未配置」起步。
	for _, on := range []bool{false, true} {
		router := assemble(startupChannels{warehouseFulfillment: on, operatorVerifier: unsetOperator})
		request := httptest.NewRequest(http.MethodPost, "/commercial-registration-number-type-registrations", strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+presentedOperatorToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if !leftUnconfigured(t, response) {
			t.Fatalf("开关 %t：别的行答 %d %s，want 「接入渠道未配置」", on, response.Code, response.Body.String())
		}
	}
}
