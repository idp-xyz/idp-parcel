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
	"RehydrateShipmentRequest":       true,
	"RehydrateShipmentRequestSpec":   true,
	"RehydrateSubmissionVersionSpec": true,
	"RehydrateAcceptanceTaskSpec":    true,
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
}

// rehydrationSurfaceFile 是 ADR-0028 那扇「另一扇门」在磁盘上的位置。
const rehydrationSurfaceFile = "internal/parcelshipment/domain/rehydration.go"

// surfaceCandidate 是一个待判定的领域包声明：它在哪个文件、叫什么、以及（是方法时）挂在
// 哪个类型上。
type surfaceCandidate struct {
	path         string
	name         string
	receiverType string
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
func belongsToRehydrationSurface(candidate surfaceCandidate) bool {
	return candidate.path == rehydrationSurfaceFile ||
		strings.HasPrefix(candidate.name, "Rehydrate") ||
		rehydrationEntryIdentifiers[candidate.receiverType]
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
// **为什么不是导入门禁。**ADR-0028 第 24 行写的是「重建入口所在的包，只允许持久化适配器
// 及其测试导入」，但那个手段在 Go 里对这个入口不可表达，人类已裁定改用引用粒度、且不另立
// 记录——第 24 行承重的是约束与强度（只允许持久化适配器及其测试、由机器强制、注释拦不住
// 下一个人），三样一样不少，变的只是扫描粒度。下面是当时排除掉的路，写在这里免得下一个人
// 再走一遍：
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
					path:         file.path,
					name:         typed.Name.Name,
					receiverType: receiverTypeName(typed),
				}
				if !typed.Name.IsExported() || !belongsToRehydrationSurface(candidate) {
					continue
				}
				// 方法不许上重建面，登记它也不行。rehydrationReferencesIn 只认
				// `<包本地名>.Name` 形状的选择器，而方法调用写成 `spec.Build()`，限定符是
				// 变量不是包名，方法表达式的外层 X 也不是标识符——两种写法它都看不见。
				// 于是一个方法形状的入口进了名单，名单会声称覆盖了一个门禁其实拦不住的
				// 东西，正是这条规则要防的那类失效发生在它自己身上。
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
	for name := range rehydrationSurfaceOpenIdentifiers {
		if !declared[name] {
			t.Errorf("rehydrationSurfaceOpenIdentifiers 里的 %q 在领域包里已不存在；名单该清理", name)
		}
	}
}

// TestTheRehydrationSurfaceIsNotHeldUpByNamingAlone 证并集的三条各自都能单独拦住东西。
//
// 今天领域包里那六个既在门那个文件里、名字又都带约定前缀，三条判出来的结果一模一样，因此
// 上一条测试全绿并不能说明文件那半或接收者那条在起作用——把它们删掉，上一条照样绿。这条用
// 合成取值把三条分开验，免得并集里悄悄只剩一条承重。
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

// TestNothingHandsOutARestrictedTypeWithoutBeingRestricted 守住真正承重的那条性质。
//
// 名单式覆盖看起来在守「每个入口都登记了」，实际承重的是另一条：**受限入口只能经由一个受限
// 类型或一个受限包级函数抵达**。今天它成立，方法因此是被传递地守住的——包外要调
// `spec.Build()`，得先拿到一个 `RehydrateShipmentRequestSpec` 值，而领域包没有任何导出函数
// 交回它，于是调用方只能自己指名那个类型，而类型名在受限名单里，引用扫描看得见。
//
// 哪天出现一条不必指名受限类型就能拿到它的路径——一个导出函数返回了 spec——上面整套当场就
// 穿了，而在这条测试之前不会有任何东西变红。把那句话写成断言，是因为本仓对「约束只写在注释
// 里」的既定态度就是它等于没有约束。
//
// 已知缺口：它只看函数结果。导出结构体的导出字段同样能把受限类型递出去，判那个要 go/types
// 与类型信息，本文件这套语法扫描做不到。写在这里免得下一个人以为覆盖是全的。
func TestNothingHandsOutARestrictedTypeWithoutBeingRestricted(t *testing.T) {
	t.Parallel()

	scanned := 0
	for _, file := range parseRepositorySources(t) {
		if file.pkg != parcelShipmentDomain {
			continue
		}
		scanned++
		for _, decl := range file.syntax.Decls {
			function, isFunction := decl.(*ast.FuncDecl)
			if !isFunction || !function.Name.IsExported() || function.Type.Results == nil {
				continue
			}
			if rehydrationEntryIdentifiers[function.Name.Name] {
				continue
			}
			for _, handed := range restrictedTypesIn(function.Type.Results) {
				t.Errorf("%s：导出函数 %s 交回受限类型 %s，而它自己不在受限名单里；调用方从此不必指名任何受限标识符就能拿到它，引用扫描看不见这条路",
					file.path, function.Name.Name, handed)
			}
		}
	}
	if scanned == 0 {
		t.Fatalf("没扫到 %s 的任何文件；本规则会永远空过", parcelShipmentDomain)
	}
}

// restrictedTypesIn 列出一段结果列表里提到的受限类型，按出现顺序去重。指针、切片、映射都
// 认——递出去的是同一个类型，包了一层不改变调用方能拿到它这件事。
func restrictedTypesIn(results *ast.FieldList) []string {
	var found []string
	seen := map[string]bool{}
	ast.Inspect(results, func(node ast.Node) bool {
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
