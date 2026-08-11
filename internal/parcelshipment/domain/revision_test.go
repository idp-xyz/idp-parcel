package domain_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// TestNoStateTransitionMovesTheAggregateRevision 守 ADR-0028「状态转移一律不动版本」。
//
// 版本由仓储在写入成功后推进：一次保存对应一次推进，而两次转移之间只保存一次。转移各自加一
// 会让版本跳号，框架合同当场不成立；转移把版本刷回零更坏——随后一次按预期版本的写入要么被
// 当成并发冲突失败，要么覆盖掉别人的写入。
//
// 要守的风险不是「忘了带」，是日后某个转移改成重新构造一个 `ShipmentRequest{…}`，或改成指针
// 接收者就地改。所以本条不手工列举转移，而是反射枚举出全部合法形状的转移再断言每一条都
// 被下表盖到：ADR-0028 点名否掉了「六处都要记得」，理由是它守不到第七个转移出现那天。
//
// 合法形状由 ADR 写死：「转移以值接收者复制整份聚合再返回」。因此本条同时拒绝另外两种写法：
// 指针接收者、以及返回值不是恰好 `(ShipmentRequest, error)` 的方法——它们不是「漏覆盖」，
// 是形状本身违 ADR；扫不到却声称守住，比没有守卫更坏（ADR-0029 末尾同一条道理）。
// 包级函数形状由 TestNoPackageLevelFunctionActsAsAShipmentRequestTransition 另守。
//
// 它直到重建入口落地才写得出来：版本非零的聚合只有那扇门造得出，`SubmitShipmentRequest`
// 产出的永远是零，而零过一遍转移还是零——那样的断言恒真，绿得毫无意义。
func TestNoStateTransitionMovesTheAggregateRevision(t *testing.T) {
	transitions := map[string]func(*testing.T, domain.ShipmentRequest) (domain.ShipmentRequest, error){
		"Decide": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.Decide(decisionSpec(t, everyGroupPassingFor(t, "parcel-1")))
		},
		"RejectByAuthority": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.RejectByAuthority(activeRejectionSpec(t))
		},
		"WithdrawByCustomer": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.WithdrawByCustomer(withdrawalSpec(t))
		},
		"CompleteManualReview": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.CompleteManualReview(reviewCompletion(t))
		},
		"RecordProcessingAttempt": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.RecordProcessingAttempt(
				internalAttempt(t, "COMMERCIAL_BASIS_UNAVAILABLE", firstAttemptAt),
			)
		},
		// 资料修订只对`已接受`开放，所以这一条要先越过决定边界。中间那次 Decide 同样不许动
		// 版本，链起来测反而比单测更接近真实调用序列。
		"AmendCustomerSourceData": func(t *testing.T, request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			accepted, err := request.Decide(decisionSpec(t, everyGroupPassingFor(t, "parcel-1")))
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			version, err := domain.FormCustomerSourceDataVersion(amendmentSpec(t))
			if err != nil {
				t.Fatalf("form customer source data version: %v", err)
			}
			return accepted.AmendCustomerSourceData(version)
		},
	}

	classified := classifyAggregateTransitions(reflect.TypeOf(domain.ShipmentRequest{}))
	if len(classified.valueTransitions) == 0 {
		t.Fatal("没有反射到任何状态转移；本用例会永远空过")
	}
	for _, violation := range classified.shapeViolations {
		t.Errorf("%s；ADR-0028 要求转移以值接收者返回 (ShipmentRequest, error)，"+
			"别的形状会让本用例扫不到它——看起来像在守，实际是假阴性", violation)
	}
	for _, name := range classified.valueTransitions {
		if _, covered := transitions[name]; !covered {
			t.Errorf("ShipmentRequest.%s 是一条状态转移，本用例却没在守它不动聚合版本；"+
				"新转移要在表里加一行——ADR-0028 明否「六处都要记得」，它守不到第七处", name)
		}
	}

	for name, apply := range transitions {
		t.Run(name, func(t *testing.T) {
			start, err := domain.RehydrateShipmentRequest(submittedSnapshot(t))
			if err != nil {
				t.Fatalf("rehydrate: %v", err)
			}
			if start.Revision() == 0 {
				t.Fatal("前置不成立：起点聚合版本为零，「转移不动版本」这条断言会恒真")
			}

			moved, err := apply(t, start)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if moved.Revision() != start.Revision() {
				t.Fatalf("revision %d → %d；转移动了聚合版本，「一次保存对应一次推进」的合同就此不成立",
					start.Revision(), moved.Revision())
			}
		})
	}
}

// TestTransitionClassificationCatchesPointerAndOddSignatures 证分类器真能看见假阴性那两类。
//
// 上一条用例对今天的 ShipmentRequest 全绿，证明不了它挡得住「改成指针接收者」——那正是
// 盲区本身。这条用合成类型把指针接收者与三返回值放进方法集，断言两者都进 shapeViolations，
// 且只有值接收者的 `(T, error)` 算合法转移。
func TestTransitionClassificationCatchesPointerAndOddSignatures(t *testing.T) {
	t.Parallel()

	classified := classifyAggregateTransitions(reflect.TypeOf(shapeProbe{}))
	if len(classified.valueTransitions) != 1 || classified.valueTransitions[0] != "ValueMove" {
		t.Fatalf("valueTransitions = %v, want [ValueMove]", classified.valueTransitions)
	}
	if !containsViolation(classified.shapeViolations, "PointerMove") {
		t.Fatalf("shapeViolations = %v；指针接收者没被抓住——假阴性还在", classified.shapeViolations)
	}
	if !containsViolation(classified.shapeViolations, "TripleMove") {
		t.Fatalf("shapeViolations = %v；非 (T, error) 签名没被抓住", classified.shapeViolations)
	}
	if !containsViolation(classified.shapeViolations, "PointerReturningValue") {
		t.Fatalf("shapeViolations = %v；返回 *T 的指针方法没被抓住", classified.shapeViolations)
	}
}

// shapeProbe 只给上一条分类器用。三个违例形状 + 一个合法形状。
type shapeProbe struct{}

func (shapeProbe) ValueMove() (shapeProbe, error) { return shapeProbe{}, nil }

func (s *shapeProbe) PointerMove() (shapeProbe, error) { return *s, nil }

func (shapeProbe) TripleMove() (shapeProbe, shapeProbe, error) {
	return shapeProbe{}, shapeProbe{}, nil
}

func (s *shapeProbe) PointerReturningValue() (*shapeProbe, error) { return s, nil }

func containsViolation(items []string, method string) bool {
	for _, item := range items {
		if len(item) >= len(method) {
			for i := 0; i+len(method) <= len(item); i++ {
				if item[i:i+len(method)] == method {
					return true
				}
			}
		}
	}
	return false
}

type transitionClassification struct {
	valueTransitions []string
	shapeViolations  []string
}

// classifyAggregateTransitions 把一个聚合类型上「像转移」的导出方法分成两类：
// 合法的值接收者 `(T, error)`，以及其他任何返回了 T / *T 的形状（含指针接收者）。
//
// 必须同时扫值方法集与指针方法集：只扫值方法集时，指针接收者根本不出现——那正是 N10
// 的假阴性。值接收者方法会出现在两份方法集里，以值方法集为准去重。
func classifyAggregateTransitions(aggregateType reflect.Type) transitionClassification {
	if aggregateType.Kind() == reflect.Pointer {
		aggregateType = aggregateType.Elem()
	}
	ptrType := reflect.PointerTo(aggregateType)
	errorType := reflect.TypeOf((*error)(nil)).Elem()

	valueNames := map[string]bool{}
	classified := transitionClassification{}
	for index := 0; index < aggregateType.NumMethod(); index++ {
		method := aggregateType.Method(index)
		valueNames[method.Name] = true
		switch {
		case isValueTransitionSignature(method.Type, aggregateType, errorType):
			classified.valueTransitions = append(classified.valueTransitions, method.Name)
		case returnsAggregate(method.Type, aggregateType):
			classified = appendShape(classified,
				aggregateType.Name()+"."+method.Name+" 返回了聚合却不是 (T, error)")
		}
	}

	for index := 0; index < ptrType.NumMethod(); index++ {
		method := ptrType.Method(index)
		if valueNames[method.Name] {
			continue
		}
		if !returnsAggregate(method.Type, aggregateType) {
			continue
		}
		classified = appendShape(classified,
			aggregateType.Name()+"."+method.Name+" 是指针接收者上的转移形状（ADR-0028 要求值接收者）")
	}
	return classified
}

func appendShape(classified transitionClassification, message string) transitionClassification {
	classified.shapeViolations = append(classified.shapeViolations, message)
	return classified
}

// isValueTransitionSignature 认 `func (T) ...(?, ...) (T, error)`。
// reflect 的方法类型把接收者算作 In(0)。
func isValueTransitionSignature(methodType, aggregateType, errorType reflect.Type) bool {
	if methodType.NumOut() != 2 || methodType.NumIn() < 1 {
		return false
	}
	if methodType.In(0) != aggregateType {
		return false
	}
	return methodType.Out(0) == aggregateType && methodType.Out(1) == errorType
}

func returnsAggregate(methodType, aggregateType reflect.Type) bool {
	ptrType := reflect.PointerTo(aggregateType)
	for index := 0; index < methodType.NumOut(); index++ {
		out := methodType.Out(index)
		if out == aggregateType || out == ptrType {
			return true
		}
	}
	return false
}

// TestNoPackageLevelFunctionActsAsAShipmentRequestTransition 守第三盲区：包级函数形状。
//
// 同包函数同样设得了未导出字段，因此 `func AdvanceX(r ShipmentRequest) (ShipmentRequest, error)`
// 是一条完整的转移，却不在任何方法集里——反射枚举永远看不见它。今天领域包里没有这种函数
// （Submit / Rehydrate 都不收 ShipmentRequest）；本条让它一出现就红，而不是靠人记得去改反射。
func TestNoPackageLevelFunctionActsAsAShipmentRequestTransition(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("定位不了本文件")
	}
	domainDir := filepath.Dir(thisFile)

	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, domainDir, nil, 0)
	if err != nil {
		t.Fatalf("解析领域包：%v", err)
	}

	scannedFiles := 0
	anchored := false
	for _, pkg := range packages {
		for path, file := range pkg.Files {
			if filepath.Base(path) == filepath.Base(thisFile) {
				continue
			}
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			scannedFiles++
			if filepath.Base(path) == "shipment_request.go" {
				anchored = true
			}
			for _, decl := range file.Decls {
				funcDecl, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || !funcDecl.Name.IsExported() {
					continue
				}
				// 只跳接收者是 ShipmentRequest 的方法——它们由反射那一半枚举。**按「有没有
				// 接收者」筛会漏掉中间那一格**：`func (spec SomeSpec) ApplyTo(r ShipmentRequest)
				// (ShipmentRequest, error)` 既不在 ShipmentRequest 的方法集里（反射看不见），
				// 也不是包级函数（旧筛法跳过），而它同包因此照样设得了未导出字段，是一条完整
				// 的转移。
				if receiverTypeNameOf(funcDecl) == "ShipmentRequest" {
					continue
				}
				if !funcTakesAndReturnsNamedType(funcDecl, "ShipmentRequest") {
					continue
				}
				t.Errorf("%s：导出函数 %s 同时收下并交回 ShipmentRequest，却不是 ShipmentRequest "+
					"上的方法；这是一条反射枚举看不见的状态转移，请改成值接收者方法，或把它从"+
					"转移角色里拿掉",
					filepath.Base(path), funcDecl.Name.Name)
			}
		}
	}
	// 今天期望是零，所以本条正常情况下永远绿——而「永远绿」既可能因为没人违规，也可能因为
	// 扫描根本没生效。用 `found < 0` 当守卫挡不住后者：那个条件恒不成立。锚在真实实例上：
	// 扫到了非测试文件，且扫到了聚合定义所在的那个文件。判据本身能不能看见违规，由
	// TestTheTransitionScanSeesMethodsOnOtherTypes 用合成源码另证。
	if scannedFiles == 0 {
		t.Fatal("没扫到领域包的任何非测试文件；本条会永远空过")
	}
	if !anchored {
		t.Fatal("没扫到 shipment_request.go；文件锚已失效，扫描面可能已经塌了")
	}
}

// receiverTypeNameOf 取方法接收者的类型名，指针剥到底层那个标识符；包级函数交回空串。
func receiverTypeNameOf(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return ""
	}
	return identName(funcDecl.Recv.List[0].Type)
}

// TestTheTransitionScanSeesMethodsOnOtherTypes 证上一条的判据真能看见那条夹在中间的漏法。
//
// 上一条对今天的领域包全绿，证明不了它挡得住 `func (spec SomeSpec) ApplyTo(...)`——那正是
// 盲区本身。这条用合成源码把四种形状分开验：挂在别的类型上的方法要被认出来，包级函数照旧要
// 被认出来，挂在 ShipmentRequest 上的方法要被跳过（反射那一半管它），既不收也不交回的不算。
func TestTheTransitionScanSeesMethodsOnOtherTypes(t *testing.T) {
	t.Parallel()

	source := `package domain
type SomeSpec struct{}
func (spec SomeSpec) ApplyTo(r ShipmentRequest) (ShipmentRequest, error) { return r, nil }
func (request ShipmentRequest) Decide(n int) (ShipmentRequest, error) { return request, nil }
func AdvanceX(r ShipmentRequest) (ShipmentRequest, error) { return r, nil }
func Unrelated(n int) int { return n }
`
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", source, 0)
	if err != nil {
		t.Fatalf("解析合成源码：%v", err)
	}

	var flagged []string
	for _, decl := range file.Decls {
		funcDecl, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || !funcDecl.Name.IsExported() {
			continue
		}
		if receiverTypeNameOf(funcDecl) == "ShipmentRequest" {
			continue
		}
		if funcTakesAndReturnsNamedType(funcDecl, "ShipmentRequest") {
			flagged = append(flagged, funcDecl.Name.Name)
		}
	}

	if strings.Join(flagged, ",") != "ApplyTo,AdvanceX" {
		t.Fatalf("flagged = %v, want [ApplyTo AdvanceX]；"+
			"ApplyTo 是挂在别的类型上的转移，按「有没有接收者」筛会让它从两半中间漏过去", flagged)
	}
}

// TestEveryShipmentRequestFieldIsClassifiedForRehydration 守重建入口不静默漏掉新字段。
//
// `RehydrateShipmentRequest` 用具名字段的结构体字面量只写了七个字段，其余六个留零值——对
// `已提交`这是对的（ADR-0030 逐条证过那六个在这个状态下必然缺席）。但 Go 的具名字段字面量
// **不要求穷尽**：日后给 `ShipmentRequest` 加一个在`已提交`下就有值的字段，重建会静默把它
// 留零，而 ADR-0028 说重建入口「此后是审计与评审的固定关注点」——固定关注点靠人记得是不够的。
// 形状与 ADR-0028 亲自解决过的「第七个转移」相同，那里明否了「六处都要记得」。
//
// 因此这里要的不是「字段数没变」，而是**每个字段恰好落在两份名单之一**，且带进来的那一份
// 与字面量真正写下的键**逐个相等**——只有分类名单而不比对字面量，一个字段可以登记在册却
// 从没被赋值，而那种漏法正是本条要防的。
func TestEveryShipmentRequestFieldIsClassifiedForRehydration(t *testing.T) {
	t.Parallel()

	// carriedByRehydration 是重建入口必须原样带进来的字段。
	carriedByRehydration := map[string]bool{
		"revision":          true,
		"shipmentRequestID": true,
		"batchID":           true,
		"state":             true,
		"currentVersion":    true,
		"acceptanceTask":    true,
		"submittedAt":       true,
	}
	// absentInSubmitted 是`已提交`下必然缺席的判断产物。这扇门开到别的状态那天，它们要从
	// 这一份挪到上一份并各自补上快照表达，而不是继续留零——ADR-0030 的入口条件说的就是它。
	absentInSubmitted := map[string]bool{
		"withdrawal":         true,
		"decision":           true,
		"decisionFormed":     true,
		"baseline":           true,
		"commitment":         true,
		"sourceDataVersions": true,
	}

	aggregate := reflect.TypeOf(domain.ShipmentRequest{})
	if aggregate.NumField() == 0 {
		t.Fatal("没反射到任何字段；本条会永远空过")
	}
	declared := map[string]bool{}
	for index := 0; index < aggregate.NumField(); index++ {
		name := aggregate.Field(index).Name
		declared[name] = true
		carried, absent := carriedByRehydration[name], absentInSubmitted[name]
		switch {
		case carried && absent:
			t.Errorf("ShipmentRequest.%s 两份名单都登记了；两份必须互斥", name)
		case !carried && !absent:
			t.Errorf("ShipmentRequest.%s 没有分类。它在`已提交`下有值，就补进 carriedByRehydration "+
				"并在 RehydrateShipmentRequestSpec 上给它一个表达；在`已提交`下必然缺席，就补进 "+
				"absentInSubmitted 并说明由哪条转移写下。留着不分类，重建会静默把它留零", name)
		}
	}
	for name := range carriedByRehydration {
		if !declared[name] {
			t.Errorf("carriedByRehydration 里的 %q 在聚合上已不存在；名单该清理", name)
		}
	}
	for name := range absentInSubmitted {
		if !declared[name] {
			t.Errorf("absentInSubmitted 里的 %q 在聚合上已不存在；名单该清理", name)
		}
	}

	assigned := rehydrationLiteralKeys(t)
	if len(assigned) == 0 {
		t.Fatal("没在 RehydrateShipmentRequest 里找到 ShipmentRequest 字面量；本条后半段会永远空过")
	}
	for name := range carriedByRehydration {
		if !assigned[name] {
			t.Errorf("%s 登记为「重建要带进来」，而 RehydrateShipmentRequest 的字面量没有赋它；"+
				"它会静默留零值", name)
		}
	}
	for name := range assigned {
		if !carriedByRehydration[name] {
			t.Errorf("RehydrateShipmentRequest 的字面量赋了 %s，而它没登记在 carriedByRehydration 里；"+
				"两份必须一致，否则名单说明不了这扇门到底带进来什么", name)
		}
	}
}

// rehydrationLiteralKeys 取 RehydrateShipmentRequest 里那个 ShipmentRequest 字面量赋了哪些字段。
//
// 走 AST 而不是反射：字段未导出，包外读不到值，而「赋没赋过」恰恰是要判的东西。只认顶层那个
// 字面量的键；嵌套的 SubmissionVersion / AcceptanceDecisionTask 字面量各有自己的字段集，混进来
// 会让两份名单对不上。
func rehydrationLiteralKeys(t *testing.T) map[string]bool {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("定位不了本文件")
	}
	path := filepath.Join(filepath.Dir(thisFile), "rehydration.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("解析 rehydration.go：%v", err)
	}

	keys := map[string]bool{}
	for _, decl := range file.Decls {
		funcDecl, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || funcDecl.Name.Name != "RehydrateShipmentRequest" {
			continue
		}
		ast.Inspect(funcDecl, func(node ast.Node) bool {
			literal, isLiteral := node.(*ast.CompositeLit)
			if !isLiteral || identName(literal.Type) != "ShipmentRequest" {
				return true
			}
			for _, element := range literal.Elts {
				pair, isPair := element.(*ast.KeyValueExpr)
				if !isPair {
					continue
				}
				if key, isIdent := pair.Key.(*ast.Ident); isIdent {
					keys[key.Name] = true
				}
			}
			// 顶层那个字面量已经取到，不再往里走——嵌套字面量的键不属于本聚合。
			return false
		})
	}
	return keys
}

func funcTakesAndReturnsNamedType(funcDecl *ast.FuncDecl, typeName string) bool {
	if funcDecl.Type.Results == nil || funcDecl.Type.Params == nil {
		return false
	}
	takes, returns := false, false
	for _, field := range funcDecl.Type.Params.List {
		if identName(field.Type) == typeName {
			takes = true
		}
	}
	for _, field := range funcDecl.Type.Results.List {
		if identName(field.Type) == typeName {
			returns = true
		}
	}
	return takes && returns
}

func identName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return identName(typed.X)
	default:
		return ""
	}
}
