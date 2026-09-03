package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 管理台发出的每一条业务路径都必须在 parcel-api 的端点表上（票 admin-web-audit-followups/03）。
//
// 两侧今天逐条对得上，但那是一次人工比对的结果。端点改名、前端多写一条，编译器两边都不报：
// 前端的路径是字符串，后端的路由表是另一处的字面量，中间没有任何类型把它们钉在一起。这与
// production_wiring_baseline 要治的「未接线看着像已接线」同形，只是方向反过来——**一条发向
// 不存在端点的请求，在代码里看着像有人接**，运行时才拿到一个 404，而 404 与「渠道未配置」是
// ADR-0055 专门要分开的两件事。
//
// 只断言前端 ⊆ 端点表，不反向：六个渠道 / 一线作业端 / 外部集成入口（NO 收寄、TF 交付与更正、
// 客户追踪视图、索赔受理、关务外部结果）按 ADR-0021 不在管理台，反向断言会永远红。
func TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	patterns := parcelAPIEndpointPatterns(t, filepath.Join(root, "cmd", "parcel-api"))
	consumers := adminWebPathConsumers(t, root, filepath.Join(root, "apps", "admin-web", "src"))

	for _, violation := range adminWebPathsOffTheTable(consumers, patterns) {
		t.Errorf("%s：管理台发向 %s，parcel-api 端点表无此行", violation.file, violation.path)
	}
}

// adminWebPathConsumer 是一处前端路径字面量：在哪个文件、指向哪条路径（查询串已截掉）。
type adminWebPathConsumer struct {
	file string
	path string
}

// adminWebPathLiteral 认前端发向后端的路径字面量：引号或模板串里紧跟着的 `/首段/次段…`，可选
// 带一个方法词前缀（`'GET /xxx'` 这类是页面给操作者看的说明串，漂了同样误导人，所以一并认）；
// 查询串与其后的模板插值到闭引号为止都归入第三组，比对时截掉。
//
// 路径段只认小写字母、数字与连字符——本仓端点表的命名约定就是这样（`/commercial-policies`、
// `/network-catalog-node-registrations`），带大写、下划线或正则元字符的串都不是端点路径。
var adminWebPathLiteral = regexp.MustCompile(
	"['\"`](?:(?:GET|POST|PUT|DELETE|PATCH) )?(/[a-z][a-z0-9-]*(?:/[a-z0-9-]+)*)(\\?[^'\"`]*)?['\"`]",
)

// adminWebOwnedPrefixes 是前端侧自己拥有、不发向 parcel-api 业务路由的首段：`/api` 是开发代理
// 前缀（vite.config.ts 剥掉后转发），`/oidc` 是令牌交换的同源代理，`/auth/callback` 是 OIDC 回跳。
// 三条都由前端配置拥有，不在端点表上是对的。
var adminWebOwnedPrefixes = map[string]bool{"api": true, "oidc": true, "auth": true}

// adminWebPathsIn 从一份 TS/TSX 源文本里取路径字面量。
func adminWebPathsIn(source string) []string {
	var paths []string
	for _, match := range adminWebPathLiteral.FindAllStringSubmatch(source, -1) {
		path := match[1]
		first := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0]
		if adminWebOwnedPrefixes[first] {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// adminWebPathConsumers 扫管理台源码树里的 .ts 与 .tsx，文件按仓根相对路径报。筛空即红：目录
// 搬家或改名时这条门禁不该恒绿地空过。
func adminWebPathConsumers(t *testing.T, root, srcRoot string) []adminWebPathConsumer {
	t.Helper()

	var consumers []adminWebPathConsumer
	scanned := 0
	walkErr := filepath.WalkDir(srcRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		for _, found := range adminWebPathsIn(string(source)) {
			consumers = append(consumers, adminWebPathConsumer{file: filepath.ToSlash(relative), path: found})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历 %s：%v", srcRoot, walkErr)
	}
	if scanned == 0 {
		t.Fatalf("%s 下没有 .ts/.tsx；管理台源码已搬家或改名，本条门禁会永远空过", srcRoot)
	}
	if len(consumers) == 0 {
		t.Fatalf("%s 下一条路径字面量都没认出来；要么前端换了发请求的写法，要么 adminWebPathLiteral 失配", srcRoot)
	}
	return consumers
}

// parcelAPIEndpointPatterns 从 cmd/parcel-api 的生产文件里取 `[]httpapi.BusinessEndpoint` 字面量
// 条目的 Pattern。按类型认字面量而不按函数名（照 tools/mechanism-inventory 的口径）：装配函数
// 改名、拆分、搬文件都不影响；换一种字面量类型才会认不到，而那种改动本该顺手回来改这里。
func parcelAPIEndpointPatterns(t *testing.T, cmdDir string) map[string]bool {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(cmdDir, "*.go"))
	if err != nil {
		t.Fatalf("列 %s：%v", cmdDir, err)
	}
	patterns := map[string]bool{}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		syntax, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("解析 %s：%v", path, parseErr)
		}
		for _, pattern := range endpointPatternsIn(syntax) {
			patterns[pattern] = true
		}
	}
	if len(patterns) == 0 {
		t.Fatalf("%s 下没认出任何 []httpapi.BusinessEndpoint 字面量；装配点换了形状，本条门禁会永远空过", cmdDir)
	}
	return patterns
}

// endpointPatternsIn 取一份 Go 源里所有 `[]httpapi.BusinessEndpoint{...}` 条目的 Pattern 字面值。
// 导入按路径认不按别名认：装配文件给 httpapi 起别名也照样认得到。
func endpointPatternsIn(syntax *ast.File) []string {
	httpapiAlias := ""
	for _, spec := range syntax.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil || imported != modulePath+"/internal/platform/httpapi" {
			continue
		}
		httpapiAlias = "httpapi"
		if spec.Name != nil {
			httpapiAlias = spec.Name.Name
		}
	}
	if httpapiAlias == "" {
		return nil
	}

	var patterns []string
	ast.Inspect(syntax, func(node ast.Node) bool {
		literal, isLiteral := node.(*ast.CompositeLit)
		if !isLiteral || !isBusinessEndpointSlice(literal.Type, httpapiAlias) {
			return true
		}
		for _, element := range literal.Elts {
			if pattern, ok := endpointPatternOf(element); ok {
				patterns = append(patterns, pattern)
			}
		}
		return true
	})
	return patterns
}

func isBusinessEndpointSlice(expr ast.Expr, httpapiAlias string) bool {
	array, isArray := expr.(*ast.ArrayType)
	if !isArray || array.Len != nil {
		return false
	}
	selector, isSelector := array.Elt.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "BusinessEndpoint" {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && pkg.Name == httpapiAlias
}

func endpointPatternOf(element ast.Expr) (string, bool) {
	entry, isLiteral := element.(*ast.CompositeLit)
	if !isLiteral {
		return "", false
	}
	for _, field := range entry.Elts {
		pair, isPair := field.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		key, isIdent := pair.Key.(*ast.Ident)
		if !isIdent || key.Name != "Pattern" {
			continue
		}
		value, isBasic := pair.Value.(*ast.BasicLit)
		if !isBasic || value.Kind != token.STRING {
			return "", false
		}
		pattern, err := strconv.Unquote(value.Value)
		if err != nil {
			return "", false
		}
		return pattern, true
	}
	return "", false
}

// adminWebPathsOffTheTable 交回不在端点表上的前端路径，按文件与路径排序去重，报错稳定可读。
func adminWebPathsOffTheTable(consumers []adminWebPathConsumer, patterns map[string]bool) []adminWebPathConsumer {
	seen := map[adminWebPathConsumer]bool{}
	var violations []adminWebPathConsumer
	for _, consumer := range consumers {
		if patterns[consumer.path] || seen[consumer] {
			continue
		}
		seen[consumer] = true
		violations = append(violations, consumer)
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file != violations[j].file {
			return violations[i].file < violations[j].file
		}
		return violations[i].path < violations[j].path
	})
	return violations
}

// TestTheAdminWebPathLiteralRecognisesTheShapesTheFrontendActuallyWrites 直接测识别谓词。
//
// 上一条门禁今天恒绿（两侧本就对得上），所以它绿证明不了正则认对了东西：认少了门禁照样绿，
// 而那时它守的面比看上去的窄。这里逐种写法各给一例，包括必须**不**认的那几种。
func TestTheAdminWebPathLiteralRecognisesTheShapesTheFrontendActuallyWrites(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		// 查阅函数里的单引号路径。
		`return exchangeMasterData<Body>('/commercial-service-products');`: {"/commercial-service-products"},
		// 模板串带查询与插值：查询串截掉，插值里的括号不把匹配吃到别处。
		"exchange(`/commercial-policies?kind=${encodeURIComponent(kind)}`, { method: 'GET' })": {"/commercial-policies"},
		// 命令面的多段路径。
		`post<Body>('/shipment-requests/parcel-cancellations', draft)`: {"/shipment-requests/parcel-cancellations"},
		// 给操作者看的说明串带方法词，同样要认——它漂了误导的是人。
		`endpoint: 'GET /customs-ports-paths?registry=candidate-port',`:            {"/customs-ports-paths"},
		"endpoint={`POST ${commercialRegistrationEndpoints['customer-account']}`}": nil,
		// 前端侧自己拥有的三条不认。
		`const TOKEN_URL = '/oidc/oauth2/token';`:         nil,
		`export const CALLBACK_PATH = '/auth/callback';`:  nil,
		`configureMasterDataApi({ basePrefix: '/api' });`: nil,
		// 不是路径的东西：正则、注释里的中文、带大写的标识。
		`path.replace(/^\/api/, '')`: nil,
		`'/Users/name'`:              nil,
		`'/customs_case_registers'`:  nil,
		"'/customs-ports-paths' '/route-plans?register=initial-route'": {"/customs-ports-paths", "/route-plans"},
	}

	for source, want := range cases {
		got := adminWebPathsIn(source)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("adminWebPathsIn(%q) = %v，想要 %v", source, got, want)
		}
	}
}

// TestTheAdminWebEndpointGateCanActuallyCatchAViolation 用合成的装配源与前端片段证明这条门禁
// 判得出红——一条只在恒绿仓上跑过的门禁，谁也不知道它红起来是什么样。
func TestTheAdminWebEndpointGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	assembly := `package main

import api "` + modulePath + `/internal/platform/httpapi"

func assemble() []api.BusinessEndpoint {
	return []api.BusinessEndpoint{
		{Pattern: "/pricing-price-cards", Handler: nil},
		{Pattern: "/network-catalog", Handler: nil},
	}
}
`
	syntax, err := parser.ParseFile(token.NewFileSet(), "endpoints.go", assembly, 0)
	if err != nil {
		t.Fatalf("解析合成装配源：%v", err)
	}
	patterns := map[string]bool{}
	for _, pattern := range endpointPatternsIn(syntax) {
		patterns[pattern] = true
	}
	if len(patterns) != 2 || !patterns["/pricing-price-cards"] || !patterns["/network-catalog"] {
		t.Fatalf("合成装配源里的两条端点没认全：%v", patterns)
	}

	consumers := []adminWebPathConsumer{
		{file: "apps/admin-web/src/pages/pricing/api.ts", path: "/pricing-price-cards"},
		{file: "apps/admin-web/src/pages/network/api.ts", path: "/network-catalog"},
		{file: "apps/admin-web/src/pages/network/api.ts", path: "/network-catalogue"},
		{file: "apps/admin-web/src/pages/network/api.ts", path: "/network-catalogue"},
	}
	violations := adminWebPathsOffTheTable(consumers, patterns)
	if len(violations) != 1 || violations[0].path != "/network-catalogue" {
		t.Fatalf("想要恰好一条指向 /network-catalogue 的违例（同文件同路径去重），得到 %v", violations)
	}
}
