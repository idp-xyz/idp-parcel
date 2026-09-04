package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 类型可达性棘轮门禁——函数名棘轮（production_wiring_ratchet_test.go）的姊妹，各守一族。
//
// 守的同样是方向不是状态：`internal/<上下文>/domain` 下一个导出的顶层具名类型，若生产代码
// 顺着任何一条路都碰不到它，它就进名单；门禁只判名单有没有变长，不判名单里的东西该不该在。
//
// 为什么要第二道而不是把函数名棘轮加宽：那一道按名字前缀排除了 `New*`，理由成立（值对象构造
// 器是另一族，全收会淹掉名单），但聚合根若按 `NewXxx` 命名就跟着一起漏了，而 `NewDecimal`
// 那种与被删掉的 `DecimalFromInt64` 同形、命名方向相反的影子，也只有按类型才看得见。改判据
// 会重建基线、历轮不可比，而棘轮的价值有一半在可比——所以另立一份，两族各守各的。
//
// 判据是**可达性**而不是「有没有被指名」：`CurrencyCode` 这类值对象由聚合的取值方法返回出去，
// 外部代码从不指名它却完全用得上，按指名判全是噪声。所以分两步——先取种子：domain 包之外的
// 非测试代码里以 `<导入名>.<名字>` 出现过的名字，是类型就直接入种子，是函数、常量或变量则把
// 其签名或声明类型里的本包类型入种子；再沿包内类型图闭包：T→U 的边来自 T 的字段类型、T 的
// 方法签名，以及 T 自身的类型表达式（切片、map、函数类型里出现的本包类型）。闭包之外的导出
// 类型才是「有模型无执行器」。
//
// 与当初那支探针（已退役，见 .scratch/domain-executor-audit/README.md）相比多走了三格，都朝
// 「量准」的方向：生产代码的范围与函数名棘轮同一口径（含 `cmd/`——装配点与 CLI 是真实的生产
// 消费者，探针只扫 `internal/`）；导出常量与变量的声明类型算边（`domain.KindA` 让 Kind 可达，
// 含 iota 组里隐式继承类型的那几条）；未导出的中间类型也在图上（它是路上的一段，不是终点）。
// 三格都只会让名单变短，不会凭空多报。
//
// 用选择器加导入表而不用全文正则：正则会让跨上下文同名类型互相掩盖（一处有消费就让另一处的
// 缺口消失），而那正是本门禁要找的东西。解析导入名再匹配选择器，同名各算各的。
//
// 守不住的几格，如实写在这里（不写数目，理由同函数名棘轮头注）：
//   - 接口值传递：某类型只经由它实现的某个接口被用到时，图上没有那条边。方向是多报——多出
//     来的一行会有人看见并写理由，比少报安全。
//   - 点导入与反射按字符串取用的类型。本仓两者都没有（同函数名棘轮头注的实测），换个仓库不成立。
//   - 导出了却只在包内用的类型（由未导出函数产出、包外从不经手）会进名单，而那是该改小写的
//     设计瑕疵不是缺执行器。**基线不替它们分类**——每一行的理由由加它的人写，那时他知道答案。
//   - 只看 `internal/<上下文>/domain` 下的导出顶层具名类型；别名（`type X = Y`）不算，它没有
//     自己的身份。

// reachabilityEntry 是名单里的一条。键是**包 + 类型名**，理由与函数名棘轮的 wiringEntry 相同：
// 跨包重名时裸名字作键会让一增一减互相抵消。
type reachabilityEntry struct {
	pkg  string
	name string
}

func (entry reachabilityEntry) String() string { return entry.pkg + " " + entry.name }

// domainTypeGraph 是一个 domain 包的类型图。三张表的键都是裸类型名——图是包内的，包已由
// pkg 字段钉住。
type domainTypeGraph struct {
	pkg string
	// declared 是导出的顶层具名类型；别名不入。
	declared map[string]bool
	// edges[T] 是从 T 出发一步可达的本包类型：T 的类型表达式里出现的、以及接收者为 T 的
	// 方法签名里出现的。
	edges map[string]map[string]bool
	// viaName[F] 是导出函数 F 的签名、或导出常量/变量 F 的声明类型里出现的本包类型——生产
	// 代码写 `domain.NewX(...)` 或 `domain.KindFoo` 时，X 与 Kind 就该可达。
	viaName map[string]map[string]bool
}

func newDomainTypeGraph(pkg string) *domainTypeGraph {
	return &domainTypeGraph{
		pkg:      pkg,
		declared: map[string]bool{},
		edges:    map[string]map[string]bool{},
		viaName:  map[string]map[string]bool{},
	}
}

func (graph *domainTypeGraph) link(target map[string]map[string]bool, from string, exprs ...ast.Expr) {
	set, ok := target[from]
	if !ok {
		set = map[string]bool{}
		target[from] = set
	}
	for _, expr := range exprs {
		if expr == nil {
			continue
		}
		for _, name := range namedTypesIn(expr) {
			set[name] = true
		}
	}
}

// namedTypesIn 摘出一个类型表达式里所有本包标识符。跨包选择器（`time.Time`）不摘；字段名与
// 参数名也不摘——`Tenant TenantID` 这一格里只有 TenantID 是类型，字段名恰与某个类型同名时
// 不该凭空形成一条边。未导出的也摘：它们是路上的一段（`type Aggregate struct{ inner detail }`
// 里的 detail 再持有导出类型时那条路是真的走得通），只在报名单时才按导出过滤。内建名
// （`string`、`error`）也会被摘到，但它们不是任何图的键，落下去是空操作。
func namedTypesIn(expr ast.Expr) []string {
	var names []string
	var visit func(node ast.Node) bool
	visit = func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			return false
		case *ast.Field:
			if typed.Type != nil {
				ast.Inspect(typed.Type, visit)
			}
			return false
		case *ast.Ident:
			names = append(names, typed.Name)
		}
		return true
	}
	ast.Inspect(expr, visit)
	return names
}

// collectDomainTypeGraph 把一份 domain 包语法树并进图里。
func collectDomainTypeGraph(syntax *ast.File, graph *domainTypeGraph) {
	for _, decl := range syntax.Decls {
		switch typed := decl.(type) {
		case *ast.GenDecl:
			switch typed.Tok {
			case token.TYPE:
				for _, spec := range typed.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec.Name == nil {
						continue
					}
					if typeSpec.Name.IsExported() && !typeSpec.Assign.IsValid() {
						graph.declared[typeSpec.Name.Name] = true
					}
					// 未导出类型也要有边：`type Aggregate struct{ inner detail }`、`detail` 再持有
					// 导出类型时，那条路是真的走得通的。
					graph.link(graph.edges, typeSpec.Name.Name, typeSpec.Type)
				}
			case token.CONST, token.VAR:
				// `const ( A Kind = iota; B; C )` 里 B、C 的类型继承自上一条带值的声明——Go 的
				// 隐式重复。只有带值的那条能改写「当前继承的类型」。
				var inherited ast.Expr
				for _, spec := range typed.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					declaredType := valueSpec.Type
					if typed.Tok == token.CONST {
						if len(valueSpec.Values) > 0 {
							inherited = valueSpec.Type
						} else {
							declaredType = inherited
						}
					}
					if declaredType == nil {
						continue
					}
					for _, name := range valueSpec.Names {
						if name.IsExported() {
							graph.link(graph.viaName, name.Name, declaredType)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if typed.Name == nil || typed.Type == nil {
				continue
			}
			// receiverTypeName 复用 rehydration_gate_test.go 的那一个：指针与泛型实例都剥到底层标识符。
			if receiver := receiverTypeName(typed); receiver != "" {
				graph.link(graph.edges, receiver, typed.Type)
				continue
			}
			if typed.Recv == nil && typed.Name.IsExported() {
				graph.link(graph.viaName, typed.Name.Name, typed.Type)
			}
		}
	}
}

// domainImportNames 把一份文件里指向各被扫 domain 包的导入映射成「本地名 → 包目录」。
// 别名导入（`pcdomain "…/partycommercial/domain"`）按别名认，不按默认包名。
func domainImportNames(syntax *ast.File, graphs map[string]*domainTypeGraph) map[string]string {
	names := map[string]string{}
	for _, spec := range syntax.Imports {
		if spec.Path == nil {
			continue
		}
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		pkgDir, ok := strings.CutPrefix(path, modulePath+"/")
		if !ok {
			continue
		}
		if _, scanned := graphs[pkgDir]; !scanned {
			continue
		}
		local := pkgDir[strings.LastIndex(pkgDir, "/")+1:]
		if spec.Name != nil {
			local = spec.Name.Name
		}
		names[local] = pkgDir
	}
	return names
}

// collectDomainTypeSeeds 记下一份生产文件对各 domain 包的选择器引用。调用方负责只传非测试、
// 且不在该 domain 包内的文件——包内引用不是「生产消费者」，那是它自己。
func collectDomainTypeSeeds(source wiringSource, graphs map[string]*domainTypeGraph, seeds map[reachabilityEntry]bool) {
	names := domainImportNames(source.syntax, graphs)
	if len(names) == 0 {
		return
	}
	ast.Inspect(source.syntax, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok || selector.Sel == nil {
			return true
		}
		if pkgDir, ok := names[ident.Name]; ok && pkgDir != source.pkgDir {
			seeds[reachabilityEntry{pkg: pkgDir, name: selector.Sel.Name}] = true
		}
		return true
	})
}

// closeOverDomainTypes 从种子出发沿包内类型图求可达的导出类型集。
func closeOverDomainTypes(graph *domainTypeGraph, seedNames map[string]bool) map[string]bool {
	reachable := map[string]bool{}
	var queue []string
	push := func(name string) {
		if reachable[name] {
			return
		}
		// 未导出类型也入队——它是路上的一段，不是终点；报名单时只看 declared。
		reachable[name] = true
		queue = append(queue, name)
	}
	for name := range seedNames {
		if graph.declared[name] {
			push(name)
		}
		for linked := range graph.viaName[name] {
			push(linked)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range graph.edges[current] {
			push(next)
		}
	}
	return reachable
}

// buildDomainTypeGraphs 按包建图。
func buildDomainTypeGraphs(sources []wiringSource) map[string]*domainTypeGraph {
	graphs := map[string]*domainTypeGraph{}
	for _, source := range sources {
		if source.isTest || !isScannedDomainPackage(source.pkgDir) {
			continue
		}
		graph, ok := graphs[source.pkgDir]
		if !ok {
			graph = newDomainTypeGraph(source.pkgDir)
			graphs[source.pkgDir] = graph
		}
		collectDomainTypeGraph(source.syntax, graph)
	}
	return graphs
}

// currentlyUnreachableDomainTypes 算出此刻的名单。
func currentlyUnreachableDomainTypes(sources []wiringSource) []reachabilityEntry {
	graphs := buildDomainTypeGraphs(sources)

	seeds := map[reachabilityEntry]bool{}
	for _, source := range sources {
		if source.isTest {
			continue
		}
		collectDomainTypeSeeds(source, graphs, seeds)
	}

	var unreachable []reachabilityEntry
	for pkgDir, graph := range graphs {
		seedNames := map[string]bool{}
		for seed := range seeds {
			if seed.pkg == pkgDir {
				seedNames[seed.name] = true
			}
		}
		reachable := closeOverDomainTypes(graph, seedNames)
		for name := range graph.declared {
			if !reachable[name] {
				unreachable = append(unreachable, reachabilityEntry{pkg: pkgDir, name: name})
			}
		}
	}
	sort.Slice(unreachable, func(i, j int) bool { return unreachable[i].String() < unreachable[j].String() })
	return unreachable
}

const typeReachabilityBaselineFile = "production_type_reachability_baseline.txt"

// readTypeReachabilityBaseline 读基线。每行 `包 类型名`，`#` 起注释；空文件合法。
//
// 与 readWiringBaseline 同形而不抽公共函数：抽了就得改那边，而「不动函数名棘轮」是票面红线。
// 两份基线的键类型也刻意不同，免得一边的名单被拿去核另一边。
func readTypeReachabilityBaseline(t *testing.T) map[reachabilityEntry]bool {
	t.Helper()

	raw, err := os.ReadFile(typeReachabilityBaselineFile)
	if err != nil {
		t.Fatalf("读基线 %s：%v", typeReachabilityBaselineFile, err)
	}

	baseline := make(map[reachabilityEntry]bool)
	for index, line := range strings.Split(string(raw), "\n") {
		if cut := strings.IndexByte(line, '#'); cut >= 0 {
			line = line[:cut]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("%s 第 %d 行不是「包 类型名」两段：%q", typeReachabilityBaselineFile, index+1, line)
		}
		baseline[reachabilityEntry{pkg: fields[0], name: fields[1]}] = true
	}
	return baseline
}

// TestTypeReachabilityRatchetNoNewDomainTypeGoesUnreachable 名单只许变短。
//
// 新增一条不是不许，是要有人显式写进基线并留一行理由——它在等哪一层、由谁接，或者它其实
// 该改小写。写理由的人在写的当下正好知道答案，门禁不替他猜。
//
// 三个用例名都带 TypeReachability，让 `-run TypeReachability` 一次跑全这道门禁。
func TestTypeReachabilityRatchetNoNewDomainTypeGoesUnreachable(t *testing.T) {
	baseline := readTypeReachabilityBaseline(t)

	for _, entry := range currentlyUnreachableDomainTypes(loadWiringSources(t)) {
		if !baseline[entry] {
			t.Errorf("%s 是导出的领域类型，生产代码顺着任何一条路都碰不到它，而它不在 %s 里。"+
				"要么让生产代码消费它（直接指名，或经某个已消费的类型的字段/方法签名/构造函数拿到它），"+
				"要么把这一行加进基线并写明它在等什么：\n\t%s",
				entry, typeReachabilityBaselineFile, entry)
		}
	}
}

// TestTypeReachabilityBaselineHasNoStaleEntry 基线不许烂，理由与函数名棘轮同一条：只判增量
// 的门禁会让被删、改名、或已接上线的条目永远躺在基线里，地面真相漂走而门禁一直绿。
func TestTypeReachabilityBaselineHasNoStaleEntry(t *testing.T) {
	sources := loadWiringSources(t)

	declared := make(map[reachabilityEntry]bool)
	for pkgDir, graph := range buildDomainTypeGraphs(sources) {
		for name := range graph.declared {
			declared[reachabilityEntry{pkg: pkgDir, name: name}] = true
		}
	}

	unreachable := make(map[reachabilityEntry]bool)
	for _, entry := range currentlyUnreachableDomainTypes(sources) {
		unreachable[entry] = true
	}

	var stale []reachabilityEntry
	for entry := range readTypeReachabilityBaseline(t) {
		if !declared[entry] || !unreachable[entry] {
			stale = append(stale, entry)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].String() < stale[j].String() })

	for _, entry := range stale {
		t.Errorf("%s 在基线里，但它此刻不在名单上。两种成因，先分清再剪：\n"+
			"\t一、它已被删、改名、改小写或搬包——剪掉这一行。\n"+
			"\t二、生产代码真的碰得到它了——剪掉这一行，这是好消息；顺手看看它上方的理由行说的"+
			"「等谁」是不是这一位，不是的话新来的那条路值得看一眼。",
			entry)
	}
}

// TestTheTypeReachabilityRatchetCanActuallyCatchAViolation 证这道门禁真能红。
//
// 与本包其余门禁同一条理由：一个从来不会失败的门禁比没有门禁更坑人。用合成源码逼出建图、
// 取种子与闭包各自的边界，不重复跑全仓。
func TestTheTypeReachabilityRatchetCanActuallyCatchAViolation(t *testing.T) {
	parse := func(t *testing.T, source string) *ast.File {
		t.Helper()
		syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", source, 0)
		if err != nil {
			t.Fatalf("解析合成源码：%v", err)
		}
		return syntax
	}
	const domainPkg = "internal/x/domain"
	domainSource := func(t *testing.T, body string) wiringSource {
		return wiringSource{pkgDir: domainPkg, syntax: parse(t, "package domain\n\n"+body)}
	}
	consumer := func(t *testing.T, pkgDir, body string) wiringSource {
		return wiringSource{
			pkgDir:  pkgDir,
			imports: map[string]bool{modulePath + "/" + domainPkg: true},
			syntax:  parse(t, "package consumer\n\nimport \""+modulePath+"/"+domainPkg+"\"\n\n"+body),
		}
	}
	names := func(entries []reachabilityEntry) []string {
		var out []string
		for _, entry := range entries {
			out = append(out, entry.name)
		}
		return out
	}
	same := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for index := range got {
			if got[index] != want[index] {
				return false
			}
		}
		return true
	}

	cases := []struct {
		name    string
		sources []wiringSource
		want    []string
	}{
		{
			name:    "只有声明、无人消费——进名单",
			sources: []wiringSource{domainSource(t, "type Orphan struct{}\n")},
			want:    []string{"Orphan"},
		},
		{
			name: "生产代码直接指名——可达",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n"),
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: nil,
		},
		{
			name: "顺着已消费类型的字段可达",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{ Money Amount }\n\ntype Amount struct{}\n"),
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: nil,
		},
		{
			name: "顺着已消费类型的方法签名可达——取值方法返回的值对象不是缺口",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n\nfunc (Aggregate) Currency() CurrencyCode { return \"\" }\n\ntype CurrencyCode string\n"),
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: nil,
		},
		{
			name: "只被 New* 构造函数交出的聚合根——这正是函数名棘轮看不见的那一格",
			sources: []wiringSource{
				domainSource(t, "type Mapping struct{}\n\nfunc NewMapping() (Mapping, error) { return Mapping{}, nil }\n"),
				consumer(t, "internal/x/application", "func Do() { _, _ = domain.NewMapping() }\n"),
			},
			want: nil,
		},
		{
			name: "顺着导出常量的声明类型可达——含 iota 组里隐式继承类型的那几条",
			sources: []wiringSource{
				domainSource(t, "type Kind string\n\ntype Grade int\n\nconst KindA Kind = \"A\"\n\nconst (\n\tGradeOne Grade = iota\n\tGradeTwo\n)\n"),
				consumer(t, "internal/x/application", "var _ = domain.KindA\n\nvar _ = domain.GradeTwo\n"),
			},
			want: nil,
		},
		{
			name: "只被测试消费——照样进名单，那正是本门禁要抓的形状",
			sources: []wiringSource{
				domainSource(t, "type Orphan struct{}\n"),
				{
					pkgDir:  "internal/x/application",
					isTest:  true,
					imports: map[string]bool{modulePath + "/" + domainPkg: true},
					syntax:  parse(t, "package application\n\nimport \""+modulePath+"/"+domainPkg+"\"\n\nvar _ domain.Orphan\n"),
				},
			},
			want: []string{"Orphan"},
		},
		{
			name: "别名不是身份——不进名单",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n\ntype Alias = Aggregate\n"),
			},
			want: []string{"Aggregate"},
		},
		{
			name: "字段名与类型同名不形成边——名字不是类型",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{ Tenant string }\n\ntype Tenant struct{}\n"),
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: []string{"Tenant"},
		},
		{
			name: "别名导入按别名认",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n"),
				{
					pkgDir:  "internal/x/application",
					imports: map[string]bool{modulePath + "/" + domainPkg: true},
					syntax:  parse(t, "package application\n\nimport xdomain \""+modulePath+"/"+domainPkg+"\"\n\nvar _ xdomain.Aggregate\n"),
				},
			},
			want: nil,
		},
		{
			name: "不导入该 domain 包的文件里出现同名选择器不算——名字不是唯一键",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n"),
				{
					pkgDir:  "internal/platform/buildinfo",
					imports: map[string]bool{},
					syntax:  parse(t, "package buildinfo\n\ntype domainT struct{ Aggregate int }\n\nvar domain domainT\n\nvar _ = domain.Aggregate\n"),
				},
			},
			want: []string{"Aggregate"},
		},
		{
			name: "跨包同名各算各的——一处有消费不能掩盖另一处的缺口",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{}\n"),
				{pkgDir: "internal/y/domain", syntax: parse(t, "package domain\n\ntype Aggregate struct{}\n")},
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: []string{"Aggregate"},
		},
		{
			name: "经未导出中间类型仍然可达——路上的一段不必导出",
			sources: []wiringSource{
				domainSource(t, "type Aggregate struct{ inner detail }\n\ntype detail struct{ Leaf Leaf }\n\ntype Leaf struct{}\n"),
				consumer(t, "internal/x/application", "var _ domain.Aggregate\n"),
			},
			want: nil,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := names(currentlyUnreachableDomainTypes(test.sources))
			if !same(got, test.want) {
				t.Fatalf("名单 = %v，want %v", got, test.want)
			}
		})
	}

	// 键必须是包加类型名：两个不同包的同名类型不能互相抵消。
	if (reachabilityEntry{pkg: "internal/a/domain", name: "T"}) == (reachabilityEntry{pkg: "internal/b/domain", name: "T"}) {
		t.Fatal("包加名没有构成唯一键，跨包重名会让一增一减互相抵消")
	}
}
