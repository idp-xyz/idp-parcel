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
// 也不要求方法体里有 switch——映射查表、切片下标都是合法写法，把它们判成违规就会逼人给规则
// 开例外，而例外正是门禁失效的常见起点。要防的失败是「压根没碰 String()」，这个判据对它
// 一样灵，代价只是放过一种现实中不会出现的写法（在别处提一次名字却不真的分派）。
func TestEveryEnumConstantIsNamedByItsStringMethod(t *testing.T) {
	t.Parallel()

	byPackage := map[string][]*ast.File{}
	for _, file := range parseRepositorySources(t) {
		byPackage[file.pkg] = append(byPackage[file.pkg], file.syntax)
	}

	scanned := 0
	for _, pkg := range sortedKeys(byPackage) {
		omissions, count := enumOmissionsIn(byPackage[pkg])
		scanned += count
		for _, omitted := range omissions {
			t.Errorf("%s：%s 声明了却没在它的 String() 里出现过；漏登记的取值交回空串，而空串在下游会被构造器拒绝后静默丢弃、或拌进摘要让两个不同取值归一",
				strings.TrimPrefix(pkg, modulePath+"/"), omitted)
		}
	}
	// 报出判过多少个，好让「收集侧悄悄少认了一批」在 -v 下看得见。下面那道守卫只挡得住
	// 完全空过，挡不住「本该五十个却只扫到三个」——那种收缩不会有任何东西变红。
	t.Logf("判过 %d 个带 String() 的整型枚举", scanned)
	if scanned == 0 {
		t.Fatal("没扫到任何带 String() 的整型枚举；本条门禁会永远空过")
	}
}

// enumOmissionsIn 找出一个包里「声明了却没在自己 String() 里露过面」的枚举常量，并回报本次
// 判过多少个枚举——扫零与全绿在输出上一模一样，调用方需要能把两者分开。
//
// 按包收而不是按文件收：类型是包作用域的，声明类型的文件与声明常量的文件可以不是同一个，
// 按文件判会把这种拆法误报成「有常量没类型」。
func enumOmissionsIn(files []*ast.File) (omissions []string, scanned int) {
	enums := map[string]bool{}
	constants := map[string][]string{}
	stringMethods := map[string]*ast.FuncDecl{}
	packageValues := map[string][]ast.Expr{}

	for _, file := range files {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				collectEnumDeclarations(typed, enums, constants)
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

	for name := range enums {
		declared := constants[name]
		method, hasString := stringMethods[name]
		if !hasString || len(declared) == 0 {
			continue
		}
		scanned++

		mentioned := mentionedBy(method, packageValues)
		for index, constant := range declared {
			// 第一个常量是零值哨兵（`XxxInvalid`、`PendingReasonNone`），本仓一律有意让它
			// 落进 default 交回空串——那正是「这个值不该出现」的表达，给它一个名字反而会让
			// 一个未初始化的枚举看起来像个正经取值。
			if index == 0 {
				continue
			}
			// 未导出的常量不属公开值域，是上界哨兵之类的内部记号（`judgmentPendingReasonEnd`）。
			// 它们本就不该有字符串表示。
			if !ast.IsExported(constant) {
				continue
			}
			if !mentioned[constant] {
				omissions = append(omissions, name+"."+constant)
			}
		}
	}
	sort.Strings(omissions)
	return omissions, scanned
}

// collectEnumDeclarations 从一个声明块里取出整型具名类型与挂在它们名下的常量。
func collectEnumDeclarations(decl *ast.GenDecl, enums map[string]bool, constants map[string][]string) {
	switch decl.Tok {
	case token.TYPE:
		for _, spec := range decl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Assign.IsValid() {
				continue
			}
			if base, ok := typeSpec.Type.(*ast.Ident); ok && integerBaseTypes[base.Name] {
				enums[typeSpec.Name.Name] = true
			}
		}
	case token.CONST:
		// current 跨行续用，因为 iota 块里只有头一行写类型。但续用只在整行都省略时成立：
		// 一旦某行自带值而不带类型，它就是一个新的无类型常量，后面的行也不再续用上一个
		// 类型——照抄「记住最后一次见过的类型」会把它们全算进上一个枚举。
		current := ""
		for _, spec := range decl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			switch {
			case valueSpec.Type != nil:
				current = ""
				if named, ok := valueSpec.Type.(*ast.Ident); ok {
					current = named.Name
				}
			case len(valueSpec.Values) > 0:
				current = ""
			}
			if current == "" {
				continue
			}
			for _, name := range valueSpec.Names {
				constants[current] = append(constants[current], name.Name)
			}
		}
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
	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1；跨文件的枚举没被判到，本规则会对这种拆法整包免检", scanned)
	}
	if strings.Join(got, ",") != "Outcome.Rejected" {
		t.Fatalf("omissions = %v, want [Outcome.Rejected]", got)
	}
}
