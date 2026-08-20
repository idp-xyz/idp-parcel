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

// 例外清单的标注取值。标的是**可复核的判据**，不是「无害」那种复核不了的结论——判据看一眼
// 那个方法还在不在、键里有没有版本维、两个状态是不是同一对象的就能核；结论只能重新把领域读
// 一遍，而复核不了的结论会把误判一直藏下去。
//
// 判据是一句话：**后一条会不会改写或取代前一条说过的事。** 五取其一，答不出再取第六格：
//
//	更正入口：<Type>.<Method>    有先后（真风险）——显式更正 / 重派生 / 撤销入口
//	版本进键：<键里的版本维>      有先后（真风险）——无更正方法，但同主体多版
//	状态序列：<A> → <B>         有先后（真风险）——无更正方法，先后两条是同一对象的相继状态
//	依赖前序：<B> 引用 <A>       有先后但可自愈——见下
//	无先后：<为何可交换>          后一条不改写前一条
//	待裁：<答不出的那一句>        判不准，等人裁——既不算真风险也不算无害
//
// **剩余真风险 = 前三类的行数**；清单总长 = 剩余工量。**待裁那几行是未知，不是零**——把它们
// 记进无害会让工量看起来已经收敛，而它们恰恰是最可能藏着缺陷的几行。
//
// 六个前缀由 TestEveryExceptionCarriesACheckableVerdict 强制。没有这道检查，「待地盘主人标注」
// 那种既非判据也非结论的占位就能在清单里坐满一整轮——它正是这么发生的。
//
// 「依赖前序」单列而不计入真风险，是因为它的失效方式不同：前三类是**重述乱序**，两条都被成功
// 处理、只是顺序反了，消费方无从得知，最终状态静默错；而依赖乱序会让消费方找不到被引用的对象
// 因而报错回滚重投，前序到了就好——是噪音不是丢失。
//
// **但这条自愈有前提，且前提今天还没人定：**它要求消费方把「引用对象尚未到达」当作**可重试**
// 失败。ADR-0049 定了毒丸显式拒收入账，若哪个消费方把这一格归进毒丸，那份信封就真丢了。而
// 归类是消费方的选择，**眼下一个消费者都还没建**——所以这条约束现在记下来成本为零，等有了
// 消费者再发现就是丢数据之后才发现。
//
// 四类缺一不可，而这个分类是三个通道各漏一类凑出来的，不是谁一次设计对的：
//
//   - 只问「有无更正入口」会漏掉**版本进键**——`supplier_bill_handoff.go` 就是：没有 Replace，
//     但版本在键里因而同一主张多条，v2 重述 v1 说过的事。
//   - 只问「有无更正入口」也会漏掉**状态序列**——`governance_handoff.go` 的暂停与恢复没有任何
//     更正方法，恢复却必须排在暂停之后。
//   - 而把「多条但互不相干」误记成「一对象一意图」虽不改变结论，却会让下一个人照错理由推断。
//
// 标注原先规定「由该行的地盘主人填，不由建清单的人代填」，理由是建清单的人扫得出「同一表达
// 式」却扫不出「同主体会不会出多条」，代填出来的是一个看起来很硬、实则凭印象的数字。
//
// **这条规定作废，因为它假定地盘主人会一直在。** 二十六行「待地盘主人标注」在清单里坐了整整
// 一轮没人动过——会话是易朽的，没有任何人会回来填。改成的口径是：**填的人自己按判据逐行读
// 领域，读得出就填判据，读不出就填「待裁」并写清卡在哪一句。** 原先那条担心的「凭印象的数
// 字」由两件事挡住：判据本身可复核（那个方法还在不在、键里有没有版本维，看一眼就知道），以及
// 待裁这一格给了「我不知道」一个正当出口——没有它，不知道的行只能被填成一个好看的值。
const (
	annotationCorrectionEntry = "更正入口："
	annotationVersionInKey    = "版本进键："
	annotationStateSequence   = "状态序列："
	annotationDependsOnPrior  = "依赖前序："
	annotationNoOrdering      = "无先后："
	annotationPendingRuling   = "待裁："

	annotationNoRewrite = annotationNoOrdering + "各条互不相干"
)

// annotationPrefixes 是标注允许的开头。顺序即上面判据表的顺序，前三个是真风险。
var annotationPrefixes = []string{
	annotationCorrectionEntry,
	annotationVersionInKey,
	annotationStateSequence,
	annotationDependsOnPrior,
	annotationNoOrdering,
	annotationPendingRuling,
}

// allowedSameExpression 是本门禁落地那一刻已经存在的位置，值是上面四类之一的标注。
// **每修一处删一行；删行要与修复同笔提交。**
//
// **清单对新来者是关的**：口径既已定，新写的 handoff 拿不到例外——它答不出自己在等什么。
// 这就是它与一张「先放着」清单的全部差别。
//
// 清单只许变短由 TestNoEnvelopeTakesItsIDAndPartitionKeyFromTheSameExpression 的后半段守着：
// 一处修好了却留在清单里，同样报错。这一段当场抓到过两次——我凭记忆填了两个不违规的文件，
// 以及一处修好后例外行忘了删。
//
// **清单按文件计，缺陷按口计，两者不一一对应。** `operating_handoff.go` 一个文件里坐着重分摊
// 与重派生两个缺陷、共用一个 `shape.eventID`：改一半门禁依旧红，两个分支都改完才能删那一行。
//
// **一句必须留着的话：本清单清空 ≠ 这一类缺陷清完了。** 门禁拦的是「ID 与 PartitionKey 同源」
// 这个句法形状；而分区键取得太细却不同源的口（`eventID` 在业务键上拼出来的那种）**既不进门禁
// 也不进本清单**，它的用例还会全绿。那一类只能靠人拿上面那句判据重扫。
//
// **删例外行要与修复同笔提交，别分两笔。**这份清单归门禁作者所有，但删自己那一行的是修复的
// 人——分两笔的话，两笔之间的 HEAD 是红的，而这一批有四个人在同一棵树上并行改，那段窗口里
// 谁验全仓都会红。已经发生过一次：对账单那处修复与删行分了两笔，中间 HEAD 红了一轮。
var allowedSameExpression = map[string]string{
	// 关务案件链是四个 handoff 各出一封（建立 → 申报提交 → 核对 → 关闭），而分区由键值定、
	// 不由 handoff 定：四口不改成同一个键公式就落不进同一分区，各自改成业务键也白改。这一句
	// 本上下文当前无主，三行因此待裁。verification 与 gate 另有各自独立成立的真风险，不等这一裁。
	"internal/customscompliance/adapters/postgres/case_closure_handoff.go": annotationPendingRuling +
		"案件链是否需保序；且 CloseCustomsCaseHandler.Handle 写明「重开走 Reopen」，重开后再关会撞同一 ID",
	"internal/customscompliance/adapters/postgres/customs_case_handoff.go": annotationPendingRuling +
		"案件链是否需保序——本口是链首，它取什么键公式决定了另外三口得跟着取什么",
	"internal/customscompliance/adapters/postgres/declaration_submission_handoff.go": annotationPendingRuling +
		"案件链是否需保序；ID 缺版本维一事已另有票（.scratch/declaration-envelope-version-dedup/issues/01），今天无触发路径",
	"internal/customscompliance/adapters/postgres/external_result_handoff.go": annotationNoOrdering +
		"同一来源标识只出一份内容——ReceiveExternalResultHandler.Handle 对异内容判冲突，不出第二封",
	"internal/customscompliance/adapters/postgres/follow_up_handoff.go": annotationStateSequence +
		"ManageFollowUpHandler.FormTarget → .RecordEffect，同一目标键先后两拍撞同一 ID",
	"internal/customscompliance/adapters/postgres/manifest_handoff.go": annotationCorrectionEntry +
		"ReceiveManifestHandler.Revise",

	"internal/networkrouting/adapters/postgres/initial_route_handoff.go": annotationNoOrdering +
		"判断键含接受基线，重判走新基线即新键；同键由 CreateInitialRouteHandler.Handle 判重放返原",

	"internal/nodeoperations/adapters/postgres/collaboration_acceptance_handoff.go": annotationNoOrdering +
		"同一事项只决定一次——AcceptCollaborationHandler.Accept 幂等按（租户+事项），异内容答冲突不顶替",
	"internal/nodeoperations/adapters/postgres/execution_fact_handoff.go": annotationNoRewrite +
		"（键取到动作，同一事项的各动作各是一条独立事实；消费侧 CC 的 factSetDigest 先把事实引用排序，装载顺序不构成不同内容）",
	"internal/nodeoperations/adapters/postgres/node_intake_handoff.go": annotationNoOrdering +
		"同一收寄键只出一份——ReceiveDeliveredUnitHandler.Handle 幂等/冲突按内容指纹分界，且只在收寄判断成立时交意图",

	"internal/settlementaccounting/adapters/postgres/advance_recovery_handoff.go": annotationDependsOnPrior +
		"recovery-adjustment 引用 advance-recovery——Adjust 先核对回收在场，且不改写原回收",
	"internal/settlementaccounting/adapters/postgres/charge_confirmation_handoff.go": annotationNoOrdering +
		"一笔费用只确认一次，本上下文没有费用的更正、撤销或重确认入口",
	"internal/settlementaccounting/adapters/postgres/claim_settlement_handoff.go": annotationDependsOnPrior +
		"claim-adjustment 引用 claim-amount / receivable / acknowledgement——Adjust 核对目标在场，且不改写原金额（AT-SA-152）",
	"internal/settlementaccounting/adapters/postgres/operating_handoff.go": annotationCorrectionEntry +
		"AllocateCostsHandler.Reallocate 与 .Rederive——一个文件两个缺陷，共用同一个 shape.eventID",
	"internal/settlementaccounting/adapters/postgres/settlement_application_handoff.go": annotationCorrectionEntry +
		"MapExternalFundsHandler.Reverse",

	"internal/transportfulfillment/adapters/postgres/capacity_consumption_handoff.go": annotationNoRewrite +
		"（一个池有多个预占，各条是互不相干的消耗事实，累加可交换）",
	"internal/transportfulfillment/adapters/postgres/disposition_execution_handoff.go": annotationNoOrdering +
		"同一处置不开两条旅程——StartAlternateJourneyHandler.Handle 幂等按（租户+原旅程+目的+处置依据）",
	"internal/transportfulfillment/adapters/postgres/exception_journey_handoff.go": annotationNoOrdering +
		"与 disposition_execution 同键、同一拍入队，两口靠类型段错开；幂等口径同上",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_handoff.go": annotationNoOrdering +
		"同一揽收尝试键只出一份——PerformOffsitePickupHandler.Handle 幂等/冲突按内容指纹分界",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_registration_handoff.go": annotationPendingRuling +
		"同一载运对象能否出现第二次成功的对象级揽收登记——能则两次是同一条控制链的先后拍，而交接登记那口已按「一个对象一条链」把主体取到对象",
	"internal/transportfulfillment/adapters/postgres/regulatory_acceptance_handoff.go": annotationNoOrdering +
		"同一协作事项只承接一次——AcceptRegulatoryDispositionHandler.Handle 对同键异内容判冒名冲突，不顶替",
	"internal/transportfulfillment/adapters/postgres/transport_commission_handoff.go": annotationNoOrdering +
		"同一委托只发提交一拍——CommissionTransportHandler.CancelCommission 只 Replace 存储，取消不经本口交意图",
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

// TestEveryExceptionCarriesACheckableVerdict 守清单的标注本身。
//
// 例外可以有，但每一行都要带一句**能复核的判据**：六个前缀之一，后面跟具体内容。
//
// 前身只查「非空」，于是二十六行「待地盘主人标注」全部合格地坐了一整轮——它既不是判据也不是
// 结论，只是一句「还没人看」，而没有任何东西会因此变红。查前缀补的就是那一格：想说不知道就得
// 写「待裁：」并把答不出的那一句写出来，而那一句是可以拿去问人的。
func TestEveryExceptionCarriesACheckableVerdict(t *testing.T) {
	for path, verdict := range allowedSameExpression {
		prefix, ok := matchedAnnotationPrefix(verdict)
		if !ok {
			t.Errorf("%s 的标注没有用判据前缀开头（六选一：%s）：%q",
				path, strings.Join(annotationPrefixes, " / "), verdict)
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(verdict, prefix)) == "" {
			t.Errorf("%s 的标注只有前缀 %q，没写具体是哪一处", path, prefix)
		}
	}
}

func matchedAnnotationPrefix(verdict string) (string, bool) {
	for _, prefix := range annotationPrefixes {
		if strings.HasPrefix(verdict, prefix) {
			return prefix, true
		}
	}
	return "", false
}
