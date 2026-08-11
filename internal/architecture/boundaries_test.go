// Package architecture 存放 Parcel 模块化单体的依赖门禁。规则写成测试，是为了让
// 越界在构建时失败，而不是等评审时有人看出来。
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "go.idp.xyz/idp-parcel"

// infrastructureImports 一律不得进入领域包。领域类型只用标准库和框架的领域事件
// 合同表达。
var infrastructureImports = []string{
	"github.com/jackc/pgx",
	"github.com/go-chi/chi",
	"database/sql",
	"net/http",
	"log/slog",
	"os",
	"go.idp.xyz/idp-bento-go/postgres",
	"go.idp.xyz/idp-bento-go/eventing",
}

// nonBusinessDirectories 是 internal/ 下不表达限界上下文的目录。
//
// 用黑名单而不是列举业务模块，是因为白名单漏登会让新上下文静默逃出全部规则：
// 目录建出来了，moduleOf 却认不出它，跨模块与驱动那两条便对它不生效，且没有
// 任何东西会报警。[CONTEXT-MAP](../../docs/domain/CONTEXT-MAP.md) 还有几个上下文
// 尚未落成目录，反过来写才能让它们一出现就默认受护。
var nonBusinessDirectories = map[string]bool{
	"platform":     true,
	"architecture": true,
}

// frameworkTestkit 一律不得进入生产包。这是 `PBC-08` 的负向门禁：框架的消费者
// 合同要求生产依赖图排除 testkit。测试文件不受此限，但那是因为 loadSources
// 排掉了 `_test.go`，不是这条规则给了豁免——本规则对它扫到的每个文件一视同仁。
const frameworkTestkit = "go.idp.xyz/idp-bento-go/testkit"

type sourceFile struct {
	pkg     string
	path    string
	imports []string
}

func TestDomainPackagesDoNotDependOnInfrastructure(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if !isDomainPackage(file.pkg) {
			continue
		}
		for _, imported := range file.imports {
			for _, forbidden := range infrastructureImports {
				if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
					t.Errorf("%s：领域包导入基础设施 %q", file.path, imported)
				}
			}
			if strings.Contains(imported, "/adapters/") {
				t.Errorf("%s：领域包导入适配器 %q", file.path, imported)
			}
		}
	}
}

// TestBusinessModulesDoNotReachIntoEachOther 守的是限界上下文的语言与数据所有权。
// ADR-0025 的「只有适配器包可以同时导入两个上下文」把许可只给适配器，`domain`、
// `application`、`ports` 一律不得跨界；「跨上下文调用的适配器落在消费侧，路径为
// `internal/<consumer>/adapters/<provider>/`」进一步定死了位置。这条规则挡的就是
// 绕开那个位置的引用。
func TestBusinessModulesDoNotReachIntoEachOther(t *testing.T) {
	t.Parallel()

	modules := businessModules(t)
	for _, file := range loadSources(t) {
		owner, owned := moduleOf(file.pkg, modules)
		if !owned || isCrossContextAdapter(file.pkg, modules) {
			continue
		}
		for _, imported := range file.imports {
			target, targeted := moduleOf(imported, modules)
			if !targeted || target == owner {
				continue
			}
			t.Errorf("%s：模块 %q 经 %q 伸进模块 %q 的内部；跨上下文翻译只能落在 internal/%s/adapters/%s/",
				file.path, owner, imported, target, owner, target)
		}
	}
}

// TestSharedPlatformDoesNotDependOnBusinessContexts 守共享管道的方向。
// [决策简报](../../docs/design/parcel-go-first-consumer-slice-decision-brief.md)
// 规定 `internal/platform` 只放进程级共享技术设施——它一旦导入某个上下文，就不再
// 是共享的了：其余每个上下文都会经由它继承对那一个上下文的依赖，而拆开要动所有
// 使用者。反方向（上下文导入 platform）正常，不在此限。
//
// 接线不受影响：把处理器挂上路由是 `cmd/` 的职责，`cmd/` 不受本规则约束。
func TestSharedPlatformDoesNotDependOnBusinessContexts(t *testing.T) {
	t.Parallel()

	modules := businessModules(t)

	for _, file := range loadSources(t) {
		// 按段匹配而不是前缀匹配：`internal/platformops` 之类的名字用前缀会被误认成
		// platform 本尊，而它按黑名单口径本该算业务上下文，于是它自己的 application
		// 导入自己的 domain 就会报出一条读起来毫无道理的假阳性。
		segments, ok := internalSegments(file.pkg)
		if !ok || segments[0] != "platform" {
			continue
		}
		for _, imported := range file.imports {
			if target, targeted := moduleOf(imported, modules); targeted {
				t.Errorf("%s：共享技术设施经 %q 依赖业务上下文 %q",
					file.path, imported, target)
			}
		}
	}
}

// isFirstPartyTestScaffolding 认出本仓自己的测试脚手架包 `internal/platform/pgtest`
// 及其子包。它必须是普通包而非 `_test.go`，因为跨包共用的测试助手只能这么写；
// 代价是它把 `testing` 拉进任何 import 它的二进制。
//
// 按段匹配而不是前缀匹配，理由与共享管道那条相同：`pgtestutil` 之类的兄弟包用
// 前缀会被误认成 pgtest 本尊，从而白拿「自身除外」并绕开本规则。
func isFirstPartyTestScaffolding(pkg string) bool {
	segments, ok := internalSegments(pkg)
	if !ok || len(segments) < 2 {
		return false
	}
	return segments[0] == "platform" && segments[1] == "pgtest"
}

// TestProductionPackagesDoNotImportFirstPartyTestScaffolding 是上一条针对框架
// `testkit` 那条规则的对称一半。挡住第三方的测试脚手架却放行自己造的同类，等于
// 把同一个风险换个来源重新放进来：`testing` 一旦被链入生产二进制，`-test.*`
// 整套 flag 都会被注册。
//
// 不写成「非测试文件不得 import testing」——那会当场报 pgtest 自己，而它必须
// import。要挡的不是它存在，是它被生产包引用。`loadSources` 已排掉 `_test.go`，
// 因此本规则的实际效果恰好是「只有测试文件能用 pgtest」，不需另开豁免。
func TestProductionPackagesDoNotImportFirstPartyTestScaffolding(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if isFirstPartyTestScaffolding(file.pkg) {
			continue
		}
		for _, imported := range file.imports {
			if isFirstPartyTestScaffolding(imported) {
				t.Errorf("%s：生产包导入测试脚手架 %q；它只能被 _test.go 使用",
					file.path, imported)
			}
		}
	}
}

func TestProductionPackagesDoNotImportTheFrameworkTestkit(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, frameworkTestkit) {
				t.Errorf("%s：生产包导入 %q", file.path, frameworkTestkit)
			}
		}
	}
}

// TestBusinessPackagesDoNotTouchTheDriverDirectly 证写入一律经框架事务，而不是
// 裸的 pgx 事务。框架合同要求写事务参与者在没有事务句柄时报错而非改用连接池，
// 业务包直接拿到驱动就绕过了这条保证。
func TestBusinessPackagesDoNotTouchTheDriverDirectly(t *testing.T) {
	t.Parallel()

	modules := businessModules(t)
	for _, file := range loadSources(t) {
		module, owned := moduleOf(file.pkg, modules)
		if !owned || strings.Contains(file.pkg, "/adapters/postgres") {
			continue
		}
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, "github.com/jackc/pgx") {
				t.Errorf("%s：模块 %q 经 %q 在持久化适配器之外碰到驱动",
					file.path, module, imported)
			}
		}
	}
}

func TestApplicationPackagesDoNotDependOnAdapters(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		// 后缀与中缀都要认：`application/子包` 同样是应用层，只判后缀会让它滑过去。
		if !strings.HasSuffix(file.pkg, "/application") && !strings.Contains(file.pkg, "/application/") {
			continue
		}
		for _, imported := range file.imports {
			if strings.Contains(imported, "/adapters/") {
				t.Errorf("%s：应用包导入适配器 %q", file.path, imported)
			}
			if strings.HasPrefix(imported, "net/http") || strings.HasPrefix(imported, "github.com/go-chi") {
				t.Errorf("%s：应用包导入传输层 %q", file.path, imported)
			}
		}
	}
}

func TestHTTPAdaptersDoNotExecuteSQL(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if !strings.Contains(file.pkg, "/adapters/http") {
			continue
		}
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, "github.com/jackc/pgx") ||
				imported == "database/sql" ||
				strings.Contains(imported, "/adapters/postgres") {
				t.Errorf("%s：入站 HTTP 适配器经 %q 直达持久化", file.path, imported)
			}
		}
	}
}

// TestCrossContextAdapterExemptionCoversOnlyTheDesignatedPosition 直接测豁免的
// 分类逻辑。全仓还没有跨上下文适配器，跨模块规则今天对所有包都判过——豁免这条
// 分支一次也没被走到，光看它绿说明不了它判得对。等第一个适配器落地才发现放宽了
// 一格，那时错误的引用已经写下了。
func TestCrossContextAdapterExemptionCoversOnlyTheDesignatedPosition(t *testing.T) {
	t.Parallel()

	modules := map[string]bool{"parcelshipment": true, "partycommercial": true}
	cases := map[string]bool{
		// ADR-0025 指定的那个位置，及其子包。
		"/internal/parcelshipment/adapters/partycommercial":      true,
		"/internal/parcelshipment/adapters/partycommercial/asof": true,
		// 与它并列但翻译的不是上下文，不得拿到跨上下文豁免。
		"/internal/parcelshipment/adapters/http":     false,
		"/internal/parcelshipment/adapters/postgres": false,
		// 非适配器层一律不豁免。
		"/internal/parcelshipment/application": false,
		"/internal/parcelshipment/ports":       false,
		"/internal/parcelshipment/domain":      false,
		// 末段不是业务模块名，只是恰好叫 adapters 的目录。
		"/internal/parcelshipment/adapters": false,
	}

	for suffix, want := range cases {
		if got := isCrossContextAdapter(modulePath+suffix, modules); got != want {
			t.Errorf("%s 的跨上下文豁免判为 %v，应为 %v", suffix, got, want)
		}
	}
}

// TestTestScaffoldingRecognitionDoesNotCatchSiblings 与上一条同理：今天全仓只有
// pgtest 一个脚手架包，「自身除外」这条分支只被它自己走过，光看规则绿说明不了它
// 认得准。兄弟包被误认会白拿豁免，那是一个不会有人发现的假阴性。
func TestTestScaffoldingRecognitionDoesNotCatchSiblings(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"/internal/platform/pgtest":          true,
		"/internal/platform/pgtest/fixtures": true,
		"/internal/platform/pgtestutil":      false,
		"/internal/platform/migrate":         false,
		"/internal/parcelshipment/domain":    false,
	}

	for suffix, want := range cases {
		if got := isFirstPartyTestScaffolding(modulePath + suffix); got != want {
			t.Errorf("%s 判为脚手架 %v，应为 %v", suffix, got, want)
		}
	}
}

func isDomainPackage(pkg string) bool {
	return strings.HasSuffix(pkg, "/domain") || strings.Contains(pkg, "/domain/")
}

// businessModules 从 internal/ 的实际目录派生限界上下文根，减去不表达上下文的那几个。
func businessModules(t *testing.T) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), "internal"))
	if err != nil {
		t.Fatalf("读取 internal/：%v", err)
	}

	modules := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() && !nonBusinessDirectories[entry.Name()] {
			modules[entry.Name()] = true
		}
	}
	if len(modules) == 0 {
		t.Fatal("internal/ 下没有业务模块；跨模块规则会永远空过")
	}
	return modules
}

func moduleOf(pkg string, modules map[string]bool) (string, bool) {
	segments, ok := internalSegments(pkg)
	if !ok || !modules[segments[0]] {
		return "", false
	}
	return segments[0], true
}

// internalSegments 取 internal/ 之后的路径分段，例如
// `…/internal/parcelshipment/adapters/partycommercial` 得到
// [parcelshipment adapters partycommercial]。
func internalSegments(pkg string) ([]string, bool) {
	prefix := modulePath + "/internal/"
	if !strings.HasPrefix(pkg, prefix) {
		return nil, false
	}
	return strings.Split(strings.TrimPrefix(pkg, prefix), "/"), true
}

// isCrossContextAdapter 判断某包是不是 [ADR-0025](../../docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md)
// 所定的跨上下文翻译位置，即「路径为 `internal/<consumer>/adapters/<provider>/`」。
//
// 只认这一种形状，不是「路径里含 /adapters/ 就放行」：`adapters/http` 与
// `adapters/postgres` 与它并列，但它们翻译的是传输与持久化，不是另一个上下文，
// 拿到跨上下文豁免就等于给了两条本不该有的越界通道。
//
// 注意本判断认的是位置，不是方向。`internal/A/adapters/B` 这个形状对称，单看
// 路径分不出 A 消费 B 还是 B 消费 A，因此它拦不住 ADR-0025 已否决的提供方侧
// 适配器。导入集合同样分不出：两侧适配器都会同时导入两个上下文，一边取对方结论、
// 一边造本方类型。真正的判据是「适配器满足的接口声明在 `internal/<owner>/ports`」，
// 而 Go 的接口是结构型满足，判它需要 `go/types` 与 `types.Implements`，本文件这套
// `parser.ImportsOnly` 扫描做不到。全仓还没有任何跨上下文适配器，那条规则也就没有
// 实例可验，先不写，留到第一个适配器落地时按真实形状补。
func isCrossContextAdapter(pkg string, modules map[string]bool) bool {
	segments, ok := internalSegments(pkg)
	if !ok || len(segments) < 3 {
		return false
	}
	return segments[1] == "adapters" && modules[segments[2]]
}

// loadSources 解析仓库的非测试 Go 文件。排除测试文件，是为了不让一个仅测试用的
// 依赖去判负生产边界规则。
func loadSources(t *testing.T) []sourceFile {
	t.Helper()

	root := repositoryRoot(t)
	var files []sourceFile

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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		file := sourceFile{
			pkg:  filepath.ToSlash(filepath.Join(modulePath, relative)),
			path: filepath.ToSlash(relative) + "/" + filepath.Base(path),
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			file.imports = append(file.imports, imported)
		}
		files = append(files, file)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历仓库：%v", walkErr)
	}
	if len(files) == 0 {
		t.Fatal("没有找到 Go 源文件；边界门禁会永远空过")
	}
	return files
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录：%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("测试工作目录之上找不到 go.mod")
		}
		directory = parent
	}
}
