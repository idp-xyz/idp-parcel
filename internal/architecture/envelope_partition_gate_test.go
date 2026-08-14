package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"testing"
)

// 信封分区键门禁：`ID` 与 `PartitionKey` 不得取自同一个表达式。
//
// 两个字段管的不是一回事——**ID 管幂等**（同一份意图不发第二遍），**分区键管顺序**（同一
// 业务对象的先后拍排在一条队里）。写成同一个字符串时，两个目标只能满足一个，而选中哪一个
// 取决于那个字符串里有没有区分维：
//
//   - 含版本或状态后缀 → 更正另成一个 ID，**不丢**，但落进另一个分区，**乱序**；
//   - 不含 → 更正撞上 `outboxintent.EnqueueOnce` 的先查后插，**静默不入队**，而编排看到的是
//     「交接成功」。
//
// 四个上下文各自查出来的十个更正入口里，没有一个同时做对两件；三个做对的全部是「没有更正
// 入口或当时就想过分区」。这不是巧合：`PartitionKey: eventID` 是一个写起来完全自然的默认，
// **而它在没有更正入口时完全无害**——错误只在后来多出第二条信封时才显形，那时写它的人已经
// 走了。靠人记不住，所以立成门禁。
//
// 正确形状（仓里现成，不是新发明）：`ID` 装区分维（有版本用版本，无版本用状态后缀，**不得
// 为此在领域里加假版本字段**），`PartitionKey` 装业务主体 `租户/<主体>`。例：
// parcel-shipment 的 `source_data_handoff`（租户/来源请求键）、customs-compliance 的
// restriction（租户/范围）。
//
// **这道门禁守不住的那一格，写在这里而不是假装守住了：主体取多粗是判断，不是套公式。**
// 把 `eventID` 展开成它的各个维再拼起来（`租户/对象/种类/版本`），表达式不再同源因而本门禁
// 放行，**而它与逐事件分区一模一样**——每个信封仍然自成一区。主体要取「其先后状态必须保序
// 的那个对象」，不是取键的全部维：transport-fulfillment 的交接登记取到（租户+对象）而不是
// （租户+对象+范围），因为控制转移对一个载运对象是一条链（先从节点交出、再由承运方接收），
// 取到范围就把这条链切成互不排队的两段。
//
// 补这一格的是**每处修复自带一条断言**：同一对象的两个版本都入队（ID 带区分维所以不丢）
// 且落在同一分区（分区键只到对象所以保序）。门禁守「不得同源」，断言守「主体取得对不对」，
// 两者缺一不可。

// envelopeType 是被查的复合字面量类型。只认这一种：本门禁守的是这份合同的两个字段。
const envelopeType = "eventing.Envelope"

// pendingDecision 是例外清单的分组维。
//
// 清单按「在等哪一个决定」分组而不是按位置罗列——按位置罗列是一张靠意志变短的清单，没有
// 完结条件；按决定分组则有：那个决定一落，整个类别同时变得可修。清单长度因此成为一个信号
// （不变短只说明决定还没落），而一张杂项清单不变短可以是任何原因，因此不信号任何东西。
//
// 新增例外要先回答「你为什么也在等这个决定」。答不上来的，就不是例外，是没改。
const decisionPartitionKeyRollout = "等地盘主人按已定口径修：第一步只改分区键（不动 ID，" +
	"因而不变更意图契约），第二步改需要区分维的那些 ID 语义"

// allowedSameExpression 是本门禁落地那一刻已经存在的位置，全部挂在同一个决定上。**每修一处
// 删一行。**
//
// 只有一个理由值，是有意的：这些位置等的是同一件事（口径落地后各主人执行），因此清单不变短
// 只说明一件事——还没人动。若日后出现第二个理由，它会在这里自己显形，而不是混进一堆位置里
// 看不见。
//
// **清单对新来者是关的**：口径既已定，新写的 handoff 答不出「我在等哪个决定」，因而拿不到
// 例外。这就是它与一张「先放着」清单的全部差别。
//
// 清单只许变短由 TestNoEnvelopeTakesItsIDAndPartitionKeyFromTheSameExpression 的后半段守着：
// 一处修好了却留在清单里，同样报错。写这份清单时我凭记忆填了两个不违规的文件，就是被那一段
// 当场抓出来的。
var allowedSameExpression = map[string]string{
	"internal/customscompliance/adapters/postgres/case_closure_handoff.go":           decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/customs_case_handoff.go":           decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/declaration_submission_handoff.go": decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/external_result_handoff.go":        decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/follow_up_handoff.go":              decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/gate_verification_handoff.go":      decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/manifest_handoff.go":               decisionPartitionKeyRollout,
	"internal/customscompliance/adapters/postgres/verification_handoff.go":           decisionPartitionKeyRollout,

	"internal/networkrouting/adapters/postgres/initial_route_handoff.go": decisionPartitionKeyRollout,

	"internal/nodeoperations/adapters/postgres/collaboration_acceptance_handoff.go": decisionPartitionKeyRollout,
	"internal/nodeoperations/adapters/postgres/execution_fact_handoff.go":           decisionPartitionKeyRollout,
	"internal/nodeoperations/adapters/postgres/node_intake_handoff.go":              decisionPartitionKeyRollout,
	"internal/nodeoperations/adapters/postgres/sealed_snapshot_handoff.go":          decisionPartitionKeyRollout,

	"internal/pilotgovernance/adapters/postgres/governance_handoff.go": decisionPartitionKeyRollout,

	"internal/settlementaccounting/adapters/postgres/advance_recovery_handoff.go":       decisionPartitionKeyRollout,
	"internal/settlementaccounting/adapters/postgres/charge_confirmation_handoff.go":    decisionPartitionKeyRollout,
	"internal/settlementaccounting/adapters/postgres/claim_settlement_handoff.go":       decisionPartitionKeyRollout,
	"internal/settlementaccounting/adapters/postgres/operating_handoff.go":              decisionPartitionKeyRollout,
	"internal/settlementaccounting/adapters/postgres/settlement_application_handoff.go": decisionPartitionKeyRollout,
	"internal/settlementaccounting/adapters/postgres/supplier_bill_handoff.go":          decisionPartitionKeyRollout,

	"internal/transportfulfillment/adapters/postgres/capacity_consumption_handoff.go":        decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/disposition_execution_handoff.go":       decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/exception_journey_handoff.go":           decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_handoff.go":              decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_registration_handoff.go": decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/regulatory_acceptance_handoff.go":       decisionPartitionKeyRollout,
	"internal/transportfulfillment/adapters/postgres/transport_commission_handoff.go":        decisionPartitionKeyRollout,
}

// sameExpressionViolation 是一处两字段同源。
type sameExpressionViolation struct {
	path       string
	expression string
}

// collectSameExpressionEnvelopes 找出把同一个表达式同时赋给 ID 与 PartitionKey 的信封构造。
//
// 比较前剥掉类型转换：`ID: eventing.EventID(eventID)` 与 `PartitionKey: eventID` 在源码上不
// 相同，指的却是同一个字符串——只比字面文本会漏掉全仓最常见的那一种写法。
func collectSameExpressionEnvelopes(syntax *ast.File, path string) []sameExpressionViolation {
	var found []sameExpressionViolation

	ast.Inspect(syntax, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || literal.Type == nil || types.ExprString(literal.Type) != envelopeType {
			return true
		}
		id, hasID := envelopeField(literal, "ID")
		partition, hasPartition := envelopeField(literal, "PartitionKey")
		if !hasID || !hasPartition {
			return true
		}
		rendered := types.ExprString(unwrapConversions(id))
		if rendered != types.ExprString(unwrapConversions(partition)) {
			return true
		}
		found = append(found, sameExpressionViolation{path: path, expression: rendered})
		return true
	})
	return found
}

func envelopeField(literal *ast.CompositeLit, name string) (ast.Expr, bool) {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if ok && key.Name == name {
			return pair.Value, true
		}
	}
	return nil, false
}

// unwrapConversions 剥掉单参数的类型转换外壳，直到剩下核心表达式。
//
// 只剥「函数位是标识符或选择器、且恰好一个实参」的调用——那是类型转换的形状。真正的函数
// 调用（比如 `eventID(key)`）也会被剥掉一层，但那反而是想要的：两个字段各自调同一个函数、
// 传同一个参数，与直接共用一个变量是同一件事。
func unwrapConversions(expr ast.Expr) ast.Expr {
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return expr
		}
		switch call.Fun.(type) {
		case *ast.Ident, *ast.SelectorExpr:
			expr = call.Args[0]
		default:
			return expr
		}
	}
}

// TestNoEnvelopeTakesItsIDAndPartitionKeyFromTheSameExpression 守那一条规则。
func TestNoEnvelopeTakesItsIDAndPartitionKeyFromTheSameExpression(t *testing.T) {
	var violations []sameExpressionViolation
	for _, source := range parseRepositorySources(t) {
		violations = append(violations, collectSameExpressionEnvelopes(source.syntax, source.path)...)
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].path < violations[j].path })

	excused := make(map[string]bool, len(allowedSameExpression))
	for _, violation := range violations {
		if reason, allowed := allowedSameExpression[violation.path]; allowed {
			excused[violation.path] = true
			t.Logf("已知例外 %s（%s）：%s", violation.path, violation.expression, reason)
			continue
		}
		t.Errorf("%s 把 %s 同时用作 ID 与 PartitionKey；"+
			"ID 应装区分维（版本或状态后缀），PartitionKey 应装业务主体（租户/<主体>）",
			violation.path, violation.expression)
	}

	// 例外清单只许变短。留着一条已经修好的例外，下一次同处回归就不会红。
	for path := range allowedSameExpression {
		if !excused[path] {
			t.Errorf("例外清单里的 %s 已经不违规了，请把它从 allowedSameExpression 删掉", path)
		}
	}
}

// TestTheEnvelopePartitionGateCanActuallyCatchAViolation 证这道门禁真能红。
//
// 与本包另三套门禁同一约定：一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信。
func TestTheEnvelopePartitionGateCanActuallyCatchAViolation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "裸变量同时给两个字段",
			source: "package p\nfunc f() { _ = eventing.Envelope{" +
				"ID: eventID, PartitionKey: eventID} }\n",
			want: 1,
		},
		{
			name: "ID 外面套一层类型转换，仍算同源",
			source: "package p\nfunc f() { _ = eventing.Envelope{" +
				"ID: eventing.EventID(eventID), PartitionKey: eventID} }\n",
			want: 1,
		},
		{
			name: "两个字段各调同一个函数传同一个参数，也算同源",
			source: "package p\nfunc f() { _ = eventing.Envelope{" +
				"ID: eventing.EventID(intakeID(key)), PartitionKey: intakeID(key)} }\n",
			want: 1,
		},
		{
			name: "分区键取业务主体——正确形状，不算",
			source: "package p\nfunc f() { _ = eventing.Envelope{" +
				`ID: eventing.EventID(eventID), PartitionKey: tenant + "/" + parcel} }` + "\n",
			want: 0,
		},
		{
			name:   "缺其中一个字段的字面量不算——那是别处的构造",
			source: "package p\nfunc f() { _ = eventing.Envelope{ID: eventID} }\n",
			want:   0,
		},
		{
			name: "别的类型同名两字段不算",
			source: "package p\nfunc f() { _ = other.Thing{" +
				"ID: eventID, PartitionKey: eventID} }\n",
			want: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			got := collectSameExpressionEnvelopes(syntax, "synthetic.go")
			if len(got) != test.want {
				t.Fatalf("认出 %d 处，want %d 处：%v", len(got), test.want, got)
			}
		})
	}
}

// TestTheExceptionListNamesTheDecisionItWaitsOn 守清单的分组方式本身。
//
// 例外可以有，但必须挂在一个具名的待决决定上。一条写不出「在等什么」的例外，与「先放着」
// 没有区别，而「先放着」是没有完结条件的。
func TestTheExceptionListNamesTheDecisionItWaitsOn(t *testing.T) {
	for path, reason := range allowedSameExpression {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s 的例外没有写明在等哪一个决定", path)
		}
	}
}
