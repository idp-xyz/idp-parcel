package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// 接线面清点数的是「装了多少」，不是「写了多少文件」。三栏各自的数据源：
//
//   - 接入面端点：`cmd/` 下生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目。不按
//     `adapters/http/` 的文件数——那数的是处理器，一个处理器可挂多个端点（TF 的 POD 首登与更正
//     即是两个端点一个处理器）。
//   - 消费适配器：`internal/<消费方>/adapters/` 下四类消费门目录的生产文件。「跨上下文消费缝」
//     那一栏按提供方目录名是不是另一个上下文来认，这四个目录名都不是上下文，因此一个都数不到。
//   - 直投路由表：`cmd/` 下生产文件里登记的不同事件类型。派发进程按特性文件登记各路（ADR-0222），键写在
//     `routeLeg` 字面量的 `event` 字段上；旧形的 `map[eventing.EventType]dispatch.Consumer` 字面量的键也认。同一事件类型
//     扇出给几路只算一条——路由表的一条就是一类信封，几路是它的扇出。
//
// 三处都按类型认字面量而不按函数名或文件名认：装配点改名、拆成几个函数、挪到别的文件，数照样对；
// 换一种字面量类型才会数不到，而那种改动本来就该顺手回来改这里。`routeLeg` 是 cmd/parcel-dispatch 自己的类型，按名字认，
// 理由同此。

// consumerDirs 是消费门目录名单。写死是有意的，理由同 nonBusinessDirs：判「这个目录是不是消费门」
// 要领域知识，工具没有也不该替它猜；新开一类消费门目录时由开目录的人在这里补一项。
var consumerDirs = []string{"inbox", "adoptconsume", "finalconsume", "veconsume"}

const (
	modulePath   = "go.idp.xyz/idp-parcel"
	httpapiPath  = modulePath + "/internal/platform/httpapi"
	dispatchPath = modulePath + "/internal/platform/dispatch"
	eventingPath = "go.idp.xyz/idp-bento-go/eventing"

	// unattributed 是归不到任何上下文的条目在表里的名字。它要显出来而不是被吞掉：一个端点的
	// 构造函数不在 `internal/<ctx>/adapters/http` 下、或一条路由的事件类型不从某个上下文的
	// 适配器包导出，本身就是要人看一眼的事。
	unattributed = "（未归类）"
)

var (
	httpAdapterPath = regexp.MustCompile(`^` + regexp.QuoteMeta(modulePath) + `/internal/([^/]+)/adapters/http$`)
	anyAdapterPath  = regexp.MustCompile(`^` + regexp.QuoteMeta(modulePath) + `/internal/([^/]+)/adapters/`)
)

// ContextTally 是一个上下文在某一栏上的数。
type ContextTally struct {
	Context string
	Count   int
}

// ConsumerTally 是一个消费方在四类消费门目录上各自的生产文件数。ByDir 的键取 consumerDirs。
type ConsumerTally struct {
	Consumer string
	ByDir    map[string]int
	Total    int
}

// WiringCensus 是一次接线面清点的全部结果。
type WiringCensus struct {
	Endpoints     []ContextTally
	EndpointTotal int
	Consumers     []ConsumerTally
	ConsumerTotal int
	Routes        []ContextTally
	RouteTotal    int
}

// ConsumerDirTotals 交回四类目录各自的合计，顺序同 consumerDirs。
func (census WiringCensus) ConsumerDirTotals() []int {
	totals := make([]int, len(consumerDirs))
	for _, consumer := range census.Consumers {
		for i, dir := range consumerDirs {
			totals[i] += consumer.ByDir[dir]
		}
	}
	return totals
}

// CensusWiring 清点 cmd/ 与 internal/ 两处的接线面。root 是仓库根。
func CensusWiring(root fs.FS) (WiringCensus, error) {
	census := WiringCensus{}

	endpoints := map[string]int{}
	routes := map[string]int{}
	seenRoutes := map[string]bool{}
	countRoute := func(parsed parsedFile, key ast.Expr) {
		identity := parsed.qualifiedName(key)
		if seenRoutes[identity] {
			return
		}
		seenRoutes[identity] = true
		census.RouteTotal++
		routes[parsed.contextOf(key, anyAdapterPath)]++
	}
	err := fs.WalkDir(root, "cmd", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isProductionGo(p) {
			return err
		}
		parsed, err := parseGoFile(root, p)
		if err != nil {
			return err
		}
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch {
			case parsed.isEndpointSlice(lit):
				for _, elt := range lit.Elts {
					census.EndpointTotal++
					endpoints[parsed.endpointContext(elt)]++
				}
			case parsed.isRouteTable(lit):
				for _, elt := range lit.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						countRoute(parsed, kv.Key)
					}
				}
			case isRouteLegSlice(lit):
				for _, elt := range lit.Elts {
					if leg, ok := elt.(*ast.CompositeLit); ok {
						if key := routeLegEvent(leg); key != nil {
							countRoute(parsed, key)
						}
					}
				}
			case isRouteLeg(lit):
				if key := routeLegEvent(lit); key != nil {
					countRoute(parsed, key)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return census, err
	}

	consumers := map[string]*ConsumerTally{}
	err = fs.WalkDir(root, "internal", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isProductionGo(p) {
			return err
		}
		parts := strings.Split(p, "/")
		if len(parts) < 3 {
			return nil
		}
		for _, dir := range consumerDirs {
			if !matchesAdapter(parts, dir) {
				continue
			}
			tally := consumers[parts[1]]
			if tally == nil {
				tally = &ConsumerTally{Consumer: parts[1], ByDir: map[string]int{}}
				consumers[parts[1]] = tally
			}
			tally.ByDir[dir]++
			tally.Total++
			census.ConsumerTotal++
		}
		return nil
	})
	if err != nil {
		return census, err
	}

	census.Endpoints = sortedTallies(endpoints)
	census.Routes = sortedTallies(routes)
	for _, tally := range consumers {
		census.Consumers = append(census.Consumers, *tally)
	}
	sort.Slice(census.Consumers, func(i, j int) bool {
		return census.Consumers[i].Consumer < census.Consumers[j].Consumer
	})
	return census, nil
}

func isProductionGo(p string) bool {
	return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
}

// sortedTallies 按上下文名排序；未归类那一格排最后，免得它混在字母序里被当成一个上下文。
func sortedTallies(counts map[string]int) []ContextTally {
	var out []ContextTally
	for name, count := range counts {
		out = append(out, ContextTally{Context: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Context == unattributed) != (out[j].Context == unattributed) {
			return out[j].Context == unattributed
		}
		return out[i].Context < out[j].Context
	})
	return out
}

// parsedFile 是一份已解析的 Go 源文件连同它的导入表（包名或别名 → 导入路径）。
//
// 只做语法解析不做类型检查：认字面量的类型靠「选择子的包别名解析到哪条导入路径」就够了，而加载
// 类型信息要把整个模块编起来——端口清点已经付过一次那个代价，接线面不必再付。
type parsedFile struct {
	file    *ast.File
	imports map[string]string
}

func parseGoFile(root fs.FS, p string) (parsedFile, error) {
	src, err := fs.ReadFile(root, p)
	if err != nil {
		return parsedFile{}, err
	}
	file, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
	if err != nil {
		return parsedFile{}, fmt.Errorf("解析 %s：%w", p, err)
	}
	imports := map[string]string{}
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		// 未起别名的导入按路径末段当包名。它对包名与目录名不同的包会认错，但本清点要认的四个
		// 包（httpapi、dispatch、eventing 与各上下文的 adapters/*）末段都就是包名。
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = importPath
	}
	return parsedFile{file: file, imports: imports}, nil
}

// refersTo 判一个类型表达式是不是 `<某导入>.<name>`，导入按路径认而不按别名认。
func (parsed parsedFile) refersTo(expr ast.Expr, importPath, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return parsed.imports[pkg.Name] == importPath && selector.Sel.Name == name
}

// isEndpointSlice 认 `[]httpapi.BusinessEndpoint{...}`。
func (parsed parsedFile) isEndpointSlice(lit *ast.CompositeLit) bool {
	array, ok := lit.Type.(*ast.ArrayType)
	return ok && array.Len == nil && parsed.refersTo(array.Elt, httpapiPath, "BusinessEndpoint")
}

// isRouteTable 认 `map[eventing.EventType]dispatch.Consumer{...}`。
func (parsed parsedFile) isRouteTable(lit *ast.CompositeLit) bool {
	mapType, ok := lit.Type.(*ast.MapType)
	return ok &&
		parsed.refersTo(mapType.Key, eventingPath, "EventType") &&
		parsed.refersTo(mapType.Value, dispatchPath, "Consumer")
}

// endpointContext 由端点条目的 Handler 构造函数所在包推出上下文：构造函数必须来自
// `internal/<ctx>/adapters/http`（ADR-0018 端点落在各上下文自己的 http 适配器里）。Handler
// 不是一次直接的构造调用、或构造函数不在那个位置，都归未归类——数照样数，只是归属要人看。
func (parsed parsedFile) endpointContext(elt ast.Expr) string {
	entry, ok := elt.(*ast.CompositeLit)
	if !ok {
		return unattributed
	}
	for _, field := range entry.Elts {
		kv, ok := field.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Handler" {
			continue
		}
		call, ok := kv.Value.(*ast.CallExpr)
		if !ok {
			return unattributed
		}
		return parsed.contextOf(call.Fun, httpAdapterPath)
	}
	return unattributed
}

// isRouteLegSlice 认 `[]routeLeg{...}`，isRouteLeg 认 `routeLeg{...}`。
func isRouteLegSlice(lit *ast.CompositeLit) bool {
	array, ok := lit.Type.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	ident, ok := array.Elt.(*ast.Ident)
	return ok && ident.Name == "routeLeg"
}

func isRouteLeg(lit *ast.CompositeLit) bool {
	ident, ok := lit.Type.(*ast.Ident)
	return ok && ident.Name == "routeLeg"
}

// routeLegEvent 取一路登记的 `event` 字段；没写的交回 nil，不数。
func routeLegEvent(leg *ast.CompositeLit) ast.Expr {
	for _, field := range leg.Elts {
		kv, ok := field.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "event" {
			return kv.Value
		}
	}
	return nil
}

// qualifiedName 是路由键的身份：`<别名>.<名字>` 按导入路径展开，各文件别名不同也认得出是同一个常量；认不出的按原文。
func (parsed parsedFile) qualifiedName(expr ast.Expr) string {
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		if pkg, ok := selector.X.(*ast.Ident); ok {
			if importPath, ok := parsed.imports[pkg.Name]; ok {
				return importPath + "." + selector.Sel.Name
			}
		}
	}
	var text strings.Builder
	_ = printer.Fprint(&text, token.NewFileSet(), expr)
	return text.String()
}

// contextOf 把 `<别名>.<名字>` 里的别名解析成导入路径，再按 pattern 取出上下文名。
func (parsed parsedFile) contextOf(expr ast.Expr, pattern *regexp.Regexp) string {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return unattributed
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return unattributed
	}
	match := pattern.FindStringSubmatch(parsed.imports[pkg.Name])
	if match == nil {
		return unattributed
	}
	return match[1]
}
