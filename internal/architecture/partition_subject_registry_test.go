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

// 分区主体登记门禁（partition-key-space-collision 票 02）。
//
// 分区由键值字符串决定，不由上下文、事件类型或 handoff 决定——两个上下文各自算出同一个
// 字符串，信封就进同一分区并互相排队（票 01 实测过一次：TF 揽收登记口并进（租户+对象）后，
// 一封停在未决的揽收信封把同一包裹已经派生的 VE 投影堵在分区头）。而「全仓分区键表达求值后
// 取交集」那条扫描做不出来：键是运行期字符串拼接，静态无值可求；退成类型层也不成立——那处
// 碰撞正是两个不同类型承载同一字符串。论证原文见票 01「要答的」第三问，此处不复述。
//
// 所以不建扫描，建登记处：每个交接口在下面的中心表里声明自己的分区主体。「载运对象与包裹
// 在排队意义上是不是一个主体」这类判断从散落在几十个 PartitionKey 表达式里，挪到一张人能
// 一眼扫完的表上——表存在本身就是为了让人能拍这类问题（ADR-0074 决定五指定本表为主体名的
// 登记去处）。
//
// 门禁只查两件：
//
//	一、覆盖完整且双向——构造带 PartitionKey 的 eventing.Envelope 的每个非测试文件都要有
//	    登记行，新写的口不登记就红；登记行指向的文件不再构造这种信封也红（基线不许烂，
//	    与 production_wiring_ratchet 同一理由）。
//	二、同主体名跨上下文不得静默——两个不同上下文声明了同一个主体名时，同名的每一行都
//	    必须带「｜裁：」段，内容是裁定引用（如 ADR-0074）或一句显式待裁。门禁挡的是静默
//	    共用，不是挡未决——未决必须可见，可见的未决正是这张表要产出的东西。
//
// 它守不住什么——这一节不许删（票 02 原文要求）：
//
// 它守不住「两个主体名其实是同一个字符串」。票 01 那处碰撞的两口若各自诚实声明为
// 「租户/载运对象」与「租户/包裹」，本门禁放行——两个主体名不同，而它们的值在当时的代码里
// 是同一个字符串（cmd/parcel-dispatch 的揽收采用链里，同一个原始串既构造
// tfdomain.NewCarriedObjectReference 也构造 psdomain.NewDeclaredParcelID）。
//
// 本门禁因此不解票 01。它做的是另一件事：把排队主体判断挪到这张表上。把这一节删掉或改软，
// 下一个人会把本门禁当成已经守住了票 01，那比没有这张表更坏。
//
// 其余守不住的，如实列：
//   - 名字是文本，取名归登记人。两个真同主体取了两个名，同名检查不响；两个异主体恰好取了
//     同名，多要一次裁段。表的价值在于让取名本身可见可审，不在于替人取对。
//   - 声明与键表达式是否一致只有一格机械核（同表达式必为逐信封，见下），其余靠人对着键
//     表达式复核；改键的人正路过登记行，声明与键同笔改。
//   - 逐信封行免于同名检查：分区即信封 ID（或含其全部维），无共享队列可言；两个口的 ID
//     字符串意外同值仍属上面第一格。
//
// 登记行取值的三种前缀（与 envelope 门禁的判据前缀同一纪律：写可复核的判据，不写结论）：
//
//	主体：<主体名>[｜裁：<裁定引用或显式待裁句>]   有排队主体的口
//	逐信封：<键取什么>                             分区即信封 ID（或含其全部维），无排队主体
//	待裁：<答不出的那一句>                          主体本身判不准，等人裁
//
// ID 与分区键同表达式的那些口（envelope 门禁 allowedSameExpression 清单）分区空间就是信封
// ID 空间，声明必须是「逐信封」，由用例机械核——为何容许 ID 即分区，判据在那份清单的对应
// 行，此处不复制。逐信封不限于那些口：parcel-pricing 评价口键带租户段、与 ID 不同表达式，
// 粒度仍是一评价一区。
const (
	partitionSubjectPrefix     = "主体："
	partitionPerEnvelopePrefix = "逐信封："
	partitionPendingPrefix     = "待裁："

	// partitionRulingMark 是主体行内裁段的定界符。跨上下文同名的每一行都必须带它。
	partitionRulingMark = "｜裁："
)

var partitionDeclarationPrefixes = []string{
	partitionSubjectPrefix,
	partitionPerEnvelopePrefix,
	partitionPendingPrefix,
}

// declaredPartitionSubjects 是中心登记表：键为交接口文件路径（与 allowedSameExpression
// 同口径），值为该口的分区主体声明。首版四十六行按 7ac2f30 的键表达式如实转录；TF 三口
// 与 VE 四口的主体名以 ADR-0074 决定五为权威，其余口按键表达式取领域原词，判不准的写待裁。
//
// 新写的交接口与登记行同笔提交；剪掉一口同笔删行。
var declaredPartitionSubjects = map[string]string{
	// —— customs-compliance ——
	"internal/customscompliance/adapters/postgres/case_closure_handoff.go":           partitionSubjectPrefix + "租户/关务案件",
	"internal/customscompliance/adapters/postgres/customs_case_handoff.go":           partitionPerEnvelopePrefix + "键即信封 ID（建案幂等键）",
	"internal/customscompliance/adapters/postgres/declaration_submission_handoff.go": partitionPerEnvelopePrefix + "键即信封 ID（申报提交认领键）",
	"internal/customscompliance/adapters/postgres/external_result_handoff.go":        partitionPerEnvelopePrefix + "键即信封 ID（外部结果来源标识）",
	"internal/customscompliance/adapters/postgres/follow_up_handoff.go":              partitionSubjectPrefix + "租户/后续动作目标键（触发/版本/种类）",
	"internal/customscompliance/adapters/postgres/gate_verification_handoff.go":      partitionSubjectPrefix + "租户/门禁核对键（范围/动作/边界）",
	"internal/customscompliance/adapters/postgres/manifest_handoff.go":               partitionSubjectPrefix + "租户/舱单",
	"internal/customscompliance/adapters/postgres/restriction_handoff.go":            partitionSubjectPrefix + "租户/范围（限制决定范围）",
	"internal/customscompliance/adapters/postgres/verification_handoff.go":           partitionSubjectPrefix + "租户/处置执行决定",

	// —— network-routing ——
	"internal/networkrouting/adapters/postgres/initial_route_handoff.go": partitionPerEnvelopePrefix + "键即信封 ID（判断键含接受基线）",
	"internal/networkrouting/adapters/postgres/reachability_handoff.go":  partitionSubjectPrefix + "租户/请求关联",

	// —— node-operations ——
	"internal/nodeoperations/adapters/postgres/collaboration_acceptance_handoff.go": partitionPerEnvelopePrefix + "键即信封 ID（协作事项决定）",
	"internal/nodeoperations/adapters/postgres/execution_fact_handoff.go":           partitionPerEnvelopePrefix + "键即信封 ID（键取到动作，各动作独立成条）",
	"internal/nodeoperations/adapters/postgres/node_intake_handoff.go":              partitionPerEnvelopePrefix + "键即信封 ID（收寄键）",
	"internal/nodeoperations/adapters/postgres/sealed_snapshot_handoff.go":          partitionSubjectPrefix + "租户/集运单元",

	// —— parcel-pricing ——
	"internal/parcelpricing/adapters/postgres/evaluation_handoff.go": partitionPerEnvelopePrefix + "租户段+评价标识（与 ID 不同表达式，粒度仍一评价一区）",

	// —— parcel-shipment ——
	// 采用链三口与 VE 四口同名同值：租户段之后拼的都是同一个包裹身份字符串，FanOut 之下
	// 两链信封落同一分区（票 01 Comments 二实测）。ADR-0074 只裁了 TF×VE，这一对没有裁定
	// 记录，按门禁第二查的口径显式带待裁句，不静默。
	"internal/parcelshipment/adapters/postgres/acceptance_decision_handoff.go": partitionSubjectPrefix + "租户/委托请求",
	"internal/parcelshipment/adapters/postgres/final_outcome_handoff.go":       partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "待裁——PS 采用链与 VE 四口共用包裹分区是否有意，无裁定记录（ADR-0074 只裁 TF×VE）",
	"internal/parcelshipment/adapters/postgres/network_intake_handoff.go":      partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "待裁——PS 采用链与 VE 四口共用包裹分区是否有意，无裁定记录（ADR-0074 只裁 TF×VE）",
	"internal/parcelshipment/adapters/postgres/parcel_cancellation_handoff.go": partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "待裁——PS 采用链与 VE 四口共用包裹分区是否有意，无裁定记录（ADR-0074 只裁 TF×VE）",
	// 「委托已提交」口按（租户/客户账户/委托）排队——同一委托的提交侧事件一条队，不同
	// 委托互不阻塞。与上面接受决定口的「租户/委托请求」是同一聚合的两个口、两个分区
	// 字符串：本表的可扫性正是为让这类取舍摆在明面（要不要并队归裁定，不归本行）。
	"internal/parcelshipment/adapters/postgres/shipment_request_submitted_handoff.go": partitionSubjectPrefix + "租户/客户账户/委托",
	// 「复核已完成」与上一行同主体是有意的，不是漏改的重复行：续办信封必须与同一份委托的
	// 提交信封排同一条队，否则续办可能越过一封还没投出去的提交（ADR-0086 Decision 二）。
	// 两行同名同上下文，跨上下文那道裁段不适用。
	"internal/parcelshipment/adapters/postgres/manual_review_completed_handoff.go": partitionSubjectPrefix + "租户/客户账户/委托",
	"internal/parcelshipment/adapters/postgres/source_data_handoff.go":             partitionSubjectPrefix + "租户/来源请求键",

	// —— party-commercial ——
	// 「参数已登记」（ADR-0094 决定四的续办触发）按租户排队：消费门对该租户的重驱一轮接一轮，
	// 不同租户互不阻塞；ID 带承载版本维管幂等，与分区键不同源。主体名只到租户，仓内其余口没有
	// 同名主体，跨上下文那道裁段不适用。
	"internal/partycommercial/adapters/postgres/operator_registration_completed_handoff.go": partitionSubjectPrefix + "租户（运营登记续办）",

	// —— pilot-governance ——
	// 治理两形的键今天都不带租户段，如实转录；要不要补租户维归 PG 地盘，不在本表定。
	"internal/pilotgovernance/adapters/postgres/governance_handoff.go": partitionSubjectPrefix + "暂停标识（暂停与恢复同区）与接管三维（对象范围/能力/事实种类），两形皆无租户段",

	// —— settlement-accounting ——
	"internal/settlementaccounting/adapters/postgres/advance_recovery_handoff.go":       partitionPerEnvelopePrefix + "键即信封 ID（回收与其调整各自认领）",
	"internal/settlementaccounting/adapters/postgres/charge_confirmation_handoff.go":    partitionPerEnvelopePrefix + "键即信封 ID（费用确认一拍）",
	"internal/settlementaccounting/adapters/postgres/claim_settlement_handoff.go":       partitionPerEnvelopePrefix + "键即信封 ID（索赔结算各拍各自认领）",
	"internal/settlementaccounting/adapters/postgres/operating_handoff.go":              partitionSubjectPrefix + "租户/分摊（/allocation/ 段）与租户/经营结果键（范围/账期/口径，/operating-result/ 段）",
	"internal/settlementaccounting/adapters/postgres/settlement_application_handoff.go": partitionSubjectPrefix + "租户/核销申请（/application/ 段）",
	"internal/settlementaccounting/adapters/postgres/statement_handoff.go":              partitionSubjectPrefix + "租户/对账单（发布与作废同区，/statement/ 段）",
	"internal/settlementaccounting/adapters/postgres/supplier_bill_handoff.go":          partitionSubjectPrefix + "租户/账单主张（/bill/ 段）",

	// —— transport-fulfillment ——
	"internal/transportfulfillment/adapters/postgres/capacity_consumption_handoff.go":            partitionPerEnvelopePrefix + "键即信封 ID（各消耗事实互不相干）",
	"internal/transportfulfillment/adapters/postgres/disposition_execution_handoff.go":           partitionPerEnvelopePrefix + "键即信封 ID（处置执行）",
	"internal/transportfulfillment/adapters/postgres/effective_delivery_handoff.go":              partitionSubjectPrefix + "租户/载运对象/口名" + partitionRulingMark + "ADR-0074 决定二、五（本口段 /effective-delivery）",
	"internal/transportfulfillment/adapters/postgres/exception_journey_handoff.go":               partitionPerEnvelopePrefix + "键即信封 ID（与处置执行同键，靠类型段错开）",
	"internal/transportfulfillment/adapters/postgres/external_tracking_fact_handoff.go":          partitionSubjectPrefix + "租户/载运对象/口名" + partitionRulingMark + "ADR-0074 决定二、五（本口段 /external-carrier-tracking）",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_handoff.go":                  partitionPerEnvelopePrefix + "键即信封 ID（揽收尝试）",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_registration_handoff.go":     partitionSubjectPrefix + "租户/载运对象/口名" + partitionRulingMark + "ADR-0074 决定二、五（本口段 /offsite-pickup-registration）",
	"internal/transportfulfillment/adapters/postgres/regulatory_acceptance_handoff.go":           partitionPerEnvelopePrefix + "键即信封 ID（监管处置承接）",
	"internal/transportfulfillment/adapters/postgres/transport_commission_handoff.go":            partitionPerEnvelopePrefix + "键即信封 ID（运输委托提交一拍）",
	"internal/transportfulfillment/adapters/postgres/transport_handover_registration_handoff.go": partitionSubjectPrefix + "租户/载运对象/口名" + partitionRulingMark + "ADR-0074 决定二、五（本口段 /transport-handover-registration）",

	// —— visibility-exception ——
	"internal/visibilityexception/adapters/postgres/customer_view_handoff.go":  partitionSubjectPrefix + "租户/客户",
	"internal/visibilityexception/adapters/postgres/disposition_handoff.go":    partitionSubjectPrefix + "租户/异常案件",
	"internal/visibilityexception/adapters/postgres/eta_handoff.go":            partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "ADR-0074 决定四、五",
	"internal/visibilityexception/adapters/postgres/liability_handoff.go":      partitionSubjectPrefix + "租户/索赔批次",
	"internal/visibilityexception/adapters/postgres/notification_handoff.go":   partitionSubjectPrefix + "租户/客户",
	"internal/visibilityexception/adapters/postgres/projection_handoff.go":     partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "ADR-0074 决定四、五",
	"internal/visibilityexception/adapters/postgres/triage_handoff.go":         partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "ADR-0074 决定四、五",
	"internal/visibilityexception/adapters/postgres/visibility_gap_handoff.go": partitionSubjectPrefix + "租户/包裹" + partitionRulingMark + "ADR-0074 决定四、五",
}

// fileDeclaresPartition 判断一份语法树里有没有构造带 PartitionKey 字段的 eventing.Envelope。
// 复用 envelope 门禁的 envelopeType 与 envelopeField，只读不改那道门禁的规则——本门禁是
// 并列的第二条，不是它的扩展。
func fileDeclaresPartition(syntax *ast.File) bool {
	declares := false
	ast.Inspect(syntax, func(node ast.Node) bool {
		if declares {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok || literal.Type == nil || types.ExprString(literal.Type) != envelopeType {
			return true
		}
		if _, has := envelopeField(literal, "PartitionKey"); has {
			declares = true
			return false
		}
		return true
	})
	return declares
}

// declaredSubjectName 取主体行裁段之前的主体名。非主体行交回 false。
func declaredSubjectName(declaration string) (string, bool) {
	if !strings.HasPrefix(declaration, partitionSubjectPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(declaration, partitionSubjectPrefix)
	if cut := strings.Index(name, partitionRulingMark); cut >= 0 {
		name = name[:cut]
	}
	return strings.TrimSpace(name), true
}

// declaringContext 取路径所属的限界上下文段（internal/<上下文>/…）。
func declaringContext(path string) string {
	segments := strings.Split(path, "/")
	if len(segments) >= 2 && segments[0] == "internal" {
		return segments[1]
	}
	return segments[0]
}

// carriesRuling 判断主体行是否带非空裁段。
func carriesRuling(declaration string) bool {
	cut := strings.Index(declaration, partitionRulingMark)
	return cut >= 0 && strings.TrimSpace(declaration[cut+len(partitionRulingMark):]) != ""
}

// silentlySharedSubjectRows 找出「主体名有第二个上下文也在声明，而本行没带裁段」的行。
// 逐信封与待裁行不参加：前者无排队主体（免检理由见文件头），后者主体本身未定、无名可比。
func silentlySharedSubjectRows(registry map[string]string) []string {
	contextsByName := make(map[string]map[string]bool)
	for path, declaration := range registry {
		name, ok := declaredSubjectName(declaration)
		if !ok {
			continue
		}
		if contextsByName[name] == nil {
			contextsByName[name] = make(map[string]bool)
		}
		contextsByName[name][declaringContext(path)] = true
	}

	var offending []string
	for path, declaration := range registry {
		name, ok := declaredSubjectName(declaration)
		if !ok || len(contextsByName[name]) < 2 || carriesRuling(declaration) {
			continue
		}
		offending = append(offending, path)
	}
	sort.Strings(offending)
	return offending
}

func matchedPartitionPrefix(declaration string) (string, bool) {
	for _, prefix := range partitionDeclarationPrefixes {
		if strings.HasPrefix(declaration, prefix) {
			return prefix, true
		}
	}
	return "", false
}

// TestEveryPartitionEmitterDeclaresItsSubject 守覆盖：表与现实双向一致，且每行带可复核前缀。
func TestEveryPartitionEmitterDeclaresItsSubject(t *testing.T) {
	emitting := make(map[string]bool)
	for _, source := range parseRepositorySources(t) {
		if fileDeclaresPartition(source.syntax) {
			emitting[source.path] = true
		}
	}
	if len(emitting) == 0 {
		t.Fatal("全仓找不到任何构造带 PartitionKey 信封的文件；本门禁会永远空跑")
	}

	var missing []string
	for path := range emitting {
		if _, declared := declaredPartitionSubjects[path]; !declared {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	for _, path := range missing {
		t.Errorf("%s 构造带 PartitionKey 的信封，而它不在 declaredPartitionSubjects 里。"+
			"声明它的分区主体（主体：／逐信封：／待裁：三选一），与该口同笔提交", path)
	}

	var stale []string
	for path := range declaredPartitionSubjects {
		if !emitting[path] {
			stale = append(stale, path)
		}
	}
	sort.Strings(stale)
	for _, path := range stale {
		t.Errorf("登记表里的 %s 已不构造带 PartitionKey 的信封（被删、改名搬家或不再发信封）。"+
			"剪掉这一行——留着它，表的地面真相就漂走了", path)
	}

	paths := make([]string, 0, len(declaredPartitionSubjects))
	for path := range declaredPartitionSubjects {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		declaration := declaredPartitionSubjects[path]
		prefix, ok := matchedPartitionPrefix(declaration)
		if !ok {
			t.Errorf("%s 的声明没有用登记前缀开头（三选一：%s）：%q",
				path, strings.Join(partitionDeclarationPrefixes, " / "), declaration)
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(declaration, prefix)) == "" {
			t.Errorf("%s 的声明只有前缀 %q，没写内容", path, prefix)
		}
	}

	// ID 与分区键同表达式 ⇒ 分区空间即信封 ID 空间 ⇒ 声明必须是逐信封。只读一致性核，
	// 不改 allowedSameExpression 的规则；缺行由上面的覆盖检查报，此处跳过。
	for path := range allowedSameExpression {
		declaration, declared := declaredPartitionSubjects[path]
		if !declared {
			continue
		}
		if !strings.HasPrefix(declaration, partitionPerEnvelopePrefix) {
			t.Errorf("%s 在 allowedSameExpression 里（ID 与分区键同表达式），分区空间就是"+
				"信封 ID 空间，声明必须以 %q 开头，而不是 %q",
				path, partitionPerEnvelopePrefix, declaration)
		}
	}
}

// TestNoPartitionSubjectNameIsSharedAcrossContextsSilently 守第二查：跨上下文同名必须带裁段。
func TestNoPartitionSubjectNameIsSharedAcrossContextsSilently(t *testing.T) {
	for _, path := range silentlySharedSubjectRows(declaredPartitionSubjects) {
		declaration := declaredPartitionSubjects[path]
		name, _ := declaredSubjectName(declaration)
		t.Errorf("%s 声明的主体名 %q 有第二个上下文也在声明，而本行没带 %q 段。"+
			"补一句裁定引用（如 ADR-0074），或一句显式待裁——不允许静默共用",
			path, name, partitionRulingMark)
	}
}

// TestThePartitionSubjectRegistryGateCanActuallyCatchAViolation 证这道门禁真能红。
//
// 与本包其余门禁同一约定：一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信。
func TestThePartitionSubjectRegistryGateCanActuallyCatchAViolation(t *testing.T) {
	parse := func(t *testing.T, source string) *ast.File {
		t.Helper()
		syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", source, 0)
		if err != nil {
			t.Fatalf("解析合成源码：%v", err)
		}
		return syntax
	}

	extraction := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "构造带 PartitionKey 的信封被认出",
			source: "package p\n\nfunc f() { _ = eventing.Envelope{ID: a, PartitionKey: b} }\n",
			want:   true,
		},
		{
			name:   "不带 PartitionKey 字段的信封构造不算——它没声明分区",
			source: "package p\n\nfunc f() { _ = eventing.Envelope{ID: a} }\n",
			want:   false,
		},
		{
			name:   "别的类型带同名字段不算",
			source: "package p\n\nfunc f() { _ = other.Thing{ID: a, PartitionKey: b} }\n",
			want:   false,
		},
	}
	for _, test := range extraction {
		t.Run(test.name, func(t *testing.T) {
			if got := fileDeclaresPartition(parse(t, test.source)); got != test.want {
				t.Fatalf("fileDeclaresPartition = %v, want %v", got, test.want)
			}
		})
	}

	sharing := []struct {
		name     string
		registry map[string]string
		want     []string
	}{
		{
			name: "跨上下文同名、其中一行没带裁段——报没带的那一行",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionSubjectPrefix + "租户/案件",
				"internal/b/adapters/postgres/y.go": partitionSubjectPrefix + "租户/案件" + partitionRulingMark + "ADR-9999",
			},
			want: []string{"internal/a/adapters/postgres/x.go"},
		},
		{
			name: "跨上下文同名、两行都带裁段——放行",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionSubjectPrefix + "租户/案件" + partitionRulingMark + "ADR-9999",
				"internal/b/adapters/postgres/y.go": partitionSubjectPrefix + "租户/案件" + partitionRulingMark + "待裁——共队是否有意未裁",
			},
			want: nil,
		},
		{
			name: "同一上下文内同名——不是跨上下文，放行",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionSubjectPrefix + "租户/客户",
				"internal/a/adapters/postgres/y.go": partitionSubjectPrefix + "租户/客户",
			},
			want: nil,
		},
		{
			name: "裁段定界符在而内容为空——等于没带，照报",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionSubjectPrefix + "租户/案件" + partitionRulingMark,
				"internal/b/adapters/postgres/y.go": partitionSubjectPrefix + "租户/案件" + partitionRulingMark + "ADR-9999",
			},
			want: []string{"internal/a/adapters/postgres/x.go"},
		},
		{
			name: "逐信封行跨上下文同文案——无排队主体，不参加同名检查",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionPerEnvelopePrefix + "键即信封 ID",
				"internal/b/adapters/postgres/y.go": partitionPerEnvelopePrefix + "键即信封 ID",
			},
			want: nil,
		},
		{
			name: "待裁行跨上下文同文案——主体未定无名可比，不参加",
			registry: map[string]string{
				"internal/a/adapters/postgres/x.go": partitionPendingPrefix + "主体判不准",
				"internal/b/adapters/postgres/y.go": partitionPendingPrefix + "主体判不准",
			},
			want: nil,
		},
	}
	for _, test := range sharing {
		t.Run(test.name, func(t *testing.T) {
			got := silentlySharedSubjectRows(test.registry)
			if len(got) != len(test.want) {
				t.Fatalf("报出 %d 行，want %d 行：%v", len(got), len(test.want), got)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("第 %d 行 = %s，want %s", index, got[index], test.want[index])
				}
			}
		})
	}
}
