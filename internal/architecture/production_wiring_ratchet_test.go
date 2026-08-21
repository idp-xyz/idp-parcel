package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// 生产接线棘轮门禁。
//
// 守的是方向不是状态：`internal/<上下文>/domain` 下一个导出的顶层工厂函数，若全仓没有任何
// 非测试调用点，它就进名单。**门禁不判断名单里的东西该不该在那儿**——只判断名单有没有变长。
//
// 为什么不判状态：本仓机制先行、实例后到，一个 PN 切片还没开工的能力，它的领域工厂本来就
// 应当没有生产调用点。要判「这一条是接过线后来烂了，还是切片根本还没到」，得知道该能力归
// 哪个切片、那切片开工没有——**那是产品判断，门禁拿不到，也不该替它拍**。棘轮不需要这个
// 输入：新增一条要有人写一行理由，而写理由的人在写的当下正好知道答案。
//
// 排除 `New*`：值对象与标识构造器是另一族，本票「边界」一节已将其列为后续。排除用前缀是
// 有意的，方向与「用前缀圈定范围」相反——**漏一个 `New*` 只多一行噪声让人看见，而漏一个
// 工厂就永远没人看见**。判一道护栏好不好不看它用什么写，看它漏的时候倒向哪一边。
//
// 守不住的三格，如实写在这里：
//   - 引用按标识符名认。同包内一个同名局部变量会被当成引用，于是那个工厂看起来「已接线」。
//     方向是少报，也就是门禁变安静——这一格没有便宜的堵法，只能写明。
//   - 只看 `internal/<上下文>/domain` 下的导出顶层函数。方法、未导出者、以及 domain 之外
//     的构造都不在网内。
//   - 反射与代码生成绕得过去。实测于 2026-08-21：生产领域代码零 `reflect` 使用、`internal`
//     下零 `go:generate`，所以此刻这一格是空的；它会不会变要靠人，门禁看不见。

// wiringEntry 是名单里的一条。键是**包 + 名**而不是裸名字：本仓已经出过跨包重名
// （`NewDispositionRequests` 等三个各在两个包里声明一次，那正是一次基线错数的成因），
// 而裸名字作键时两个不同函数会互相抵消——一增一减，名单长度不变，门禁不响。
type wiringEntry struct {
	pkg  string
	name string
}

func (entry wiringEntry) String() string { return entry.pkg + " " + entry.name }

type wiringSource struct {
	pkgDir string
	isTest bool
	syntax *ast.File
}

// collectDomainFactories 取一份语法树里的候选工厂声明。
//
// 判据是「顶层、导出、非 New 开头」。不看返回类型：`(领域类型, error)` 是本仓现有工厂的
// 惯常形状，但把它写进判据等于假设下一个工厂也长这样，而那是个会自己变假的假设。
func collectDomainFactories(syntax *ast.File, pkgDir string) []wiringEntry {
	var found []wiringEntry

	for _, decl := range syntax.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name == nil {
			continue
		}
		name := function.Name.Name
		if !ast.IsExported(name) || strings.HasPrefix(name, "New") {
			continue
		}
		found = append(found, wiringEntry{pkg: pkgDir, name: name})
	}
	return found
}

// countProductionReferences 数一个候选在非测试代码里被引用了几次，不含它自己的声明。
//
// 跨包调用在 Go 里一定写成 `包名.函数名`，所以包外只认选择器；包内认裸标识符。分开认是为了
// 少踩同名：一个别处的 `Evaluate` 方法不会被当成本包这个 `Evaluate` 的调用。
func countProductionReferences(sources []wiringSource, entry wiringEntry) int {
	count := 0

	for _, source := range sources {
		if source.isTest {
			continue
		}
		samePackage := source.pkgDir == entry.pkg

		ast.Inspect(source.syntax, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncDecl:
				// 声明自己不算引用；函数体仍要走下去，工厂之间会互相调用。
				if samePackage && typed.Recv == nil && typed.Name != nil && typed.Name.Name == entry.name {
					if typed.Body != nil {
						ast.Inspect(typed.Body, func(inner ast.Node) bool {
							if ident, ok := inner.(*ast.Ident); ok && ident.Name == entry.name {
								count++
							}
							return true
						})
					}
					return false
				}
			case *ast.SelectorExpr:
				if !samePackage && typed.Sel != nil && typed.Sel.Name == entry.name {
					count++
				}
				return true
			case *ast.Ident:
				if samePackage && typed.Name == entry.name {
					count++
				}
			}
			return true
		})
	}
	return count
}

// isScannedDomainPackage 复用本包已有的 isDomainPackage，另加一道 `internal/` 前缀：
// 平台件与仓根下若出现同名目录不在本门禁的网内，而 isDomainPackage 只看后缀。
func isScannedDomainPackage(pkgDir string) bool {
	return strings.HasPrefix(pkgDir, "internal/") && isDomainPackage(pkgDir)
}

func loadWiringSources(t *testing.T) []wiringSource {
	t.Helper()

	root := repositoryRoot(t)
	var sources []wiringSource

	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "docs", "backup", ".scratch", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		syntax, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		relative, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		sources = append(sources, wiringSource{
			pkgDir: filepath.ToSlash(relative),
			isTest: strings.HasSuffix(path, "_test.go"),
			syntax: syntax,
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历仓库：%v", walkErr)
	}
	if len(sources) == 0 {
		t.Fatal("没有找到 Go 源文件；接线棘轮会永远空跑")
	}
	return sources
}

// currentlyUnwiredFactories 算出此刻的名单。
func currentlyUnwiredFactories(sources []wiringSource) []wiringEntry {
	var candidates []wiringEntry
	for _, source := range sources {
		if source.isTest || !isScannedDomainPackage(source.pkgDir) {
			continue
		}
		candidates = append(candidates, collectDomainFactories(source.syntax, source.pkgDir)...)
	}

	var unwired []wiringEntry
	for _, candidate := range candidates {
		if countProductionReferences(sources, candidate) == 0 {
			unwired = append(unwired, candidate)
		}
	}
	sort.Slice(unwired, func(i, j int) bool { return unwired[i].String() < unwired[j].String() })
	return unwired
}

const wiringBaselineFile = "production_wiring_baseline.txt"

// readWiringBaseline 读基线。每行 `包 名`，`#` 起注释；空文件是合法的（表示不容忍任何一条）。
func readWiringBaseline(t *testing.T) map[wiringEntry]bool {
	t.Helper()

	raw, err := os.ReadFile(wiringBaselineFile)
	if err != nil {
		t.Fatalf("读基线 %s：%v", wiringBaselineFile, err)
	}

	baseline := make(map[wiringEntry]bool)
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
			t.Fatalf("%s 第 %d 行不是「包 名」两段：%q", wiringBaselineFile, index+1, line)
		}
		baseline[wiringEntry{pkg: fields[0], name: fields[1]}] = true
	}
	return baseline
}

// TestNoNewProductionFactoryGoesUnwired 名单只许变短。
//
// 新增一条不是不许，是要有人显式写进基线并留一行理由——**而理由由加它的人在加的当下写，
// 那时他正好知道这属哪个切片、为什么现在还不接**。这正是本门禁不做批量分类的原因。
func TestNoNewProductionFactoryGoesUnwired(t *testing.T) {
	baseline := readWiringBaseline(t)

	for _, entry := range currentlyUnwiredFactories(loadWiringSources(t)) {
		if !baseline[entry] {
			t.Errorf("%s 是导出的领域工厂，全仓无非测试调用点，而它不在 %s 里。"+
				"要么接上生产调用路径，要么把这一行加进基线并写明为什么现在不接：\n\t%s",
				entry, wiringBaselineFile, entry)
		}
	}
}

// TestWiringBaselineHasNoStaleEntry 基线不许烂。
//
// 只判增量的门禁有一个自带的坏处：函数被删了、改名了、搬包了，条目还躺在基线里，于是基线的
// 地面真相悄悄漂走而门禁一直绿。**一个存下来的数，人人信它而没人重导**——本仓已经在别处栽过
// 这一种。代价近乎为零：同一遍扫描就知道。
func TestWiringBaselineHasNoStaleEntry(t *testing.T) {
	sources := loadWiringSources(t)

	declared := make(map[wiringEntry]bool)
	for _, source := range sources {
		if source.isTest || !isScannedDomainPackage(source.pkgDir) {
			continue
		}
		for _, entry := range collectDomainFactories(source.syntax, source.pkgDir) {
			declared[entry] = true
		}
	}

	unwired := make(map[wiringEntry]bool)
	for _, entry := range currentlyUnwiredFactories(sources) {
		unwired[entry] = true
	}

	var stale []wiringEntry
	for entry := range readWiringBaseline(t) {
		if !declared[entry] {
			stale = append(stale, entry)
			continue
		}
		if !unwired[entry] {
			stale = append(stale, entry)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].String() < stale[j].String() })

	for _, entry := range stale {
		t.Errorf("%s 在基线里，但它此刻要么已不存在、要么已经有生产调用点了。"+
			"把这一行从 %s 剪掉——留着它，下一次真有东西退回未接线时名单长度不变，门禁不会红。",
			entry, wiringBaselineFile)
	}
}

// TestTheProductionWiringRatchetCanActuallyCatchAViolation 证这道门禁真能红。
//
// 本包其余门禁都带一个同名用例，理由一样：**一个从来不会失败的门禁比没有门禁更坑人，因为
// 它还会取信**。这里用合成源码逼出提取器与引用计数各自的边界，不重复跑全仓。
func TestTheProductionWiringRatchetCanActuallyCatchAViolation(t *testing.T) {
	parse := func(t *testing.T, source string) *ast.File {
		t.Helper()
		syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", source, 0)
		if err != nil {
			t.Fatalf("解析合成源码：%v", err)
		}
		return syntax
	}

	extraction := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "导出的顶层函数被取出",
			source: "package p\n\nfunc FormThing() error { return nil }\n",
			want:   []string{"FormThing"},
		},
		{
			name:   "New 开头的不算——值对象与标识构造是另一族",
			source: "package p\n\nfunc NewThingID() error { return nil }\n",
			want:   nil,
		},
		{
			name:   "未导出的不算",
			source: "package p\n\nfunc formThing() error { return nil }\n",
			want:   nil,
		},
		{
			name:   "方法不算——本门禁只认顶层构造",
			source: "package p\n\ntype T struct{}\n\nfunc (T) FormThing() error { return nil }\n",
			want:   nil,
		},
		{
			name:   "动词不在任何前缀集里的照样被取出——这正是宽网与白名单的分别",
			source: "package p\n\nfunc AssessSafeHandoff() error { return nil }\n",
			want:   []string{"AssessSafeHandoff"},
		},
	}

	for _, test := range extraction {
		t.Run(test.name, func(t *testing.T) {
			got := collectDomainFactories(parse(t, test.source), "internal/x/domain")
			if len(got) != len(test.want) {
				t.Fatalf("取出 %d 个，want %d 个：%v", len(got), len(test.want), got)
			}
			for index := range got {
				if got[index].name != test.want[index] {
					t.Fatalf("第 %d 个 = %s，want %s", index, got[index].name, test.want[index])
				}
			}
		})
	}

	entry := wiringEntry{pkg: "internal/x/domain", name: "FormThing"}
	declaration := wiringSource{
		pkgDir: "internal/x/domain",
		syntax: parse(t, "package domain\n\nfunc FormThing() error { return nil }\n"),
	}

	if got := countProductionReferences([]wiringSource{declaration}, entry); got != 0 {
		t.Fatalf("只有声明时引用数 = %d，want 0——否则每个工厂都会被算成已接线", got)
	}

	caller := wiringSource{
		pkgDir: "internal/x/application",
		syntax: parse(t, "package application\n\nimport \"x/domain\"\n\nfunc Do() error { return domain.FormThing() }\n"),
	}
	if got := countProductionReferences([]wiringSource{declaration, caller}, entry); got != 1 {
		t.Fatalf("包外选择器调用时引用数 = %d，want 1", got)
	}

	testOnly := wiringSource{
		pkgDir: "internal/x/domain",
		isTest: true,
		syntax: parse(t, "package domain\n\nfunc TestX() { _ = FormThing }\n"),
	}
	if got := countProductionReferences([]wiringSource{declaration, testOnly}, entry); got != 0 {
		t.Fatalf("只被测试引用时引用数 = %d，want 0——那正是本门禁要抓的形状", got)
	}

	// 键必须是包加名：两个不同包的同名函数不能互相抵消。
	if (wiringEntry{pkg: "internal/a/domain", name: "FormThing"}) == (wiringEntry{pkg: "internal/b/domain", name: "FormThing"}) {
		t.Fatal("包加名没有构成唯一键，跨包重名会让一增一减互相抵消")
	}
}
