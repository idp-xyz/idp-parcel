// 一次性探针：找出「有领域模型、但生产代码里没有任何执行器」的类型。
//
// 为什么要它：开发主线把「骨架完整」定义成「该切片的规则与判断都有执行器」，而 r27 给这条
// 判据的证据是三样——生产代码零 TODO/FIXME/not implemented/panic(、编排与适配器文件计数、
// 端口两口径缺口。三样对本探针要找的这一类**在构造上全盲**：一个只有领域模型、无人消费的
// 类型是干净的完整代码不含标记；它不是编排也不是适配器；而端口两口径都从
// `internal/*/ports` **已声明的接口**出发，一条规则从来没被开过端口，它不在样本里。
//
// 判据是**可达性**，不是「有没有被指名」。第一版按指名判，1127 个导出领域类型里报出 403 个，
// 全是噪声：`CurrencyCode` 这类值对象由聚合的取值方法返回出去，外部代码从不指名它却完全用得上
// ——那不是缺执行器，那是正常的 Go。
//
// 所以改成两步。先取**种子**：生产代码（该 domain 包之外的非测试 .go）里以
// `<domain 包别名>.<名字>` 出现过的名字，类型直接入种子，函数则把其签名里的类型入种子。
// 再沿包内的类型图闭包：T→U 的边来自 T 的结构体字段类型，以及接收者为 T 的方法签名。
// 闭包之外的导出类型，才是生产代码**顺着任何一条路都碰不到**的——那才是「有模型无执行器」。
//
// 用选择器表达式而不用全文正则：正则会被跨上下文同名类型互相掩盖（一处有消费就让另一处的
// 缺口消失），而那正是本探针要找的东西。解析导入别名再匹配选择器，同名不互相干扰。
//
// 已知会漏（方向是保守的，宁可少报不多报）：
//   - 点导入与反射按字符串取用的类型。本仓两者都没有，但换个仓库就不成立。
//   - 接口值传递：某类型只经由它实现的某个接口被用到时，图上没有那条边。
//
// 扔弃件，不进 CI。够格与否看跑出来的信噪比再定。
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const domainSuffix = "/domain"

type declared struct {
	context  string
	name     string
	file     string
	kind     string
	testHits int
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	internal := filepath.Join(root, "internal")

	var decls []*declared
	// 各 map 的键一律是「上下文.名字」：同名类型分属不同上下文时各算各的。
	consumed := map[string]bool{}
	// edges[T] 是从 T 出发一步可达的类型集：T 的字段类型，以及接收者为 T 的方法签名类型。
	edges := map[string]map[string]bool{}
	// funcSig[F] 是包级函数 F 签名里出现的类型集——生产代码调 domain.NewX 时，X 就该可达。
	funcSig := map[string]map[string]bool{}
	declaredTypes := map[string]bool{}
	// 自身 domain 包测试里的出现次数，用来把「有测试无执行器」与「域内死码」分开——
	// 前者是规则实现了却没人能触发它，后者多半只是没删干净，两者要人做的事不同。
	testHits := map[string]int{}

	fset := token.NewFileSet()

	err := filepath.WalkDir(internal, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		slash := filepath.ToSlash(path)
		isTest := strings.HasSuffix(slash, "_test.go")
		inDomain, ctx := domainPackageOf(slash)

		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", slash, parseErr)
		}

		switch {
		case inDomain && !isTest:
			collectDeclarations(file, ctx, slash, &decls, declaredTypes)
			collectGraph(file, ctx, edges, funcSig)
		case inDomain && isTest:
			// 同包测试用裸名字引用，没有选择器可匹配；外部测试包（domain_test）则有。
			countTestMentions(file, ctx, testHits)
		case !isTest:
			collectDomainSelectors(file, consumed)
		}
		return nil
	})
	if err != nil {
		return err
	}

	reachable := closeOver(consumed, declaredTypes, edges, funcSig)

	var orphans []*declared
	for _, decl := range decls {
		key := decl.context + "." + decl.name
		if reachable[key] {
			continue
		}
		decl.testHits = testHits[key]
		orphans = append(orphans, decl)
	}
	sort.Slice(orphans, func(i, j int) bool {
		if orphans[i].context != orphans[j].context {
			return orphans[i].context < orphans[j].context
		}
		return orphans[i].name < orphans[j].name
	})

	report(decls, orphans)
	return nil
}

// domainPackageOf 判断一个文件是否直接躺在某上下文的 domain 包里，并交回上下文名。
// 只认 internal/<ctx>/domain/*.go 一层，子包另算——本仓 domain 下没有子包。
func domainPackageOf(slash string) (bool, string) {
	var rest string
	switch {
	case strings.Contains(slash, "/internal/"):
		rest = slash[strings.Index(slash, "/internal/")+len("/internal/"):]
	case strings.HasPrefix(slash, "internal/"):
		rest = strings.TrimPrefix(slash, "internal/")
	default:
		return false, ""
	}
	dir := filepath.ToSlash(filepath.Dir(rest))
	ctx, ok := strings.CutSuffix(dir, domainSuffix)
	if !ok || strings.Contains(ctx, "/") {
		return false, ""
	}
	return true, ctx
}

func collectDeclarations(file *ast.File, ctx, slash string, out *[]*declared, declaredTypes map[string]bool) {
	for _, node := range file.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !ts.Name.IsExported() || ts.Assign.IsValid() {
				continue
			}
			declaredTypes[ctx+"."+ts.Name.Name] = true
			*out = append(*out, &declared{
				context: ctx,
				name:    ts.Name.Name,
				file:    slash,
				kind:    typeKind(ts.Type),
			})
		}
	}
}

// collectGraph 建包内可达图：类型 → 其字段类型与自身方法签名里的类型；包级函数 → 其签名类型。
func collectGraph(file *ast.File, ctx string, edges, funcSig map[string]map[string]bool) {
	add := func(target map[string]map[string]bool, from string, exprs ...ast.Expr) {
		set, ok := target[from]
		if !ok {
			set = map[string]bool{}
			target[from] = set
		}
		for _, expr := range exprs {
			for _, name := range namedTypesIn(expr) {
				set[ctx+"."+name] = true
			}
		}
	}

	for _, node := range file.Decls {
		switch decl := node.(type) {
		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				from := ctx + "." + ts.Name.Name
				// 结构体字段、接口方法与其它构造（切片、map、函数类型）一并走，
				// 因为「顺着这个类型能拿到什么」不取决于它长成哪一种。
				add(edges, from, ts.Type)
			}
		case *ast.FuncDecl:
			var exprs []ast.Expr
			if decl.Type.Params != nil {
				for _, field := range decl.Type.Params.List {
					exprs = append(exprs, field.Type)
				}
			}
			if decl.Type.Results != nil {
				for _, field := range decl.Type.Results.List {
					exprs = append(exprs, field.Type)
				}
			}
			if recv := receiverName(decl); recv != "" {
				add(edges, ctx+"."+recv, exprs...)
				continue
			}
			if decl.Name.IsExported() {
				add(funcSig, ctx+"."+decl.Name.Name, exprs...)
			}
		}
	}
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	names := namedTypesIn(decl.Recv.List[0].Type)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// namedTypesIn 摘出一个类型表达式里所有本包导出标识符。跨包选择器（`time.Time`）不摘。
func namedTypesIn(expr ast.Expr) []string {
	var names []string
	ast.Inspect(expr, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			return false
		case *ast.Ident:
			if n.IsExported() {
				names = append(names, n.Name)
			}
		}
		return true
	})
	return names
}

// closeOver 从生产代码指名的种子出发，沿类型图求可达集。
func closeOver(consumed, declaredTypes map[string]bool, edges, funcSig map[string]map[string]bool) map[string]bool {
	reachable := map[string]bool{}
	var queue []string
	push := func(key string) {
		if declaredTypes[key] && !reachable[key] {
			reachable[key] = true
			queue = append(queue, key)
		}
	}

	for key := range consumed {
		push(key)
		// 被指名的可能是构造函数而不是类型：domain.NewX 让 X 可达。
		for sig := range funcSig[key] {
			push(sig)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range edges[current] {
			push(next)
		}
	}
	return reachable
}

func typeKind(expr ast.Expr) string {
	switch expr.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.FuncType:
		return "func"
	default:
		return "基础/别名"
	}
}

// collectDomainSelectors 记下本文件对各上下文 domain 包的选择器引用。
func collectDomainSelectors(file *ast.File, consumed map[string]bool) {
	aliases := domainImportAliases(file)
	if len(aliases) == 0 {
		return
	}
	ast.Inspect(file, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ctx, ok := aliases[ident.Name]; ok {
			consumed[ctx+"."+sel.Sel.Name] = true
		}
		return true
	})
}

// domainImportAliases 把本文件里指向各 domain 包的导入映射成「本地名 → 上下文」。
func domainImportAliases(file *ast.File) map[string]string {
	aliases := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		rest, ok := strings.CutPrefix(path, "go.idp.xyz/idp-parcel/internal/")
		if !ok {
			continue
		}
		ctx, ok := strings.CutSuffix(rest, domainSuffix)
		if !ok || strings.Contains(ctx, "/") {
			continue
		}
		local := "domain"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		aliases[local] = ctx
	}
	return aliases
}

// countTestMentions 数自身 domain 包测试里对某名字的裸引用。同包测试没有选择器，
// 因此按标识符数；外部测试包的选择器引用也一并落到这里。
func countTestMentions(file *ast.File, ctx string, hits map[string]int) {
	aliases := domainImportAliases(file)
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if ident, ok := n.X.(*ast.Ident); ok {
				if c, ok := aliases[ident.Name]; ok {
					hits[c+"."+n.Sel.Name]++
					return false
				}
			}
		case *ast.Ident:
			if n.IsExported() {
				hits[ctx+"."+n.Name]++
			}
		}
		return true
	})
}

func report(all []*declared, orphans []*declared) {
	byContext := map[string]int{}
	for _, decl := range all {
		byContext[decl.context]++
	}
	orphansBy := map[string][]*declared{}
	for _, decl := range orphans {
		orphansBy[decl.context] = append(orphansBy[decl.context], decl)
	}

	contexts := make([]string, 0, len(byContext))
	for ctx := range byContext {
		contexts = append(contexts, ctx)
	}
	sort.Strings(contexts)

	fmt.Printf("# 领域类型零生产消费者普查（探针，扔弃件）\n\n")
	fmt.Printf("导出领域类型共 %d 个，其中零生产消费者 %d 个。\n\n", len(all), len(orphans))
	fmt.Printf("| 上下文 | 导出领域类型 | 零消费者 | 其中有自身测试 |\n|---|---|---|---|\n")
	for _, ctx := range contexts {
		withTests := 0
		for _, decl := range orphansBy[ctx] {
			if decl.testHits > 0 {
				withTests++
			}
		}
		fmt.Printf("| %s | %d | %d | %d |\n", ctx, byContext[ctx], len(orphansBy[ctx]), withTests)
	}

	fmt.Printf("\n## 逐个点名\n\n")
	for _, ctx := range contexts {
		list := orphansBy[ctx]
		if len(list) == 0 {
			continue
		}
		fmt.Printf("\n### %s（%d）\n\n", ctx, len(list))
		fmt.Printf("| 类型 | 形态 | 自身测试提及 | 声明处 |\n|---|---|---|---|\n")
		for _, decl := range list {
			fmt.Printf("| `%s` | %s | %d | %s |\n", decl.name, decl.kind, decl.testHits, decl.file)
		}
	}
}
