package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// integerBaseTypes 是本仓 iota 枚举的底层类型。
//
// 字符串底型的枚举（`type LengthUnit string`，parcelpricing 里一批）刻意不在本条约束内：
// 它们的 String() 写成 `return string(x)`，没有分派，也就没有「漏一个分支」这件事；它们的
// 风险模型是构造器加 valid() 那道入口闸，与这里要防的东西不是一回事。硬把两种形状塞进
// 一条规则，判据会松到两边都拦不住。
var integerBaseTypes = map[string]bool{
	"uint8": true, "uint16": true, "uint32": true, "uint64": true, "uint": true,
	"int8": true, "int16": true, "int32": true, "int64": true, "int": true,
	"byte": true, "rune": true,
}

// TestEveryEnumConstantIsNamedByItsStringMethod 守住「新增取值必须同时补 String()」。
//
// **为什么这条值得单独立一道门禁。**本仓的 iota 枚举一律配 `default: return ""`，而那个空串
// 很多时候是承载的，不是给人看的：
//
//   - `recordAttempt` 拿 `reason.String()` 去造 `ProcessingAttemptReason`，空串被
//     `ErrBlankValue` 拒掉之后**裸 return**，这一轮的处理尝试悄悄不落库。五条用例主路径都
//     经过它。
//   - `judgmentContinuation` 把它拌进 sha256，空串让所有漏登记的原因共用同一条续办引用，
//     正好打掉该函数注释里承诺的「停在不同阶段的两次未决给出不同引用」。
//   - `CommercialRegistry.ViewRevision` 把 `CommercialObjectKind` 与 `CommercialVersionStatus`
//     两个枚举拌进摘要，而那个摘要的职责是证明商业视图有没有变过。漏登记时两个本该不同的
//     登记项归一，失效方向是**该变而没变**——一次本该把先前解析判成`已失效`的登记变动静默
//     不触发。它偏偏又是靠派生而非手工递增来避开「总会有人忘记」的。
//
// 三处的共同点是漏补不会有任何东西变红。编译器不管这件事：switch 少一个分支照样编过。
//
// **不要照 `FutureSubmissionDisposition` 那个先例写。**本仓此前唯一一个值域扫描型伴生测试
// 用的是「扫连续区间、跳过空串、比对名字集合」，而那个 `if name != ""` 恰好把漏补的取值筛
// 掉了：补了 String() 才会变红，漏补反而全绿。它守的是相反的方向。
//
// **判据刻意宽松：只要常量在 String() 够得着的地方被提到过就算数。**不要求它出现在 case 上，
// 也不要求方法体里有 switch——`map[Outcome]string{Accepted: …}` 这种按常量名索引的查表是完整
// 实现，把它判成违规就会逼人给规则开例外，而例外正是门禁失效的常见起点。要防的失败是「压根
// 没碰 String()」，这个判据对它一样灵。
//
// **但按下标索引的位置表会被报，而且那是有意的，不是判据够不着。** `var names = [...]string{
// "", "DISTINCT", …}` 里一个常量名都不出现，本条对它无从判断新增取值登没登记——它靠位置登记，
// 而位置没有任何语法检查验得了。它自己还另有一条静默失败：字面量少一项或错一位，全部名字一起
// 平移，而每个取值看起来仍然「有名字」。所以本条对位置表的答复是**别写**；真要改成位置表，
// 该改的是那份实现而不是这条判据。这一条由 `TestTheEnumGateCanActuallyCatchAViolation` 钉住，
// 免得它退回一句没人验的声称——它此前就是那样漂了一次：头注释声称容忍切片下标，而实现从来
// 容忍不了。
func TestEveryEnumConstantIsNamedByItsStringMethod(t *testing.T) {
	t.Parallel()

	byPackage := map[string][]*ast.File{}
	for _, file := range parseRepositorySources(t) {
		byPackage[file.pkg] = append(byPackage[file.pkg], file.syntax)
	}

	scanned := 0
	seen := map[string]bool{}
	for _, pkg := range sortedKeys(byPackage) {
		omissions, types := enumOmissionsIn(byPackage[pkg])
		scanned += len(types)
		for _, name := range types {
			// 键带包限定。裸类型名在本仓会撞车——`JudgmentAsOfOutcome`、`ReachabilityValue`、
			// `NotFormedReason` 各有两个包声明同名类型。按裸名锚只断言得了「某个包里有个叫
			// 这名字的被扫到」，而承载那条静默链的可能恰好是没被扫到的那一个。
			seen[strings.TrimPrefix(pkg, modulePath+"/")+"."+name] = true
		}
		for _, omitted := range omissions {
			t.Errorf("%s：%s 声明了却没在它的 String() 里出现过；漏登记的取值交回空串，而空串在下游会被构造器拒绝后静默丢弃、或拌进摘要让两个不同取值归一",
				strings.TrimPrefix(pkg, modulePath+"/"), omitted)
		}
	}
	// 报出判过多少个，好让「收集侧悄悄少认了一批」在 -v 下看得见。
	t.Logf("判过 %d 个带 String() 的整型枚举", scanned)
	if scanned == 0 {
		t.Fatal("没扫到任何带 String() 的整型枚举；本条门禁会永远空过")
	}
	// 空过守卫只挡得住「一个都没扫到」，挡不住「本该五十个却只扫到三个」——收集侧悄悄少认
	// 一批不会有任何东西变红。所以把本文件开头点名的三个承重枚举钉到检测侧：它们各自承载一条
	// 真实的静默链（处理尝试不落库、续办引用撞车、商业视图摘要该变而没变），少认了哪一个，
	// 被它护住的那条链就重新敞开，而门禁照样全绿。同一手法见 rehydration_gate_test.go 的
	// `anchored` 标志与那里的磁盘锚。
	for _, anchor := range []string{
		"internal/parcelshipment/application.JudgmentPendingReason",
		"internal/partycommercial/domain.CommercialObjectKind",
		"internal/partycommercial/domain.CommercialVersionStatus",
	} {
		if !seen[anchor] {
			t.Errorf("%s 不在本次扫描面内；本文件开头点名它承载一条静默链，扫不到它等于那条链没人守，而本条仍会全绿", anchor)
		}
	}
}

// enumOmissionsIn 找出一个包里「声明了却没在自己 String() 里露过面」的枚举常量，并回报本次
// 判过**哪些**枚举——扫零与全绿在输出上一模一样，调用方需要能把两者分开。
//
// 交回名字而不只是个数：个数只答得了「有没有完全空过」，答不了「本该判到的那几个还在不在
// 里面」。调用方拿名字去钉承重枚举，那才挡得住收集侧悄悄收缩。
//
// 按包收而不是按文件收：类型是包作用域的，声明类型的文件与声明常量的文件可以不是同一个，
// 按文件判会把这种拆法误报成「有常量没类型」。
func enumOmissionsIn(files []*ast.File) (omissions []string, scannedTypes []string) {
	namedTypes := map[string]string{}
	constants := map[string][]enumConstant{}
	stringMethods := map[string]*ast.FuncDecl{}
	packageValues := map[string][]ast.Expr{}

	for _, file := range files {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				collectEnumDeclarations(typed, namedTypes, constants)
				collectPackageValues(typed, packageValues)
			case *ast.FuncDecl:
				if typed.Name.Name == "String" && returnsOneString(typed) {
					if receiver := receiverTypeName(typed); receiver != "" {
						stringMethods[receiver] = typed
					}
				}
			}
		}
	}

	for name := range namedTypes {
		if !restsOnAnIntegerBase(name, namedTypes) {
			continue
		}
		declared := constants[name]
		method, hasString := stringMethods[name]
		if !hasString || len(declared) == 0 {
			continue
		}
		scannedTypes = append(scannedTypes, name)

		mentioned := mentionedBy(method, packageValues)
		for _, constant := range declared {
			// 零值哨兵（`XxxInvalid`、`PendingReasonNone`）本仓一律有意让它落进 default 交回
			// 空串——那正是「这个值不该出现」的表达，给它一个名字反而会让一个未初始化的枚举
			// 看起来像个正经取值。
			//
			// **按值认，不按位置认。** `iota + 1` 那种写法下零值根本没有常量，第一个常量是
			// 真取值；按位置跳会让它永远免检，而漏补与补齐在那种形状下**一样是全绿**，唯一
			// 差别只是位置。`SourceClassification` 正是这个形状。
			if constant.zeroValued {
				continue
			}
			// 未导出的常量不属公开值域，是上界哨兵之类的内部记号（`judgmentPendingReasonEnd`）。
			// 它们本就不该有字符串表示。
			if !ast.IsExported(constant.name) {
				continue
			}
			if !mentioned[constant.name] {
				omissions = append(omissions, name+"."+constant.name)
			}
		}
	}
	sort.Strings(omissions)
	sort.Strings(scannedTypes)
	return omissions, scannedTypes
}

// enumConstant 是一个枚举常量，外加它是不是那个零值哨兵。
//
// 「是不是哨兵」得随常量一起记下来，不能等到判定时按下标推：常量可以拆在多个 const 块甚至
// 多个文件里，届时「第几个」取决于遍历顺序，而遍历顺序不是任何人打算表达的东西。
type enumConstant struct {
	name       string
	zeroValued bool
}

// restsOnAnIntegerBase 顺着具名类型链走到底，判它最终是不是落在整型上。
//
// 走链而不是只看一层，堵的是「给枚举另起一个名字」那条绕法：`type Code Outcome` 的底层类型是
// 一个具名类型而不是 `uint8`，只看一层它压根进不了扫描面，于是**整包免检且扫描数不涨**。这与
// c762b70 在重建面上堵掉的是同一个形状。
//
// 带访问集合防环。`type A B; type B A` 编不过，但本扫描是纯语法的，编不过的源码一样解析得到，
// 一个环就够让门禁挂死——而挂死的门禁与没有门禁的区别只在它看起来像在守。
//
// 全仓今天链长 ≥ 2 的声明是 0 个（实测于 c53cf4e），本函数的循环永远第一轮返回，`seen` 从未
// 用到第二次。与上面那条同理：守的是将来，而合成用例钉着它。
func restsOnAnIntegerBase(name string, namedTypes map[string]string) bool {
	seen := map[string]bool{}
	for {
		base, declared := namedTypes[name]
		if !declared || seen[name] {
			return false
		}
		seen[name] = true
		if integerBaseTypes[base] {
			return true
		}
		name = base
	}
}

// collectEnumDeclarations 从一个声明块里取出具名类型的底层类型名，与挂在它们名下的常量。
//
// 底层类型**不在这里筛整型**：`type Code Outcome` 的底层是一个具名类型，当场筛掉它就等于
// 让改名绕过整道门禁。是不是整型交给 restsOnAnIntegerBase 顺链去判。
func collectEnumDeclarations(decl *ast.GenDecl, namedTypes map[string]string, constants map[string][]enumConstant) {
	switch decl.Tok {
	case token.TYPE:
		for _, spec := range decl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Assign.IsValid() {
				continue
			}
			if base, ok := typeSpec.Type.(*ast.Ident); ok {
				namedTypes[typeSpec.Name.Name] = base.Name
			}
		}
	case token.CONST:
		// current 跨行续用，因为 iota 块里只有头一行写类型。但续用只在整行都省略时成立：
		// 一旦某行自带值而不带类型，它就是一个新的无类型常量，后面的行也不再续用上一个
		// 类型——照抄「记住最后一次见过的类型」会把它们全算进上一个枚举。
		current := ""
		for specIndex, spec := range decl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			zeroValued := false
			switch {
			case valueSpec.Type != nil:
				current = ""
				if named, ok := valueSpec.Type.(*ast.Ident); ok {
					current = named.Name
					zeroValued = declaresTheZeroValue(specIndex, valueSpec.Values)
				}
			case len(valueSpec.Values) > 0:
				current = ""
			}
			if current == "" {
				continue
			}
			for index, name := range valueSpec.Names {
				// 只有带类型那一行的**头一个**名字有资格当零值哨兵。同一行写多个名字时
				// 它们共享同一个值，后面几个不是零值的起点。
				constants[current] = append(constants[current], enumConstant{
					name:       name.Name,
					zeroValued: zeroValued && index == 0,
				})
			}
		}
	}
}

// declaresTheZeroValue 认「这一行的值就是零」：裸 `iota`，或字面量 0。
//
// `iota + 1` 是 BinaryExpr，这里认不出来——那正是本判据要与之区分的形状，也是它存在的全部
// 理由。省掉这个函数改回「第一个常量就是哨兵」，`SourceClassification` 那种写法会整类免检。
//
// **全仓今天恰好一个类型命中它，而它此刻不在扫描面内**（实测于 c53cf4e）：`SourceClassification`
// 是唯一的 `iota + 1`，三个常量在本判据下都不算哨兵、一个都不跳——但它没有 `String()`，卡在
// `hasString` 那道口子外面。所以本函数今天守的是将来：有人给它补上 `String()` 的那一刻，
// 三个取值同时进入强制范围，而旧判据会让第一个静默免检。**「没有实例」不等于「没有守住」，
// 它等于「暂时没东西可守」**，两者的区别在下一个人打算删掉本函数时才显出来。
func declaresTheZeroValue(specIndex int, values []ast.Expr) bool {
	// 只有块内**第一个** spec 才可能声明零值：`iota` 的值由位置决定，第二行上的裸 `iota`
	// 已经是 1 了。光看值表达式判不出这件事——`OutcomeInvalid T = iota` 与紧随其后的
	// `Accepted T = iota` 长得一模一样，而后者的真值是 1，把它当哨兵跳掉就是一个静默免检。
	if specIndex != 0 || len(values) == 0 {
		return false
	}
	// 只看第一个值：`A, B T = iota, iota + 1` 一行两名两值，零值是 A，不是「没有零值」。
	// 要求 `len(values) == 1` 会把这种块整块判成无哨兵，于是真正的零值也被要求有名字。
	switch typed := values[0].(type) {
	case *ast.Ident:
		return typed.Name == "iota"
	case *ast.BasicLit:
		return typed.Kind == token.INT && typed.Value == "0"
	default:
		return false
	}
}

// returnsOneString 认 `String() string` 这个形状。带第二个返回值的不是 fmt.Stringer，
// 它的空串没有本规则要防的那条下游。
func returnsOneString(function *ast.FuncDecl) bool {
	results := function.Type.Results
	if results == nil || len(results.List) != 1 || len(results.List[0].Names) > 1 {
		return false
	}
	named, ok := results.List[0].Type.(*ast.Ident)
	return ok && named.Name == "string"
}

// collectPackageValues 记下包级 var/const 的初始化表达式，供 mentionedBy 走那一跳。
func collectPackageValues(decl *ast.GenDecl, values map[string][]ast.Expr) {
	if decl.Tok != token.VAR && decl.Tok != token.CONST {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range valueSpec.Names {
			values[name.Name] = append(values[name.Name], valueSpec.Values...)
		}
	}
}

// mentionedBy 收 String() 方法体里提到的标识符，再顺着它引用的包级 var/const 走一跳。
//
// 走这一跳是为了认表驱动写法：`var names = map[Outcome]string{…}` 配
// `func (o Outcome) String() string { return names[o] }` 是一份完整实现，判它违规就是假阳性，
// 而本文件反复吃过的教训是假阳性会逼人给规则开例外，例外正是门禁失效的常见起点。
//
// **只走一跳，不递归。**跟到底最终会把整个包的标识符都拉进来，届时本规则对任何漏补都不再
// 变红——一条永远绿的门禁比没有门禁更坏。一跳够用是因为查表写法只有一层间接；真出现两层
// 的写法，本条会如实报，那时该改的是那份实现而不是这条判据。
func mentionedBy(method *ast.FuncDecl, packageValues map[string][]ast.Expr) map[string]bool {
	direct := identifiersIn(method)
	found := make(map[string]bool, len(direct))
	for name := range direct {
		found[name] = true
	}
	// 先把直接提到的名字快照下来再走那一跳：边遍历边往同一个 map 里插入，新键访不访问得到
	// 由运行时决定，本条会时绿时红。
	for name := range direct {
		for _, value := range packageValues[name] {
			for inner := range identifiersIn(value) {
				found[inner] = true
			}
		}
	}
	return found
}

func identifiersIn(node ast.Node) map[string]bool {
	found := map[string]bool{}
	ast.Inspect(node, func(inner ast.Node) bool {
		if identifier, ok := inner.(*ast.Ident); ok {
			found[identifier.Name] = true
		}
		return true
	})
	return found
}

func sortedKeys(byPackage map[string][]*ast.File) []string {
	keys := make([]string, 0, len(byPackage))
	for key := range byPackage {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestTheEnumGateCanActuallyCatchAViolation 证上一条真的拦得到东西，并把每一处碳出各验一遍。
//
// 上一条正常情况下永远绿，而「永远绿」既可能因为没人漏补，也可能因为判据根本没生效。碳出
// 尤其需要单独验：一个把所有常量都放过的碳出会让整条规则看起来在守，而它一个也拦不住。
func TestTheEnumGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		source string
		want   []string
	}{
		"漏补一个取值": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "ACCEPTED"
	default:
		return ""
	}
}`,
			want: []string{"Outcome.Rejected"},
		},
		"补齐了就不报": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "ACCEPTED"
	case Rejected:
		return "REJECTED"
	default:
		return ""
	}
}`,
			want: nil,
		},
		// 零值哨兵不该被要求有名字，否则每个枚举都会立刻报一条假阳性，而假阳性会逼人
		// 给规则开例外。
		"零值哨兵没有 case 不算漏": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
)
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "ACCEPTED"
	default:
		return ""
	}
}`,
			want: nil,
		},
		// `iota + 1` 下零值没有常量，第一个常量是真取值。按位置认哨兵会让它永远免检，而漏补
		// 与补齐在这种形状下**一样全绿**，唯一差别只是位置。`SourceClassification` 就长这样。
		"iota 加一时首个常量不是哨兵": {
			source: `package sample
type Outcome uint8
const (
	Distinct Outcome = iota + 1
	Replay
)
func (o Outcome) String() string {
	switch o {
	case Replay:
		return "REPLAY"
	default:
		return ""
	}
}`,
			want: []string{"Outcome.Distinct"},
		},
		// 没有零值常量的类型一个都不跳。这一条与上一条分开：上一条证「哨兵认错了」，这一条钉
		// 「此时不该有任何豁免」。
		"没有零值常量时一个都不跳": {
			source: `package sample
type Outcome uint8
const (
	Accepted Outcome = iota + 1
	Rejected
)
func (o Outcome) String() string { return "" }`,
			want: []string{"Outcome.Accepted", "Outcome.Rejected"},
		},
		// 判的是值不是写法：显式赋 0 与裸 iota 同为哨兵，而它后面那个取值照判。
		"显式赋零者是哨兵，其后照判": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = 0
	Accepted       Outcome = 1
)
func (o Outcome) String() string { return "" }`,
			want: []string{"Outcome.Accepted"},
		},
		// 定义型改名：底层是具名类型而不是 uint8。只看一层的话它压根进不了扫描面，整包免检
		// 且扫描数不涨——与 c762b70 在重建面上堵掉的是同一个形状。
		"给枚举另起一个名字仍在扫描面内": {
			source: `package sample
type Outcome uint8
type Code Outcome
const (
	CodeInvalid Code = iota
	CodeAccepted
	CodeRejected
)
func (c Code) String() string {
	switch c {
	case CodeAccepted:
		return "ACCEPTED"
	default:
		return ""
	}
}`,
			want: []string{"Code.CodeRejected"},
		},
		// 块内第二行重写类型再写裸 iota：它的真值是 1，不是零值哨兵。只看值表达式的判据会
		// 把它跳掉，于是一个真取值永远免检。
		"块内第二行的裸 iota 不是哨兵": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted       Outcome = iota
	Rejected
)
func (o Outcome) String() string { return "" }`,
			want: []string{"Outcome.Accepted", "Outcome.Rejected"},
		},
		// 一行两名两值：零值是第一个名字，不是「这块没有零值」。要求恰好一个值会把真正的
		// 零值哨兵也报成遗漏——方向与上一格相反的假阳性。
		"一行多名时零值仍是第一个": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid, Accepted Outcome = iota, iota + 1
	Rejected                 Outcome = iota + 2
)
func (o Outcome) String() string { return "" }`,
			want: []string{"Outcome.Accepted", "Outcome.Rejected"},
		},
		// 第二个 const 块重新从 iota 起，它的第一个常量真值就是 0，跳它是对的。这一格与
		// 上面两格一起把「块内第一个」这条判据的三面都钉住。
		"新块首行的裸 iota 仍是哨兵": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
)
const (
	Rejected Outcome = iota
	Withdrawn
)
func (o Outcome) String() string { return "" }`,
			want: []string{"Outcome.Accepted", "Outcome.Withdrawn"},
		},
		// 位置表按下标登记，本条无从判断新增取值登没登记，因此**有意**报它。头注释此前声称
		// 容忍这种写法，而实现从来容忍不了；这一格钉住的是改正后的那个声称。
		"按下标索引的位置表照报": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)
var names = [...]string{"", "ACCEPTED", "REJECTED"}
func (o Outcome) String() string { return names[o] }`,
			want: []string{"Outcome.Accepted", "Outcome.Rejected"},
		},
		"未导出的上界哨兵不算漏": {
			source: `package sample
type Reason uint8
const (
	ReasonNone Reason = iota
	Stalled
	reasonEnd
)
func (r Reason) String() string {
	switch r {
	case Stalled:
		return "STALLED"
	default:
		return ""
	}
}`,
			want: nil,
		},
		// 判据要的是「被提到过」，不是「出现在 case 上」。方法体内查表是合法写法。
		"方法体内用映射也算提到": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)
func (o Outcome) String() string {
	names := map[Outcome]string{Accepted: "ACCEPTED", Rejected: "REJECTED"}
	return names[o]
}`,
			want: nil,
		},
		// 表驱动的常见写法：表在包级，方法只查它。这是完整实现，报它就是假阳性，
		// 所以判据要顺着方法引用的包级名字走一跳。
		"包级映射走一跳也算提到": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)
var names = map[Outcome]string{Accepted: "ACCEPTED", Rejected: "REJECTED"}
func (o Outcome) String() string { return names[o] }`,
			want: nil,
		},
		// 那一跳到此为止。跟到底会把整个包的标识符都拉进来，届时本规则对任何漏补都不再
		// 变红；这条把那个界钉住，改成递归的当场变红。
		"两跳之外不再跟随": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
)
var inner = map[Outcome]string{Accepted: "ACCEPTED"}
var outer = inner
func (o Outcome) String() string { return outer[o] }`,
			want: []string{"Outcome.Accepted"},
		},
		"字符串底型不在本条约束内": {
			source: `package sample
type Unit string
const (
	Centimetre Unit = "CM"
	Metre      Unit = "M"
)
func (u Unit) String() string { return string(u) }`,
			want: nil,
		},
		"没有 String() 的枚举不报": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
)`,
			want: nil,
		},
		// 同一个 const 块里出现一个自带值的无类型常量之后，后续行不再续用上一个类型。
		// 照抄「记住最后见过的类型」会把它们全算进上一个枚举，报出一串假阳性。
		"块内断开类型续用后不误算": {
			source: `package sample
type Outcome uint8
const (
	OutcomeInvalid Outcome = iota
	Accepted
)
const (
	Accepted2 Outcome = iota + 10
	scanLimit = 32
	other     = 64
)
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "ACCEPTED"
	case Accepted2:
		return "ACCEPTED_2"
	default:
		return ""
	}
}`,
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
			got, _ := enumOmissionsIn([]*ast.File{syntax})
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("omissions = %v, want %v", got, test.want)
			}
		})
	}
}

// TestTheEnumGateSpansFilesWithinAPackage 守住按包收这条。
//
// 类型是包作用域的，声明类型的文件与声明常量的文件可以不是同一个（本仓 acceptance_basis.go
// 与 acceptance_decision.go 就各自放着一批）。判据一旦退回按文件收，这种拆法会两头落空：
// 有类型没常量的那半跳过，有常量没类型的那半根本进不了枚举名单，于是整包静默免检。
func TestTheEnumGateSpansFilesWithinAPackage(t *testing.T) {
	t.Parallel()

	declaration := `package sample
type Outcome uint8
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "ACCEPTED"
	default:
		return ""
	}
}`
	values := `package sample
const (
	OutcomeInvalid Outcome = iota
	Accepted
	Rejected
)`

	fileSet := token.NewFileSet()
	var files []*ast.File
	for name, source := range map[string]string{"a.go": declaration, "b.go": values} {
		syntax, err := parser.ParseFile(fileSet, name, source, 0)
		if err != nil {
			t.Fatalf("解析 %s：%v", name, err)
		}
		files = append(files, syntax)
	}

	got, scanned := enumOmissionsIn(files)
	if len(scanned) != 1 {
		t.Fatalf("scanned = %v, want 恰好一个；跨文件的枚举没被判到，本规则会对这种拆法整包免检", scanned)
	}
	if strings.Join(got, ",") != "Outcome.Rejected" {
		t.Fatalf("omissions = %v, want [Outcome.Rejected]", got)
	}
}
