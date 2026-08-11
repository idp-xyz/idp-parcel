// Package architecture 存放 Parcel 模块化单体的依赖门禁。规则写成测试，是为了让
// 越界在构建时失败，而不是等评审时有人看出来。
package architecture

import (
	"go/ast"
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

	for _, file := range selectSources(t, "领域包不得依赖基础设施", isDomainPackage) {
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
	ownedByAModule := func(pkg string) bool {
		_, owned := moduleOf(pkg, modules)
		return owned
	}

	for _, file := range selectSources(t, "业务模块不得互相伸手", ownedByAModule) {
		if isCrossContextAdapter(file.pkg, modules) {
			continue
		}
		owner, _ := moduleOf(file.pkg, modules)
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

	for _, file := range selectSources(t, "共享技术设施不得依赖业务上下文", isSharedPlatform) {
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

// firstPartyTestScaffoldingDirectory 是上一条规则的检测侧在磁盘上的位置。
const firstPartyTestScaffoldingDirectory = "internal/platform/pgtest"

// TestFirstPartyTestScaffoldingStillExists 把上一条的检测侧钉到磁盘上。
//
// 上一条的过滤器是否定式，永远筛不空，所以它不会像别的规则那样扫零恒绿；它的静默发生在
// 检测侧：pgtest 一改名或搬走，isFirstPartyTestScaffolding 对任何导入路径都不再命中，规则
// 照绿，而生产包从此可以随便导入那个改了名的脚手架包。
//
// TestTestScaffoldingRecognitionDoesNotCatchSiblings 拦不住这件事——它比对的是字符串常量，
// `/internal/platform/pgtest` 判 true 是永真的字符串事实，目录删了它也真。它偏偏是唯一让人
// 觉得上一条还活着的东西，因此更需要这道锚。
//
// 脚手架包真被删掉时这条会红，那是对的：判据没有实例了，上一条规则就该跟着重新审视，而不是
// 继续摆在那里看起来像在守。
func TestFirstPartyTestScaffoldingStillExists(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(repositoryRoot(t), filepath.FromSlash(firstPartyTestScaffoldingDirectory))
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("%s 不在了；isFirstPartyTestScaffolding 的检测侧从此对任何导入路径都不命中，上一条规则会静默失效",
			firstPartyTestScaffoldingDirectory)
	}

	pkg := modulePath + "/" + firstPartyTestScaffoldingDirectory
	if !isFirstPartyTestScaffolding(pkg) {
		t.Fatalf("isFirstPartyTestScaffolding 认不出磁盘上真实存在的脚手架包 %q；判据与目录已经对不上", pkg)
	}
}

func TestProductionPackagesDoNotImportTheFrameworkTestkit(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		for _, imported := range file.imports {
			// 补 `/` 边界，与本文件其余判据同口径：裸前缀会让 `…/testkitchen` 这样
			// 只是恰好同前缀的包报出假阳性，而假阳性会逼人给规则开例外，例外正是门禁
			// 失效的常见起点。
			if imported == frameworkTestkit || strings.HasPrefix(imported, frameworkTestkit+"/") {
				t.Errorf("%s：生产包导入 %q", file.path, imported)
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
	ownedByAModule := func(pkg string) bool {
		_, owned := moduleOf(pkg, modules)
		return owned
	}

	for _, file := range selectSources(t, "业务包不得直接碰驱动", ownedByAModule) {
		if isPersistenceAdapter(file.pkg) {
			continue
		}
		module, _ := moduleOf(file.pkg, modules)
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

	for _, file := range selectSources(t, "应用包不得依赖适配器", isApplicationPackage) {
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

	for _, file := range selectSources(t, "入站 HTTP 适配器不得直达持久化", isHTTPAdapter) {
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

// TestPersistenceAdapterExemptionDoesNotCatchSiblings 与上两条同理。这条豁免曾经写成
// 子串匹配，兄弟包一律白拿——分类器的伴生用例正是为了让这种事在改判据的当场变红。
func TestPersistenceAdapterExemptionDoesNotCatchSiblings(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"/internal/parcelshipment/adapters/postgres":        true,
		"/internal/parcelshipment/adapters/postgres/outbox": true,
		"/internal/parcelshipment/adapters/postgresutil":    false,
		"/internal/parcelshipment/adapters/postgres_legacy": false,
		"/internal/parcelshipment/adapters/http":            false,
		"/internal/parcelshipment/application":              false,
		"/internal/partycommercial/adapters/parcelshipment": false,
	}

	for suffix, want := range cases {
		if got := isPersistenceAdapter(modulePath + suffix); got != want {
			t.Errorf("%s 判为持久化适配器 %v，应为 %v", suffix, got, want)
		}
	}
}

// TestSharedPlatformRecognitionDoesNotCatchSiblings 与上三条同理。isSharedPlatform 的注释
// 声称它按段匹配是为了不把 `platformops` 误认成 platform 本尊，这条把那句声称变成用例——
// 否则判据改回前缀匹配也不会有任何东西变红。
func TestSharedPlatformRecognitionDoesNotCatchSiblings(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"/internal/platform":               true,
		"/internal/platform/pgtest":        true,
		"/internal/platform/migrate":       true,
		"/internal/platformops/domain":     false,
		"/internal/parcelshipment/domain":  false,
		"/internal/partycommercial/domain": false,
	}

	for suffix, want := range cases {
		if got := isSharedPlatform(modulePath + suffix); got != want {
			t.Errorf("%s 判为共享技术设施 %v，应为 %v", suffix, got, want)
		}
	}
}

// TestModulePathMatchesGoMod 把 modulePath 钉到 go.mod 上。
//
// 本包大半条规则的两侧不对称：过滤器侧的包名由这个常量拼出来（loadSources 用
// filepath.Join(modulePath, relative)），检测侧的导入路径却是从源码真读的。模块路径一改
// ——升 /v2 是最现实的那种——过滤器照常命中、检测侧全部失配，五条规则同时静默变绿，而
// go build 不会有任何反应。一个常量漂移带走五条门禁，这是本包里最便宜的一道锚。
func TestModulePathMatchesGoMod(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("读 go.mod：%v", err)
	}

	declared := ""
	for _, line := range strings.Split(string(contents), "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "module "); found {
			declared = strings.TrimSpace(rest)
			break
		}
	}
	if declared != modulePath {
		t.Fatalf("go.mod 声明的模块是 %q，本包常量 modulePath 是 %q；两者不一致时门禁的检测侧会全面失配而不报",
			declared, modulePath)
	}
}

func isDomainPackage(pkg string) bool {
	return strings.HasSuffix(pkg, "/domain") || strings.Contains(pkg, "/domain/")
}

// isSharedPlatform 按段匹配而不是前缀匹配：`internal/platformops` 之类的名字用前缀会被
// 误认成 platform 本尊，而它按黑名单口径本该算业务上下文，于是它自己的 application 导入
// 自己的 domain 就会报出一条读起来毫无道理的假阳性。
func isSharedPlatform(pkg string) bool {
	segments, ok := internalSegments(pkg)
	return ok && segments[0] == "platform"
}

// isApplicationPackage 后缀与中缀都要认：`application/子包` 同样是应用层，只判后缀会让
// 它滑过去。
func isApplicationPackage(pkg string) bool {
	return strings.HasSuffix(pkg, "/application") || strings.Contains(pkg, "/application/")
}

// isHTTPAdapter 用子串而不是分段，与 isPersistenceAdapter 相反——因为它用在**过滤器**上
// 而不是豁免上。两者的失败方向不同：过滤器放宽顶多多扫几个包（`adapters/httpx` 被一并
// 查一遍，假阳性），豁免放宽是把包整个摘出规则之外（假阴性，没有人发现得了）。这里宁可
// 多扫。
func isHTTPAdapter(pkg string) bool {
	return strings.Contains(pkg, "/adapters/http")
}

// isPersistenceAdapter 按段匹配，与第八、九条同一口径。
//
// 不用子串：`adapters/postgresutil`、`adapters/postgres_legacy` 这类兄弟包都含
// `/adapters/postgres`，一个子串豁免就把它们整包摘出去，此后它们自建 pgxpool 绕过框架
// 事务保证，没有任何东西会变红。**过滤器放宽与豁免放宽不是一回事**：前者多扫几个文件，
// 顶多假阳性；后者是假阴性，而假阴性在门禁上没有人发现得了。
func isPersistenceAdapter(pkg string) bool {
	segments, ok := internalSegments(pkg)
	if !ok || len(segments) < 3 {
		return false
	}
	return segments[1] == "adapters" && segments[2] == "postgres"
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

// unfilteredSourceConsumers 是允许绕过 selectSources、直接向 loadSources 取数的函数，每条写明
// 它为什么不需要筛空守卫。
//
// 不一刀切禁掉直接取数，是因为「没有过滤器」与「过滤器筛空了」是两回事：无过滤器的规则本就
// 没有可筛空的东西，逼它经 selectSources 只会要求它编一个恒真的过滤器。要挡的是**照抄现成先例**
// ——同一个文件里摆着两个直接调用的样板，下一条规则照着写，筛空守卫就这么悄悄没了。名单把
// 「照抄」变成「得写一句理由」。
var unfilteredSourceConsumers = map[string]string{
	"selectSources": "它自己就是那道守卫。",
	"TestProductionPackagesDoNotImportFirstPartyTestScaffolding": "判据是否定式的：跳过脚手架包本身、扫其余全部，剩下的集合不可能空。它的漂移风险在脚手架包自己会不会消失，那一格由 TestFirstPartyTestScaffoldingStillExists 的磁盘锚守着。",
	"TestProductionPackagesDoNotImportTheFrameworkTestkit":       "没有过滤器，对全部生产文件一视同仁，没有可筛空的东西。",
}

// TestNoGateTakesSourcesWithoutEitherFilteringOrSayingWhy 让 selectSources 那句「必须」真的有
// 东西强制。
//
// 在这条测试之前，那句话只写在注释里，而同一个文件里已经躺着两个直接调 loadSources 的先例——
// 一条约束写进注释、再让门禁依赖那个约定，正是本包反复批评的那个错，只不过这回发生在它自己
// 身上。下一条新规则照着先例写，筛空守卫不会有任何东西提醒它没了。
//
// 扫本包全部 `_test.go` 而不只是本文件：新门禁常常另起一个文件（`rehydration_gate_test.go`、
// `enum_exhaustiveness_test.go` 都是），只钉住本文件等于给「换个文件写」留了同一个出口。
func TestNoGateTakesSourcesWithoutEitherFilteringOrSayingWhy(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, path := range architectureTestFiles(t) {
		syntax, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s：%v", path, err)
		}
		for _, decl := range syntax.Decls {
			function, isFunction := decl.(*ast.FuncDecl)
			if !isFunction || function.Body == nil || !callsLoadSources(function.Body) {
				continue
			}
			seen[function.Name.Name] = true
			if _, allowed := unfilteredSourceConsumers[function.Name.Name]; !allowed {
				t.Errorf("%s：%s 直接向 loadSources 取数而不经 selectSources；带过滤器就改用 selectSources，确实不带过滤器就补进 unfilteredSourceConsumers 并写明为什么筛不空",
					filepath.Base(path), function.Name.Name)
			}
		}
	}

	for name, reason := range unfilteredSourceConsumers {
		if !seen[name] {
			t.Errorf("unfilteredSourceConsumers 里的 %q 已经不直接调 loadSources 了；名单该清理", name)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("unfilteredSourceConsumers 里的 %q 没写为什么不需要筛空守卫", name)
		}
	}
	// 连 selectSources 自己都没扫到，说明取数那条链已经改了形状，而上面整段仍旧全绿。
	if !seen["selectSources"] {
		t.Fatal("没扫到 selectSources 调 loadSources；本条门禁的扫描面已经失效")
	}
}

// architectureTestFiles 列出本包的测试文件。它们是 `_test.go`，loadSources 与
// parseRepositorySources 都排掉了，只能自己取。
func architectureTestFiles(t *testing.T) []string {
	t.Helper()

	pattern := filepath.Join(repositoryRoot(t), "internal", "architecture", "*_test.go")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("列 %s：%v", pattern, err)
	}
	if len(matches) == 0 {
		t.Fatalf("没找到 %s；本包已改名或搬家，钉着它的规则会永远空过", pattern)
	}
	return matches
}

func callsLoadSources(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if identifier, isIdent := call.Fun.(*ast.Ident); isIdent && identifier.Name == "loadSources" {
			found = true
		}
		return !found
	})
	return found
}

// selectSources 取出包名满足 matches 的源文件，一个都没选中时当场判红。
//
// **凡是带过滤器的扫描型门禁都必须经这里取数**，守卫才不可能被下一条规则忘掉。loadSources 自己
// 那道 len(files)==0 只守「全仓一个 Go 文件都没有」，守不住「本条规则**自己的**过滤器筛出零个
// 文件」——而后者才是现实会发生的那种：过滤器的判据全都靠目录命名约定（`/domain`、
// `platform`、`/application`、`/adapters/http`），而没有任何东西强制这些名字。目录一改名
// 或一搬家，规则扫零、恒绿，且没有任何东西会报。一条扫不到任何东西的门禁比没有门禁更坏，
// 它看起来像在守。
//
// 「必须」这两个字由 TestNoGateTakesSourcesWithoutEitherFilteringOrSayingWhy 强制，不由本段注释
// 强制。不带过滤器的规则不在此列，它们直接调 loadSources 是对的——名单与理由见
// unfilteredSourceConsumers。
func selectSources(t *testing.T, rule string, matches func(pkg string) bool) []sourceFile {
	t.Helper()

	var selected []sourceFile
	for _, file := range loadSources(t) {
		if matches(file.pkg) {
			selected = append(selected, file)
		}
	}
	if len(selected) == 0 {
		t.Fatalf("「%s」的过滤器一个文件都没选中；判据依赖的目录约定多半已经变了，这条门禁会永远空过", rule)
	}
	return selected
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
