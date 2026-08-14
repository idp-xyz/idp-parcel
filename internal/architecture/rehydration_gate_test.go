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

const parcelShipmentDomain = modulePath + "/internal/parcelshipment/domain"

// rehydrationEntryIdentifiers 是 [ADR-0028](../../docs/adr/0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)
// 所定重建面上**受限**的那一半：它是本包唯一一处「相信输入」的地方，只对持久化适配器开放。
var rehydrationEntryIdentifiers = map[string]bool{
	"RehydrateShipmentRequest":        true,
	"RehydrateShipmentRequestSpec":    true,
	"RehydrateSubmissionVersionSpec":  true,
	"RehydrateAcceptanceTaskSpec":     true,
	"RehydratePriorRequestLinkSpec":   true,
	"RehydrateParcelFinalOutcome":     true,
	"RehydrateParcelFinalOutcomeSpec": true,
}

// rehydrationSurfaceOpenIdentifiers 是重建面上**有意对所有调用方开放**的那一半，每条写明为什么。
//
// 分成两份名单，而不是让判据按声明种类把 var/const 排除在重建面之外，是 domain owner 的裁断：
// 该分的维度是意图，不是「提供方那边是个什么东西」。两者今天恰好重合——重建面上唯一的 var
// 恰好就该开放——明天就不重合，而按种类排除会连同**门禁本来管得住的东西**一起放掉：
// `domain.SomeConst` 与 `domain.RehydrateShipmentRequest` 一样是选择器，rehydrationReferencesIn
// 现在就拦得住它。真出现一个必须只在门里用的常量时，它该进上面那份受限名单，不需要任何新机制。
//
// 与 ADR-0029 是同一种分格法：按消费方的恢复动作分，不按提供方观察到的原因分。
var rehydrationSurfaceOpenIdentifiers = map[string]string{
	"ErrInvalidRehydratedShipmentRequest": "重建拒绝的哨兵。errors.Is 它什么聚合都没造出来，落在 ADR-0028 要防的「业务代码断言判断路径造不出的结论」之外；放进受限名单等于禁止任何调用方判断这个拒绝。",
	"ErrRehydrationStateNotSupported":     "同上，另一个拒绝理由：这扇门本期只开到`已提交`。它与上一条按 ADR-0029 分格——恢复动作不同（等门开到那个状态，还是去查库里那一行），压成一个取值会让运维照着完好的数据去找一处不存在的损坏。分期期间编排要靠它分流，更不能拦。",
	"SubmitShipmentRequest":               "构造那扇门，按 ADR-0028 与重建分属两扇：它从零版本创生，每一条不变量都当场算，因此不需要受限——受限要防的是「相信输入」，而它谁的话都不信。它被判据四（凭空交出一个聚合）认进重建面是对的：两扇门都得登记，只有登记了，第三扇门被人加出来时才会因为「两份名单都没有」而变红。",
}

// rehydrationSurfaceFile 是 ADR-0028 那扇「另一扇门」在磁盘上的位置。
const rehydrationSurfaceFile = "internal/parcelshipment/domain/rehydration.go"

// surfaceCandidate 是一个待判定的领域包声明：它在哪个文件、叫什么、（是方法时）挂在哪个
// 类型上，以及它会不会凭空交出一个聚合。
type surfaceCandidate struct {
	path         string
	name         string
	receiverType string
	// yieldsAggregate 只对函数与方法有意义，其余声明恒为假。
	yieldsAggregate bool
}

// belongsToRehydrationSurface 判领域包里一个导出声明是否属重建面。三条取并集：在门那个
// 文件里、名字以 `Rehydrate` 开头、或者它是挂在某个受限类型上的方法。任何一条单独承重，
// 都会把一个约定变成门禁的唯一支点。
//
// 单靠名字形状时，覆盖面的支点是「下一个人会照着起名」。这条支点是三连静默的：形状扫不到
// 新入口 → 它进不了 rehydrationEntryIdentifiers → 而 rehydrationReferencesIn 正是拿那份
// 名单去查调用点的，于是业务代码调它也不会变红。一个叫 `RestoreShipmentRequest` 的入口
// 就这样从三道检查底下一起穿过去——而 ADR-0028 恰恰否决过这个名字，理由是「约束只在注释
// 里等于没有约束」。把约定写进注释再让门禁依赖它，是同一个错换了个位置犯。
//
// 第三条补的是前两条合起来仍然漏掉的那一格：`func (spec RehydrateShipmentRequestSpec)
// Build() (ShipmentRequest, error)` 写在 `shipment_request.go` 里，文件那半不中、名字那半
// 也不中，于是它压根进不了覆盖检查——它是货真价实的第二个重建入口，而下面那条方法禁令
// 因为判据不认它而无从触发。按接收者类型认，才是认「方法这个形状」而不只是「写在门里的
// 方法」。
//
// 不取 `Snapshot` 后缀：领域包里 `CommercialBasisSnapshot`、`AmendmentAuthoritySnapshot`
// 是判断的值对象，与重建无关。按后缀判会把这两个算进重建面，真入口反而一个都不中。
//
// 第四条与前三条不同类：前三条认的都是**形状**（写在哪个文件、叫什么名字、挂在哪个类型上），
// 而真正赋予重建能力的是**包成员资格**——只要在 `package domain` 里就设得了未导出字段。三条
// 形状判据合起来仍漏掉 `func (spec SomeSpec) Build() ShipmentRequest`：接收者不在受限名单，
// 文件与名字两半也不中，于是四道检查全穿（实测于 1dbc680，见对照组——同一个方法只把接收者
// 换成受限类型就在下面那条方法禁令上变红）。所以第四条按能力认：凭空交出一个聚合。
//
// 这不是新裁断，是把 rehydrationSurfaceOpenIdentifiers 上方那条既有裁断补齐——「该分的维度
// 是意图，不是提供方那边是个什么东西」当初只贯彻到了分类（两份互斥名单），没贯彻到检测。
// 检测按能力、分类按意图，`SubmitShipmentRequest` 因此落进开放名单：那不是误伤，恰恰因为
// 构造与重建是 ADR-0028 定的两扇门，每扇都得把自己的义务写下来。
func belongsToRehydrationSurface(candidate surfaceCandidate) bool {
	return candidate.path == rehydrationSurfaceFile ||
		strings.HasPrefix(candidate.name, "Rehydrate") ||
		rehydrationEntryIdentifiers[candidate.receiverType] ||
		candidate.yieldsAggregate
}

// aggregateTypeName 是本上下文的聚合根，也是上面第四条判据的落点。
const aggregateTypeName = "ShipmentRequest"

// yieldsTheAggregateWithoutTakingOne 判一条函数声明会不会凭空造出一个聚合。
//
// 接收者与入参一并算作「收了一个进来」：`func (request ShipmentRequest) Decide(…) (ShipmentRequest, error)`
// 是一次转移而不是一扇门，它由转移扫描守（那一条要求版本不动），在这里认成重建面会把领域包
// 里每个状态转移都拖上来，而它们一个都不该登记进两份名单。
func yieldsTheAggregateWithoutTakingOne(function *ast.FuncDecl) bool {
	return mentionsTheAggregate(function.Type.Results) &&
		!mentionsTheAggregate(function.Type.Params) &&
		!mentionsTheAggregate(function.Recv)
}

// mentionsTheAggregate 判一份字段表里有没有提到聚合类型。指针、切片、映射都算——递出去的是
// 同一个类型，包了一层不改变调用方能拿到它这件事。
func mentionsTheAggregate(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	found := false
	ast.Inspect(fields, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == aggregateTypeName {
			found = true
		}
		return !found
	})
	return found
}

// receiverTypeName 取方法接收者的类型名，指针与泛型实例都剥到底层那个标识符。
func receiverTypeName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return ""
	}
	expression := function.Recv.List[0].Type
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.IndexListExpr:
			expression = typed.X
		case *ast.Ident:
			return typed.Name
		default:
			return ""
		}
	}
}

// mayUseRehydrationEntry 只认 `internal/parcelshipment/adapters/postgres` 及其子包。
//
// 按段匹配而不是前缀匹配，与第八、九条同一口径：前缀会让 `postgresutil` 之类的兄弟包
// 白拿豁免。
func mayUseRehydrationEntry(pkg string) bool {
	segments, ok := internalSegments(pkg)
	if !ok || len(segments) < 3 {
		return false
	}
	return segments[0] == "parcelshipment" && segments[1] == "adapters" && segments[2] == "postgres"
}

// TestOnlyThePersistenceAdapterUsesTheRehydrationEntry 守 ADR-0028「隔离靠门禁，不靠注释」。
//
// **为什么不是导入门禁。**ADR-0028 Decision 里「隔离靠导入门禁，不靠注释」那一条写的是
// 「重建入口所在的包，只允许持久化适配器及其测试导入」，但那个手段在 Go 里对这个入口不可
// 表达，人类已裁定改用引用粒度、且不另立记录——那一条承重的是约束与强度（只允许持久化
// 适配器及其测试、由机器强制、注释拦不住下一个人），三样一样不少，变的只是扫描粒度。下面
// 是当时排除掉的路，写在这里免得下一个人再走一遍：
//
//   - 入口放 `domain` 内、对 `domain` 下导入门禁：`domain` 是 application 与 adapters/http
//     都必须导入的包，禁它等于封掉整个上下文。
//   - 入口放新包：设不了 `ShipmentRequest` 的未导出字段，而重建必须设它们。
//   - `domain/internal/...`：只对 `domain/...` 子树开放，而 `adapters/postgres` 在子树外。
//   - `parcelshipment/internal/...`：对两者都可见，但包外一样设不了未导出字段。
//   - 凭证型参数（`Rehydrate(spec, grant)`，grant 只能在受限包里造）：要让 `domain` 导入
//     `adapters/...`，当场撞第一条门禁。
//
// **为什么这条规则自己解析一遍。**`loadSources` 用的是 `parser.ImportsOnly`，那个模式下
// 拿不到函数调用与类型引用。照它写会得到一条永远绿的门禁——而一条扫不到任何东西的门禁
// 比没有门禁更坏，它看起来像在守。`TestTheRehydrationGateCanActuallyCatchAViolation`
// 就是为此存在的。
//
// **本条比 ADR 要的松一档，而这一档已经量过。**扫描跳过全部 `_test.go`，所以实际覆盖是
// 「持久化适配器 + 任何包的测试」，而 ADR-0028 要的是「持久化适配器**及其**测试」。
//
// 全仓普查过一次：测试文件里引用重建面的只有一处，是 `internal/parcelshipment/domain/`
// 下那份重建入口自己的单元测试——它调这扇门，正是为了断言门会拒绝。原先设想的那种违规
// （别的包在测试里调重建入口造夹具）今天一处也没有。
//
// 那一处判为可用：拦住它等于让门禁禁掉唯一一条证明这扇门管用的测试。于是本条留在只扫生产
// 文件这一侧——「开放扫描 + 为门自己的测试另开豁免」要新开一个假阴性面，去换零个已知违规。
//
// 另有一条盲区连开放扫描也补不上：本条靠导入表取领域包在本文件里的本地名，而领域包**内部**
// 的测试包（`package domain` 而不是 `package domain_test`）根本不导入领域包，引用一律扫不到。
// 全仓今天已经有三个文件是这种写法，所以这不是假想。
func TestOnlyThePersistenceAdapterUsesTheRehydrationEntry(t *testing.T) {
	t.Parallel()

	scanned := 0
	for _, file := range parseRepositorySources(t) {
		if mayUseRehydrationEntry(file.pkg) {
			continue
		}
		scanned++
		for _, referenced := range rehydrationReferencesIn(file.syntax) {
			t.Errorf("%s：引用了重建面 domain.%s；它只对 internal/parcelshipment/adapters/postgres 开放（ADR-0028）",
				file.path, referenced)
		}
	}
	if scanned == 0 {
		t.Fatal("没有扫到任何受本规则约束的文件；门禁会永远空过")
	}
}

// TestRehydrationEntryPermissionDoesNotCatchSiblings 是 mayUseRehydrationEntry 的伴生用例。
//
// 另外三个分类器都配了这种用例，唯独它漏了，而它是本文件里唯一的**豁免**——豁免放宽是假
// 阴性：被误认的包白拿重建面的许可，此后它绕过校验重建聚合，没有任何东西会变红。
//
// 最后一格是它真正要守住的那条：许可只发给 `parcelshipment` 自己的持久化适配器。别的上下文
// 的 `adapters/postgres` 与它形状一模一样，只差第一段，而 ADR-0028 的重建面是按上下文划的。
func TestRehydrationEntryPermissionDoesNotCatchSiblings(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"/internal/parcelshipment/adapters/postgres":        true,
		"/internal/parcelshipment/adapters/postgres/outbox": true,
		"/internal/parcelshipment/adapters/postgresutil":    false,
		"/internal/parcelshipment/adapters/http":            false,
		"/internal/parcelshipment/application":              false,
		"/internal/parcelshipment/domain":                   false,
		"/internal/partycommercial/adapters/postgres":       false,
	}

	for suffix, want := range cases {
		if got := mayUseRehydrationEntry(modulePath + suffix); got != want {
			t.Errorf("%s 判为可用重建入口 %v，应为 %v", suffix, got, want)
		}
	}
}

// TestTheRehydrationGateCanActuallyCatchAViolation 证上一条真的拓得到违规。
//
// 它不依赖仓库里存在一处违规——那种东西正常情况下不该存在，于是上一条永远绿，而「永远绿」
// 既可能因为没人违规，也可能因为扫描根本没生效。这条用合成源码把两者分开。
func TestTheRehydrationGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		source string
		want   []string
	}{
		"直接调用重建入口": {
			source: `package application
import "` + parcelShipmentDomain + `"
func rebuild(spec domain.RehydrateShipmentRequestSpec) { domain.RehydrateShipmentRequest(spec) }`,
			want: []string{"RehydrateShipmentRequestSpec", "RehydrateShipmentRequest"},
		},
		"用别名绕开": {
			source: `package application
import psd "` + parcelShipmentDomain + `"
func rebuild(spec psd.RehydrateShipmentRequestSpec) { _ = spec }`,
			want: []string{"RehydrateShipmentRequestSpec"},
		},
		// 点导入下引用是裸标识符，选择器那条通道一个也扫不到。
		"用点导入绕开": {
			source: `package application
import . "` + parcelShipmentDomain + `"
func rebuild(spec RehydrateShipmentRequestSpec) { RehydrateShipmentRequest(spec) }`,
			want: []string{"RehydrateShipmentRequestSpec", "RehydrateShipmentRequest"},
		},
		// 同一路径导入两次是合法的。只记最后一个本地名会让 domain.* 那一批全部隐形。
		"同路径导入两次": {
			source: `package application
import (
	"` + parcelShipmentDomain + `"
	psd "` + parcelShipmentDomain + `"
)
func rebuild(spec psd.RehydrateShipmentRequestSpec) { domain.RehydrateShipmentRequest(spec) }`,
			want: []string{"RehydrateShipmentRequestSpec", "RehydrateShipmentRequest"},
		},
		// 空导入只跑 init，引用不到任何东西——它不得把点导入那条通道打开。
		"空导入不是点导入": {
			source: `package application
import _ "` + parcelShipmentDomain + `"
func rebuild(spec RehydrateShipmentRequestSpec) { RehydrateShipmentRequest(spec) }`,
			want: nil,
		},
		"只用领域包的其他东西不算违规": {
			source: `package application
import "` + parcelShipmentDomain + `"
func submit(spec domain.SubmitShipmentRequestSpec) { domain.SubmitShipmentRequest(spec) }`,
			want: nil,
		},
		"同名标识符但不来自领域包不算违规": {
			source: `package application
import "example.invalid/other"
func rebuild() { other.RehydrateShipmentRequest() }`,
			want: nil,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			got := rehydrationReferencesIn(syntax)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("referenced = %v, want %v", got, test.want)
			}
		})
	}
}

// TestEveryIdentifierOnTheRehydrationSurfaceIsClassified 让「忘了把新入口补进名单」可检测。
//
// 两份名单都是手工维护的。重建面每长一样东西（`已接受`那条路径要加决定、基线、承诺，
// `已撤回`要加撤回，重建拒绝的理由也在长），都得记得补一行；漏补不会有任何东西变红，而门禁
// 会安静地放行那个新入口——**一条只守住旧成员的门禁，与没有门禁的区别只在它看起来像在守**。
//
// 因此这里要的不是「新入口在受限名单里」，而是**每个导出标识符恰好落在两份名单之一**：都不在
// 说明有人加了东西没分类，都在说明两份名单自相矛盾。反过来也查，名单里有没有已经不存在的
// 条目——名单只多不少时门禁不会变红，而那正是它开始守一批旧名字的样子。
//
// 判据见 belongsToRehydrationSurface，它与声明种类无关：函数、类型、var、const 一视同仁。
func TestEveryIdentifierOnTheRehydrationSurfaceIsClassified(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	anchored := false
	for _, file := range parseRepositorySources(t) {
		if file.pkg != parcelShipmentDomain {
			continue
		}
		if file.path == rehydrationSurfaceFile {
			anchored = true
		}
		for _, decl := range file.syntax.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				candidate := surfaceCandidate{
					path:            file.path,
					name:            typed.Name.Name,
					receiverType:    receiverTypeName(typed),
					yieldsAggregate: yieldsTheAggregateWithoutTakingOne(typed),
				}
				if !typed.Name.IsExported() || !belongsToRehydrationSurface(candidate) {
					continue
				}
				// 方法不许上重建面，登记它也不行。**这一条答的是「名单说不说得出它」，不是
				// 「包外到不到得了它」**——后者由 TestNothingHandsOut… 单独守，两条不可互相
				// 替代，理由写在那条的注释里。
				//
				// rehydrationReferencesIn 只认 `<包本地名>.Name` 形状的选择器，而方法调用写成
				// `spec.Build()`，限定符是变量不是包名，方法表达式的外层 X 也不是标识符——两种
				// 写法它都看不见。于是一个方法形状的入口进了名单，名单会声称覆盖了一个门禁其实
				// 拦不住的东西，正是这条规则要防的那类失效发生在它自己身上。
				if typed.Recv != nil {
					t.Errorf("%s：重建面上出现导出方法 %s；引用扫描看不见方法调用，请改成包级函数",
						file.path, typed.Name.Name)
					continue
				}
				declared[typed.Name.Name] = true
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch typedSpec := spec.(type) {
					case *ast.TypeSpec:
						candidate := surfaceCandidate{path: file.path, name: typedSpec.Name.Name}
						if typedSpec.Name.IsExported() && belongsToRehydrationSurface(candidate) {
							declared[typedSpec.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, name := range typedSpec.Names {
							candidate := surfaceCandidate{path: file.path, name: name.Name}
							if name.IsExported() && belongsToRehydrationSurface(candidate) {
								declared[name.Name] = true
							}
						}
					}
				}
			}
		}
	}

	// 门那个文件被改名或拆开时，并集会静静塌回只剩名字形状那一条，而上面整段仍旧全绿——
	// 那正是本规则要防的那类失效，只不过发生在它自己身上。
	if !anchored {
		t.Fatalf("没扫到 %s；重建面的文件锚点已失效，并集塌回只剩命名约定", rehydrationSurfaceFile)
	}
	if len(declared) == 0 {
		t.Fatal("领域包里没扫到任何重建面；本规则会永远空过")
	}

	for name := range declared {
		_, open := rehydrationSurfaceOpenIdentifiers[name]
		switch {
		case rehydrationEntryIdentifiers[name] && open:
			t.Errorf("domain.%s 两份名单都登记了；门禁会照受限那份拦它，而开放那份写着不该拦——两份必须互斥", name)
		case !rehydrationEntryIdentifiers[name] && !open:
			t.Errorf("domain.%s 属重建面却两份名单都没有；它是新入口就补进 rehydrationEntryIdentifiers，是有意对所有调用方开放就补进 rehydrationSurfaceOpenIdentifiers 并写明为什么", name)
		}
	}
	for name := range rehydrationEntryIdentifiers {
		if !declared[name] {
			t.Errorf("rehydrationEntryIdentifiers 里的 %q 在领域包里已不存在；名单该清理", name)
		}
	}
	for name, reason := range rehydrationSurfaceOpenIdentifiers {
		if !declared[name] {
			t.Errorf("rehydrationSurfaceOpenIdentifiers 里的 %q 在领域包里已不存在；名单该清理", name)
		}
		// 这份名单的全部价值在那句理由上：受限那份靠 rehydrationReferencesIn 承重，开放这份
		// 什么都不强制，它唯一的作用是让下一个人看懂当初为什么放行。理由留空时，「有意开放」
		// 与「懒得想、先塞进来让测试变绿」在名单里长得一模一样——而后者正是上面那条分类断言
		// 要逼出来的东西，绕开它只需要一对空引号。
		if strings.TrimSpace(reason) == "" {
			t.Errorf("rehydrationSurfaceOpenIdentifiers 里的 %q 没写为什么开放；没有理由的登记与漏归类不可区分", name)
		}
	}
}

// TestTheRehydrationSurfaceIsNotHeldUpByNamingAlone 证并集的三条各自都能单独拦住东西。
//
// 三条今天的现实覆盖并不相同，这条测试为哪一格而写要说准，否则下一个人会按一个错的理由留着
// 或删掉它。门那个文件里今天六个导出声明：
//
//   - 四个 `Rehydrate*`：名字那半独自就拦得住。
//   - 两个 `Err*`：名字那半对它们是假（`strings.HasPrefix(name, "Rehydrate")` 不认 `Err` 开头，
//     `ErrRehydrationStateNotSupported` 里更是 “Rehydration” 而非 “Rehydrate”），拦住它们的只有
//     文件那半。所以**删掉文件那半，上一条不会照样绿**：这两个会从 declared 里掉出去，开放
//     名单的反向查当场报两条「在领域包里已不存在」。文件那半有现实的承重点，而那个承重点是
//     开放名单给的——正是它把两个不带约定前缀的哨兵拉进了反向核对。
//   - 接收者那条：今天一个现实实例都没有。
//
// 于是这条测试真正不可替代的是最后一格——没有它，接收者那条子句可以被整条删掉而全仓仍然全绿。
// 前两格留着是因为并集的形状会变：`Err*` 哪天改名带上前缀，文件那半就退回没有现实实例，与今天
// 的接收者那条同状，而那时已经没有别的东西在证明它还活着。
func TestTheRehydrationSurfaceIsNotHeldUpByNamingAlone(t *testing.T) {
	t.Parallel()

	const elsewhere = "internal/parcelshipment/domain/shipment_request.go"

	cases := map[string]struct {
		candidate surfaceCandidate
		want      bool
	}{
		// ADR-0028 亲自否决过 `RestoreShipmentRequest` 这个名字。它照样得被拦住，而拦住它
		// 的只能是文件那半——名字那半对它完全无效。
		"门里的不照约定命名": {candidate: surfaceCandidate{path: rehydrationSurfaceFile, name: "RestoreShipmentRequest"}, want: true},
		"门外的照约定命名":  {candidate: surfaceCandidate{path: elsewhere, name: "RehydrateAcceptedDecision"}, want: true},
		"门里的照约定命名":  {candidate: surfaceCandidate{path: rehydrationSurfaceFile, name: "RehydrateShipmentRequest"}, want: true},
		"门外的判断值对象":  {candidate: surfaceCandidate{path: elsewhere, name: "CommercialBasisSnapshot"}, want: false},
		// 只有接收者那条认得出它：文件不对、方法名也不带约定前缀。
		"门外挂在受限类型上的方法": {
			candidate: surfaceCandidate{path: elsewhere, name: "Build", receiverType: "RehydrateShipmentRequestSpec"},
			want:      true,
		},
		"门外挂在寻常类型上的方法": {
			candidate: surfaceCandidate{path: elsewhere, name: "Build", receiverType: "ShipmentRequest"},
			want:      false,
		},
		// 前三条对它全不中，只有判据四认得出：这正是 N9 那个实测确认的覆盖洞。
		"门外挂在寻常类型上、却凭空交出聚合的方法": {
			candidate: surfaceCandidate{path: elsewhere, name: "Build", receiverType: "SomeSpec", yieldsAggregate: true},
			want:      true,
		},
		"门外凭空交出聚合的包级函数": {
			candidate: surfaceCandidate{path: elsewhere, name: "BuildShipmentRequestAt", yieldsAggregate: true},
			want:      true,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := belongsToRehydrationSurface(test.candidate); got != test.want {
				t.Fatalf("belongsToRehydrationSurface(%+v) = %v, want %v", test.candidate, got, test.want)
			}
		})
	}
}

// TestTheCapabilityCriterionRecognisesEveryShapeThatConjuresAnAggregate 钉住判据四的**识别**
// 那一步。上面那张表钉的是「判据四被并进了 belongsToRehydrationSurface」，而 N9 的洞出在识别：
// 判据认不出那个形状时，后面并不并它都一样。两条缺一不可。
//
// 领域包今天只有两个包级函数凭空交出聚合（`RehydrateShipmentRequest` 与 `SubmitShipmentRequest`，
// 两者都已登记），所以下面全部用合成源码——不用合成取值就无从证明这条判据真在起作用，而这正是
// 本包反复栽过的那一跤。
func TestTheCapabilityCriterionRecognisesEveryShapeThatConjuresAnAggregate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		declaration string
		want        bool
	}{
		// 实测确认的 N9 覆盖洞：接收者不在受限名单，文件与名字两半也不中，前三条全穿。
		"挂在寻常类型上、凭空交出聚合的方法": {
			declaration: "func (spec SomeSpec) Build() ShipmentRequest { return ShipmentRequest{} }",
			want:        true,
		},
		"指针接收者同形": {
			declaration: "func (spec *SomeSpec) Build() ShipmentRequest { return ShipmentRequest{} }",
			want:        true,
		},
		"凭空交出聚合的包级函数": {
			declaration: "func BuildShipmentRequestAt(revision int64) ShipmentRequest { return ShipmentRequest{} }",
			want:        true,
		},
		"交回聚合与错误": {
			declaration: "func SubmitShipmentRequest(spec Spec) (ShipmentRequest, error) { return ShipmentRequest{}, nil }",
			want:        true,
		},
		// 转移不是门：它收了一个进来，由转移扫描守版本不动。认成重建面会把领域包里每个状态
		// 转移都拖上来，而它们一个都不该登记进两份名单。
		"聚合自己的状态转移": {
			declaration: "func (request ShipmentRequest) Decide(spec Spec) (ShipmentRequest, error) { return request, nil }",
			want:        false,
		},
		"收下聚合的包级函数": {
			declaration: "func Advance(request ShipmentRequest) ShipmentRequest { return request }",
			want:        false,
		},
		// 只收不交回：它改不出一个新聚合递给包外。
		"只收下聚合的读取方法": {
			declaration: "func (request ShipmentRequest) State() ShipmentRequestState { return request.state }",
			want:        false,
		},
		"与聚合无关的声明": {
			declaration: "func NewSomeSpec(id string) SomeSpec { return SomeSpec{} }",
			want:        false,
		},
		// 指针与切片都算递出：包了一层不改变调用方能拿到它这件事。
		"交回聚合指针": {
			declaration: "func Conjure() *ShipmentRequest { return nil }",
			want:        true,
		},
		"交回聚合切片": {
			declaration: "func ConjureAll() []ShipmentRequest { return nil }",
			want:        true,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			syntax, err := parser.ParseFile(
				token.NewFileSet(), "synthetic.go", "package domain\n"+test.declaration, 0,
			)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			function, ok := syntax.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatalf("合成源码第一条声明不是函数：%T", syntax.Decls[0])
			}
			if got := yieldsTheAggregateWithoutTakingOne(function); got != test.want {
				t.Fatalf("yieldsTheAggregateWithoutTakingOne(%s) = %v, want %v", test.declaration, got, test.want)
			}
		})
	}
}

// TestNothingHandsOutARestrictedTypeWithoutBeingRestricted 守住真正承重的那条性质。
//
// 名单式覆盖看起来在守「每个入口都登记了」，实际承重的是另一条：**受限入口只能经由一个受限
// 名字抵达**。今天它成立，方法因此是被传递地守住的——包外要调 `spec.Build()`，得先拿到一个
// `RehydrateShipmentRequestSpec` 值，而领域包没有任何导出声明交回它，于是调用方只能自己指名
// 那个类型，而类型名在受限名单里，引用扫描看得见。
//
// **这条与那条方法禁令**（TestEveryIdentifier… 里 `typed.Recv != nil` 那一支）**不重复，两者答
// 的是不同的问题，谁也替不了谁。**那条答「名单说不说得出方法」——说不出，所以方法不许登记；
// 这条答「包外到不到得了方法」——到不了，但靠的是可达性而不是引用扫描。方法禁令挡不住一个
// 交回受限类型的导出函数（那时可达性没了，方法照样叫得动）；这条也挡不住一份声称覆盖了方法
// 的名单。两条同时在，是因为它们各自的失效方式不同。
//
// 哪天出现一条不必指名受限类型就能拿到它的路径，上面整套当场就穿，而在这条测试之前不会有任何
// 东西变红。今天认四种形状，它们的共同点是「给受限类型另开一个不受限的名字」：
//
//   - 导出函数的结果。
//   - **类型别名**。它与定义型只差一个等号，危险度差一整级：`type X Restricted` 是新类型，包外
//     要把它喂进重建入口必须写一次 `domain.Restricted(x)` 转换，而那是个选择器，引用扫描看得见；
//     `type X = Restricted` 根本不是新类型，值直接就能传，全程一次都不必提受限名字。别名再配一个
//     挂在它上面的方法（接收者名不在受限名单里，方法禁令因此不触发），就是一个四道检查全穿的
//     第二重建入口。
//   - 导出的包级 var/const。`var DefaultSnapshot RehydrateShipmentRequestSpec` 与别名同理。
//   - 导出结构体的导出字段，含嵌入字段。
//
// **仍然看不见的**：类型由右值推导且右值不是复合字面量（`var x = build()`）、经 `any` 或接口动态
// 递出、以及经另一个包中转。这三种都要 go/types 与类型信息，本文件这套语法扫描做不到。写在这里
// 免得下一个人以为覆盖是全的。
func TestNothingHandsOutARestrictedTypeWithoutBeingRestricted(t *testing.T) {
	t.Parallel()

	scanned := 0
	for _, file := range parseRepositorySources(t) {
		if file.pkg != parcelShipmentDomain {
			continue
		}
		scanned++
		for _, decl := range file.syntax.Decls {
			for _, handout := range handoutsIn(decl) {
				t.Errorf("%s：%s，而这个名字自己不在受限名单里；调用方从此不必指名任何受限标识符就能拿到它，引用扫描看不见这条路",
					file.path, handout)
			}
		}
	}
	if scanned == 0 {
		t.Fatalf("没扫到 %s 的任何文件；本规则会永远空过", parcelShipmentDomain)
	}
}

// surfaceHandout 是一处「给受限类型另开了一个不受限的名字」。
type surfaceHandout struct {
	kind       string
	name       string
	restricted string
}

func (handout surfaceHandout) String() string {
	return handout.kind + " " + handout.name + " 交出受限类型 " + handout.restricted
}

// handoutsIn 列出一条顶层声明交出了哪些受限类型。写成纯函数而不是就地 t.Errorf，是为了让
// TestTheHandoutScanCanActuallyCatchAViolation 能拿合成声明喂它——四条判据里三条今天在仓里
// 一个现实实例都没有，不用合成取值就无从证明它们真的在起作用。
func handoutsIn(decl ast.Decl) []surfaceHandout {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if !typed.Name.IsExported() || typed.Type.Results == nil ||
			rehydrationEntryIdentifiers[typed.Name.Name] {
			return nil
		}
		return handoutsAt("导出函数", typed.Name.Name, typed.Type.Results)
	case *ast.GenDecl:
		var found []surfaceHandout
		for _, spec := range typed.Specs {
			found = append(found, specHandouts(spec)...)
		}
		return found
	}
	return nil
}

func specHandouts(spec ast.Spec) []surfaceHandout {
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		if !typed.Name.IsExported() || rehydrationEntryIdentifiers[typed.Name.Name] {
			return nil
		}
		if typed.Assign.IsValid() {
			return handoutsAt("类型别名", typed.Name.Name, typed.Type)
		}
		// 定义型（无等号）不在此列：它是新类型，包外要把它喂进重建入口必须写一次
		// `domain.Restricted(x)` 转换，而那是个选择器，rehydrationReferencesIn 看得见。
		structure, isStruct := typed.Type.(*ast.StructType)
		if !isStruct || structure.Fields == nil {
			return nil
		}
		var found []surfaceHandout
		for _, field := range structure.Fields.List {
			if !fieldReachableFromOutside(field) {
				continue
			}
			found = append(found, handoutsAt("导出结构体", typed.Name.Name+" 的导出字段", field.Type)...)
		}
		return found
	case *ast.ValueSpec:
		var found []surfaceHandout
		for _, name := range typed.Names {
			if !name.IsExported() || rehydrationEntryIdentifiers[name.Name] {
				continue
			}
			if typed.Type != nil {
				found = append(found, handoutsAt("包级声明", name.Name, typed.Type)...)
			}
			// 类型由右值推导时只认复合字面量：那里的类型名写在 AST 上。调用式右值不认——
			// `var x = RehydrateShipmentRequest(spec)` 交回的是 ShipmentRequest 而不是受限
			// 类型，把它算成一处递出是假阳性，而假阳性会逼人给规则开例外。
			for _, value := range typed.Values {
				literal, isLiteral := value.(*ast.CompositeLit)
				if !isLiteral || literal.Type == nil {
					continue
				}
				found = append(found, handoutsAt("包级声明", name.Name, literal.Type)...)
			}
		}
		return found
	}
	return nil
}

// fieldReachableFromOutside 判一个结构体字段包外取不取得到。嵌入字段拿类型名当字段名，而
// 受限类型全是导出的，所以嵌入一律算得到。
func fieldReachableFromOutside(field *ast.Field) bool {
	if len(field.Names) == 0 {
		return true
	}
	for _, name := range field.Names {
		if name.IsExported() {
			return true
		}
	}
	return false
}

func handoutsAt(kind, name string, node ast.Node) []surfaceHandout {
	var found []surfaceHandout
	for _, restricted := range restrictedTypesIn(node) {
		found = append(found, surfaceHandout{kind: kind, name: name, restricted: restricted})
	}
	return found
}

// TestTheHandoutScanCanActuallyCatchAViolation 证上一条真的拦得住，且拦的是对的那一格。
//
// 四条判据里只有「导出函数的结果」在仓里有过现实实例，另外三条今天一个都没有——它们全绿既
// 可能因为没人违规，也可能因为判据压根没生效。这条用合成声明把两者分开，顺带把三处刻意不报
// 的地方也钉住：定义型、调用式右值、未导出的名字。断言带上「哪一格报的」，免得某一格的判据
// 坏掉之后由另一格顶上而测试照样绿。
func TestTheHandoutScanCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	const restricted = "RehydrateShipmentRequestSpec"

	cases := map[string]struct {
		source string
		want   []string
	}{
		"类型别名": {
			source: "type ShipmentRequestSnapshot = " + restricted,
			want:   []string{"类型别名 ShipmentRequestSnapshot 交出受限类型 " + restricted},
		},
		"定义型不算：转换时必须指名受限类型": {
			source: "type ShipmentRequestSnapshot " + restricted,
			want:   nil,
		},
		"包级变量带显式类型": {
			source: "var DefaultSnapshot " + restricted,
			want:   []string{"包级声明 DefaultSnapshot 交出受限类型 " + restricted},
		},
		"包级变量由复合字面量推导": {
			source: "var DefaultSnapshot = " + restricted + "{}",
			want:   []string{"包级声明 DefaultSnapshot 交出受限类型 " + restricted},
		},
		"包级变量由调用式推导不算：交回的不是受限类型": {
			source: "var Built = RehydrateShipmentRequest(spec)",
			want:   nil,
		},
		"未导出的包级变量不算": {
			source: "var defaultSnapshot " + restricted,
			want:   nil,
		},
		"导出结构体的导出字段": {
			source: "type Envelope struct { Spec " + restricted + " }",
			want:   []string{"导出结构体 Envelope 的导出字段 交出受限类型 " + restricted},
		},
		"导出结构体的嵌入字段": {
			source: "type Envelope struct { " + restricted + " }",
			want:   []string{"导出结构体 Envelope 的导出字段 交出受限类型 " + restricted},
		},
		"导出结构体的未导出字段不算": {
			source: "type Envelope struct { spec " + restricted + " }",
			want:   nil,
		},
		"未导出结构体不算": {
			source: "type envelope struct { Spec " + restricted + " }",
			want:   nil,
		},
		"导出函数的结果": {
			source: "func BuildSnapshot() " + restricted + " { panic(1) }",
			want:   []string{"导出函数 BuildSnapshot 交出受限类型 " + restricted},
		},
		"受限名字自己不算：它就是那个受限类型": {
			source: "type " + restricted + " struct { Version RehydrateSubmissionVersionSpec }",
			want:   nil,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", "package domain\n"+test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			var got []string
			for _, decl := range syntax.Decls {
				for _, handout := range handoutsIn(decl) {
					got = append(got, handout.String())
				}
			}
			if strings.Join(got, ";") != strings.Join(test.want, ";") {
				t.Fatalf("handouts = %v, want %v", got, test.want)
			}
		})
	}
}

// restrictedTypesIn 列出一段语法里提到的受限类型，按出现顺序去重。指针、切片、映射都认——
// 递出去的是同一个类型，包了一层不改变调用方能拿到它这件事。
func restrictedTypesIn(node ast.Node) []string {
	var found []string
	seen := map[string]bool{}
	ast.Inspect(node, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && rehydrationEntryIdentifiers[identifier.Name] && !seen[identifier.Name] {
			seen[identifier.Name] = true
			found = append(found, identifier.Name)
		}
		return true
	})
	return found
}

// rehydrationReferencesIn 找出一份源码里对重建面的引用。
//
// 先从导入表取领域包在本文件里的**全部**本地名，再只认这些限定名下的选择器。只比对标识符
// 名字会把任何叫同名方法的东西一并算上，那种误报会逼人给规则开例外，而例外正是门禁失效的
// 常见起点。
//
// 收全部本地名而不是最后一个：同一路径导入两次（`import ( "…/domain"; psd "…/domain" )`）
// 是合法的，只留最后一个会让另一个名字下的全部调用隐形。
//
// 点导入单独一条路：那时引用是裸标识符而不是选择器，选择器那条通道一个也扫不到。裸标识符
// 只在本文件真的点导入了领域包时才认——否则任何同名的局部变量都会被算成违规。
func rehydrationReferencesIn(syntax *ast.File) []string {
	locals := map[string]bool{}
	dotImported := false
	for _, spec := range syntax.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil || imported != parcelShipmentDomain {
			continue
		}
		local := "domain"
		if spec.Name != nil {
			local = spec.Name.Name
		}
		switch local {
		case "_":
			// 空导入只跑 init，引用不到任何东西。
		case ".":
			dotImported = true
		default:
			locals[local] = true
		}
	}
	if len(locals) == 0 && !dotImported {
		return nil
	}

	var referenced []string
	ast.Inspect(syntax, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			qualifier, isIdent := typed.X.(*ast.Ident)
			if !isIdent {
				return true
			}
			if locals[qualifier.Name] && rehydrationEntryIdentifiers[typed.Sel.Name] {
				referenced = append(referenced, typed.Sel.Name)
			}
			// 限定符是裸标识符时，这个选择器的两个子节点都已经判过；再往下走只会让
			// 点导入那条通道把 Sel 当成裸引用重数一遍。
			return false
		case *ast.Ident:
			if dotImported && rehydrationEntryIdentifiers[typed.Name] {
				referenced = append(referenced, typed.Name)
			}
		}
		return true
	})
	return referenced
}

type parsedSource struct {
	pkg    string
	path   string
	syntax *ast.File
}

// parseRepositorySources 与 loadSources 遍历同一批文件，但作完整解析而不是 ImportsOnly。
func parseRepositorySources(t *testing.T) []parsedSource {
	t.Helper()

	root := repositoryRoot(t)
	var files []parsedSource

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

		syntax, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		files = append(files, parsedSource{
			pkg:    filepath.ToSlash(filepath.Join(modulePath, relative)),
			path:   filepath.ToSlash(relative) + "/" + filepath.Base(path),
			syntax: syntax,
		})
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
