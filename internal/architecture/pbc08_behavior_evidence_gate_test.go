package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// PBC-08 行为面门禁：每个持久化写方法必须有同包负向证据。
//
// 「写方法」= internal/ 下非测试源里，方法体引用 `RequireExecutor` 或调用
// `EnqueueOnce` 的函数/方法——它们是把业务状态写进库的那一批口。简报明文该性质
// 「随适配器逐个成立，没有静态门禁能替它把关」，指的是**正向**行为（原子提交、
// 回滚双消）确实只能逐个真库测；但「无事务被拒」这半有一个可静态核对的替身：
// 同包测试里存在「引用 `ErrTransactionRequired` 且以该类型接收者调用该方法」的
// 测试函数。本门禁守的就是这半——负向证据在场，写方法才不会在无事务上下文里
// 静默走自动提交。
//
// 归属按**接收者变量的类型**解析，不按方法名唯一性猜（bento-gate-reeval 票 01 实证
// 过：捆绑夹具 `new*Stores` + 包内非唯一 `Save` 会让按名匹配产出成批假 MISSING）。
// 变量类型来自三条语法级路径，全部取自同包测试文件：
//  1. 直接构造：`x, err := adapter.NewIntakeAdoptions(db)` → x 是 IntakeAdoptions
//     （仓内构造器约定 New<T> 交回 <T>，identity_prefix 等门禁同赖此约定）；
//  2. 元组夹具：`a, b, c, _, _ := newJudgmentStores(t)` → 按该夹具**签名结果类型**
//     位置映射（夹具签名显式写明 `(*adapter.X, *adapter.Y, ...)`，比追踪函数体内部
//     绑定更稳）；
//  3. 结构夹具：`s := newStores(t)` 后 `s.field.Save(...)` → 按同包测试文件里该结构
//     类型的字段声明解析 field 的类型。
//
// 守不住的那一格，如实写在这里：证据判定只认**同一个测试函数内**同时出现
// `ErrTransactionRequired` 引用与解析成功的方法调用；把断言埋进辅助函数、或用
// 本门禁不认识的方式构造接收者（如把仓储再包一层自制容器）会被判 MISSING——
// 那时把测试改写成本包已通行的三种形状之一，比教门禁认第四种形状便宜。

// pbc08WriteMethod 是一个待举证的写口。typeName 为空表示包级函数（如
// outboxintent.EnqueueOnce 自身）。
type pbc08WriteMethod struct {
	pkg      string
	typeName string
	method   string
}

// collectPBC08WriteMethods 从非测试源里收写方法。只收 internal/ 下的包——cmd/ 是
// 装配面、tests/ 是合同取证面，写口全都住在 internal/ 的适配器里。
//
// 未导出的写方法（如 TF 的 ResultVersions.next）记账到同类型的**导出调用方**上：
// 包外测试构不成对未导出方法的直接调用，而守卫本来就是经导出面走到的——证据落在
// 真实调用路径上才是证据。没有任何导出调用方的未导出写方法按自身记账（那是死代码
// 或漏了出口，红出来正好）。
func collectPBC08WriteMethods(sources []parsedSource) []pbc08WriteMethod {
	type methodDecl struct {
		pkg      string
		typeName string
		fn       *ast.FuncDecl
	}
	var declared []methodDecl
	for _, source := range sources {
		if !strings.HasPrefix(source.pkg, modulePath+"/internal/") {
			continue
		}
		for _, decl := range source.syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			declared = append(declared, methodDecl{
				pkg:      source.pkg,
				typeName: pbc08ReceiverTypeName(fn),
				fn:       fn,
			})
		}
	}

	seen := map[pbc08WriteMethod]bool{}
	var methods []pbc08WriteMethod
	record := func(method pbc08WriteMethod) {
		if !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	for _, decl := range declared {
		if !pbc08BodyWrites(decl.fn.Body) {
			continue
		}
		name := decl.fn.Name.Name
		if ast.IsExported(name) || decl.typeName == "" {
			record(pbc08WriteMethod{pkg: decl.pkg, typeName: decl.typeName, method: name})
			continue
		}
		// 未导出方法：找同包同类型、体内调用它的导出方法。
		var callers []string
		for _, candidate := range declared {
			if candidate.pkg != decl.pkg || candidate.typeName != decl.typeName {
				continue
			}
			if !ast.IsExported(candidate.fn.Name.Name) {
				continue
			}
			if pbc08BodyCallsMethod(candidate.fn.Body, name) {
				callers = append(callers, candidate.fn.Name.Name)
			}
		}
		if len(callers) == 0 {
			record(pbc08WriteMethod{pkg: decl.pkg, typeName: decl.typeName, method: name})
			continue
		}
		for _, caller := range callers {
			record(pbc08WriteMethod{pkg: decl.pkg, typeName: decl.typeName, method: caller})
		}
	}
	sort.Slice(methods, func(i, j int) bool {
		a, b := methods[i], methods[j]
		if a.pkg != b.pkg {
			return a.pkg < b.pkg
		}
		if a.typeName != b.typeName {
			return a.typeName < b.typeName
		}
		return a.method < b.method
	})
	return methods
}

func pbc08BodyCallsMethod(body *ast.BlockStmt, method string) bool {
	calls := false
	ast.Inspect(body, func(node ast.Node) bool {
		if calls {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == method {
				calls = true
			}
		}
		return !calls
	})
	return calls
}

// pbc08BodyWrites 判定函数体是否引用 RequireExecutor 或调用 EnqueueOnce。
func pbc08BodyWrites(body *ast.BlockStmt) bool {
	writes := false
	ast.Inspect(body, func(node ast.Node) bool {
		if writes {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "RequireExecutor" {
				writes = true
			}
		case *ast.CallExpr:
			if calledName(n) == "EnqueueOnce" {
				writes = true
			}
		}
		return !writes
	})
	return writes
}

func pbc08ReceiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return lastTypeIdent(fn.Recv.List[0].Type)
}

// lastTypeIdent 取类型表达式末端的类型名：*adapter.IntakeAdoptions → IntakeAdoptions。
func lastTypeIdent(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return lastTypeIdent(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return lastTypeIdent(t.X)
	}
	return ""
}

func calledName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// pbc08Evidence 是一个包内已举证的（类型, 方法）集合；包级函数以空类型名登记。
type pbc08Evidence map[string]map[[2]string]bool

func (evidence pbc08Evidence) add(pkg, typeName, method string) {
	if evidence[pkg] == nil {
		evidence[pkg] = map[[2]string]bool{}
	}
	evidence[pkg][[2]string{typeName, method}] = true
}

func (evidence pbc08Evidence) covers(method pbc08WriteMethod) bool {
	return evidence[method.pkg][[2]string{method.typeName, method.method}]
}

// collectPBC08Evidence 从测试源里收负向证据。
func collectPBC08Evidence(testSources []parsedSource) pbc08Evidence {
	type packageShapes struct {
		// 夹具签名：函数名 → 结果类型名按位置排列。
		helperResults map[string][]string
		// 测试文件里声明的结构类型：类型名 → 字段名 → 字段类型名。
		structFields map[string]map[string]string
		files        []*ast.File
	}
	byPackage := map[string]*packageShapes{}
	for _, source := range testSources {
		shapes := byPackage[source.pkg]
		if shapes == nil {
			shapes = &packageShapes{
				helperResults: map[string][]string{},
				structFields:  map[string]map[string]string{},
			}
			byPackage[source.pkg] = shapes
		}
		shapes.files = append(shapes.files, source.syntax)
		for _, decl := range source.syntax.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Type.Results == nil {
					continue
				}
				var results []string
				for _, field := range d.Type.Results.List {
					name := lastTypeIdent(field.Type)
					count := len(field.Names)
					if count == 0 {
						count = 1
					}
					for i := 0; i < count; i++ {
						results = append(results, name)
					}
				}
				shapes.helperResults[d.Name.Name] = results
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					fields := map[string]string{}
					for _, field := range structType.Fields.List {
						typeName := lastTypeIdent(field.Type)
						for _, name := range field.Names {
							fields[name.Name] = typeName
						}
					}
					shapes.structFields[typeSpec.Name.Name] = fields
				}
			}
		}
	}

	evidence := pbc08Evidence{}
	for pkg, shapes := range byPackage {
		for _, file := range shapes.files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if !pbc08ReferencesTransactionRequired(fn.Body) {
					continue
				}
				variableTypes := pbc08VariableTypes(fn.Body, shapes.helperResults)
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch fun := call.Fun.(type) {
					case *ast.Ident:
						// 包级函数的直接调用（含 dot import 场合）。
						evidence.add(pkg, "", fun.Name)
					case *ast.SelectorExpr:
						method := fun.Sel.Name
						switch receiver := fun.X.(type) {
						case *ast.Ident:
							if typeName := variableTypes[receiver.Name]; typeName != "" {
								evidence.add(pkg, typeName, method)
							} else {
								// receiver 可能是包名（qual.EnqueueOnce）：按包级函数登记。
								evidence.add(pkg, "", method)
							}
						case *ast.SelectorExpr:
							// 结构夹具字段：s.field.Method(...)。
							base, ok := receiver.X.(*ast.Ident)
							if !ok {
								return true
							}
							holder := variableTypes[base.Name]
							if holder == "" {
								return true
							}
							if fieldType := shapes.structFields[holder][receiver.Sel.Name]; fieldType != "" {
								evidence.add(pkg, fieldType, method)
							}
						}
					}
					return true
				})
			}
		}
	}
	return evidence
}

func pbc08ReferencesTransactionRequired(body *ast.BlockStmt) bool {
	references := false
	ast.Inspect(body, func(node ast.Node) bool {
		if references {
			return false
		}
		if ident, ok := node.(*ast.Ident); ok && ident.Name == "ErrTransactionRequired" {
			references = true
		}
		return !references
	})
	return references
}

// pbc08VariableTypes 解析测试函数体内「变量 → 类型名」的绑定。
func pbc08VariableTypes(body *ast.BlockStmt, helperResults map[string][]string) map[string]string {
	variableTypes := map[string]string{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledName(call)
		if strings.HasPrefix(name, "New") && len(name) > len("New") {
			// 构造器约定：New<T> 交回 <T>（或 (<T>, error)），值在 LHS 首位。
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok && ident.Name != "_" {
				variableTypes[ident.Name] = strings.TrimPrefix(name, "New")
			}
			return true
		}
		results, known := helperResults[name]
		if !known {
			return true
		}
		for i, lhs := range assign.Lhs {
			if i >= len(results) {
				break
			}
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name == "_" {
				continue
			}
			variableTypes[ident.Name] = results[i]
		}
		return true
	})
	return variableTypes
}

// TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence 守 PBC-08 行为面。
func TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence(t *testing.T) {
	writeMethods := collectPBC08WriteMethods(parseRepositorySources(t))
	evidence := collectPBC08Evidence(parseRepositoryTestSources(t))

	for _, method := range writeMethods {
		if evidence.covers(method) {
			continue
		}
		name := method.method
		if method.typeName != "" {
			name = method.typeName + "." + method.method
		}
		t.Errorf("%s 的写口 %s 缺无事务负向证据：同包测试里没有「引用 ErrTransactionRequired "+
			"且以该类型接收者调用该方法」的测试函数。写口在无事务上下文必须被 RequireExecutor "+
			"拒绝，而这条性质随适配器逐个成立，请在该包补一条负向断言（形状参照各包既有的 "+
			"*RefuseToRunOutsideATransaction 用例）", method.pkg, name)
	}
}

// TestThePBC08GateCanActuallyCatchAViolation 证这道门禁真能红，并证三条归属路径
// （直接构造、元组夹具、结构夹具）各自认得出证据——与本包其余门禁同一约定：
// 一个从来不会失败的门禁比没有门禁更坑人。
func TestThePBC08GateCanActuallyCatchAViolation(t *testing.T) {
	adapterSource := `package p
func (s *Alpha) Save(ctx context.Context) error {
	if _, err := s.db.RequireExecutor(ctx); err != nil { return err }
	return nil
}
func (s *Beta) Replace(ctx context.Context) error {
	if _, err := s.db.RequireExecutor(ctx); err != nil { return err }
	return nil
}
func (s *Gamma) Register(ctx context.Context) error {
	if _, err := s.db.RequireExecutor(ctx); err != nil { return err }
	return nil
}
func (s *Delta) HandOff(ctx context.Context) error {
	return outboxintent.EnqueueOnce(ctx, s.db, s.outbox, envelope)
}
func (s *Epsilon) next(ctx context.Context) error {
	if _, err := s.db.RequireExecutor(ctx); err != nil { return err }
	return nil
}
func (s *Epsilon) Advance(ctx context.Context) error {
	return s.next(ctx)
}
func Orphan(ctx context.Context) error {
	if _, err := db.RequireExecutor(ctx); err != nil { return err }
	return nil
}
`
	testSource := `package p
type bundle struct {
	gamma *adapter.Gamma
}
func newBundle(t *testing.T) bundle { return bundle{} }
func newPairStores(t *testing.T) (*adapter.Beta, *adapter.Delta) { return nil, nil }
func TestAlphaRefuses(t *testing.T) {
	alpha, err := adapter.NewAlpha(db)
	_ = err
	if _, err := alpha.Save(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) { t.Error(err) }
}
func TestPairRefuses(t *testing.T) {
	beta, delta := newPairStores(t)
	if _, err := beta.Replace(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) { t.Error(err) }
	if err := delta.HandOff(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) { t.Error(err) }
}
func TestBundleRefuses(t *testing.T) {
	stores := newBundle(t)
	if _, err := stores.gamma.Register(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) { t.Error(err) }
}
func TestEpsilonRefuses(t *testing.T) {
	epsilon, err := adapter.NewEpsilon(db)
	_ = err
	if err := epsilon.Advance(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) { t.Error(err) }
}
func TestOrphanIsExercisedWithoutTheSentinel(t *testing.T) {
	_ = Orphan(ctx)
}
`
	parse := func(name, src string) parsedSource {
		syntax, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
		if err != nil {
			t.Fatalf("解析合成源码：%v", err)
		}
		return parsedSource{pkg: modulePath + "/internal/p", path: name, syntax: syntax}
	}

	writeMethods := collectPBC08WriteMethods([]parsedSource{parse("adapter.go", adapterSource)})
	if len(writeMethods) != 6 {
		t.Fatalf("收到 %d 个写方法，want 6（Alpha.Save/Beta.Replace/Gamma.Register/Delta.HandOff/"+
			"Epsilon.Advance/Orphan；Epsilon.next 应记账到导出调用方 Advance 而不是自身）：%v",
			len(writeMethods), writeMethods)
	}
	evidence := collectPBC08Evidence([]parsedSource{parse("adapter_test.go", testSource)})

	var missing []string
	for _, method := range writeMethods {
		if !evidence.covers(method) {
			missing = append(missing, method.typeName+"."+method.method)
		}
	}
	// Orphan 的调用在一个不引用 ErrTransactionRequired 的测试里——光调用不算证据，
	// 它必须是唯一的漏网者：四条归属路径（直接构造/元组夹具/结构夹具/未导出记账到
	// 导出调用方）都认出了各自那条证据，EnqueueOnce 途径的写方法同样被收进了待举证清单。
	if len(missing) != 1 || missing[0] != ".Orphan" {
		t.Fatalf("漏网清单 = %v，want 恰好 [.Orphan]", missing)
	}
}
