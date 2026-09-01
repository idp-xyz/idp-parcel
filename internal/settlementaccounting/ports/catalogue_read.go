package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件是结算与核算四张管理台页的伴生列表读端口（ADR-0077，票
// admin-skeleton-closure-batch/04）。读口不与既有写口共接口——扩写侧接口会拆全部写侧
// 测试替身，伴生读端口另立（ADR-0077 Decision 五）。
//
// 上列的是**登记册的照实转写**：目录上列不重建领域对象、不形成判断、不下推任何处置
// 参数。派生只到求和与计数为止，且只在同一族登记册内求（判据同 collectionremittance
// CodSubledgerCatalogueRead：派生是求和不是判断）。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容，续办是登记责任方去登记口登记，不折成未配置）。
//
// **可选值两种词形，分别对应两种缺席**：可选引用用空串——库上每个引用列都带
// btrim(...) <> '' 的非空白门，非空即真值，空串因此只可能是「这一格没有登记」；可选
// 时刻用 *time.Time——零时刻是一个合法时间值，用它兼表「没发生过」会让两态在类型上
// 分不开，而这两态要人做的事相反（未确认要人去确认，确认于零时要人去查数据）。

// ChargeConfirmationBasisEntry 是一笔客户费用已到达的一种确认依据：依据种类、依据
// 引用与登记时刻，照 charge_confirmation_basis 转写。
//
// 「到了哪几种依据」与「这类费用要哪一种」是两张表两个答案，核对是两者的交集——迁移
// 0009 分两张表正是为了让`确认条件已满足`没有第三条成立路径，读面照此分两格上列，
// 不代算交集。
type ChargeConfirmationBasisEntry struct {
	BasisKind  string
	Basis      string
	RecordedAt time.Time
}

// CustomerChargeCatalogueRow 是客户费用册上列的一行：费用身份、费用项目、采用的
// 评价引用、阶段、币种三件组、确认留痕，外加该费用已到达的确认依据与目录对该费用
// 项目要求的依据种类。
//
// 币种三件组（原币、合同结算币、换算依据）整组上列不拆散：CONTEXT 明写三件是一条
// 费用从同一个评价采用来的一组，拆开呈现会让读者以为可以各自停在不同评价上。同币种
// 时 ConversionStep 为空是库上 customer_charge_conversion_present 的正面结果，不是缺
// 数据。
//
// RequiredBasisKind 为空表示该费用项目在确认条件目录里没有行——目录内容属实例半边，
// 没有租户时整张表是空的。它与「配了但依据没到」（RequiredBasisKind 有值而
// ConfirmationBases 里没有那一种）是两个不同的答案，读面分两格摆开，由读的人判，
// 不共用一格。
// 确认时固定的七项（责任法人、结算相对方、收付方向、结算账户、合同或责任依据、主要
// 计费范围、来源事实）随 ADR-0087 决定一入册，读面照册转写。它们在非确认行上一律为空，
// 那是库上 customer_charge_confirmation_facts_coupled 的正面结果——「这一行还没确认」，
// 不是缺数据；票 admin-skeleton-closure-batch/04 当初按「册级缺席即撤栏」撤掉的那几栏
// 因此可以照册加回，缺席从册级降成了行级。
type CustomerChargeCatalogueRow struct {
	Charge               string
	FeeItem              string
	Evaluation           string
	Stage                string
	OriginalCurrency     string
	OriginalMinor        int64
	SettlementCurrency   string
	SettlementMinor      int64
	ConversionStep       string
	ConfirmationBasis    string
	ResponsibleEntity    string
	Counterparty         string
	ChargeDirection      string
	SettlementAccount    string
	ContractBasis        string
	PrimaryChargingScope string
	SourceFact           string
	FormedAt             time.Time
	ConfirmedAt          *time.Time
	RequiredBasisKind    string
	ConfirmationBases    []ChargeConfirmationBasisEntry
}

// SupplierExpectedCostCatalogueRow 是供应商预期成本版本册上列的一行。
//
// 它与客户费用分成两个列表而不是并进一张`费用明细`表：CONTEXT 把「供应商预期成本」
// 与「费用明细」立为两个词条，并明禁「内部预期伪装成供应商主张」——合成一张，
// 两族行就会共用一套栏目，而那套栏目里没有任何东西还在说这一行是内部预期。
//
// 列上没有账单主张、审核应付或付款字段，与 0008 的表面一致：那道分界在表上与在
// 类型上同样是结构性的。PriorVersion 与 CorrectionReason 成对为空表示这是首版——
// 计价纠错换版本、原版本保留，一份成本的历史因此是多行而不是一行被改写。
type SupplierExpectedCostCatalogueRow struct {
	Version             string
	Occurrence          string
	OccurrenceReason    string
	OccurrenceVersion   string
	OccurredAt          time.Time
	FeeItem             string
	PurchaseRuleVersion string
	Agreement           string
	Evaluation          string
	OriginalCurrency    string
	OriginalMinor       int64
	SettlementCurrency  string
	SettlementMinor     int64
	ConversionStep      string
	PriorVersion        string
	CorrectionReason    string
	RecordedAt          time.Time
}

// ChargeCatalogueRead 是费用与计费页的伴生列表读端口：客户费用册与供应商预期成本
// 版本册各上各的，两册不合流（理由见 SupplierExpectedCostCatalogueRow）。
type ChargeCatalogueRead interface {
	ListCustomerCharges(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CustomerChargeCatalogueRow, error)

	ListSupplierExpectedCosts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]SupplierExpectedCostCatalogueRow, error)
}

// StatementDisputeEntry 是针对某张对账单某笔费用提出的一项异议，照 statement_dispute
// 转写。裁定三件（结论、依据、时刻）同在或同缺，库上 statement_dispute_resolution_coupled
// 守着这一条；未裁定时三件皆空，读面不代填`待处理`——那会把「还没人裁」写成一个看起来
// 已经有人处置过的结论。
type StatementDisputeEntry struct {
	Dispute       string
	Charge        string
	DisputedMinor int64
	Reason        string
	OpenedAt      time.Time
	Resolution    string
	ResolutionRef string
	ResolvedAt    *time.Time
}

// SubsequentInclusionEntry 是一笔进入后续账期的纳入关系，照 subsequent_inclusion
// 转写。行上没有金额——金额永远在费用或调整本体上，纳入只拥有关系（CONTEXT
// 「后续对账纳入」词条：`UC-SA-003` 只拥有纳入关系，不创建任何调整）。
// Adjustment 为空是 LATE_CHARGE 的正面形状（迁移 0007：迟到费用不得指名调整），
// 不是漏登。
type SubsequentInclusionEntry struct {
	Inclusion        string
	Kind             string
	OriginalPeriod   string
	SubsequentPeriod string
	Charge           string
	Adjustment       string
	IncludedAt       time.Time
}

// CustomerStatementCatalogueRow 是已发布对账单册上列的一行：单号、结算账户、周期、
// 币种、总额快照、发布与作废留痕，外加挂在本单上的异议与后续账期纳入。
//
// LineCount 与 AdjustmentCount 是费用行与调整行的**行数**，不是金额：明细本身不上列
// （一张单的明细是详情面的内容，不是目录行的内容），但「这张单里有几行」与「一行都
// 没有」得分得开。总额不由行数派生，两者各自照实转写——CONTEXT 要求总额严格等于所含
// 明细之和，那是写口的不变量，读口重算一遍只会在两处各说一套。
//
// VoidBasis 与 VoidedAt 成对为空表示未作废。作废不删行不改总额，替代单用新单号，
// 所以`已作废`是这一行上的留痕而不是它的消失。
type CustomerStatementCatalogueRow struct {
	StatementNumber      string
	Account              string
	Period               string
	Currency             string
	TotalMinor           int64
	LineCount            int64
	AdjustmentCount      int64
	PublishedAt          time.Time
	VoidBasis            string
	VoidedAt             *time.Time
	Disputes             []StatementDisputeEntry
	SubsequentInclusions []SubsequentInclusionEntry
}

// SupplierBillReceptionCatalogueRow 是供应商账单接收册上列的一行：主张身份与版本、
// 供应商、责任法人、账期、币种、主张行数与匹配数，以及该范围的审核授权是否已配置。
//
// 逐行匹配与主张明细不在目录行上（同 CustomerStatementCatalogueRow 那句），只上行数：
// 「有几行未匹配」是详情面的问题，目录先要能分开「这份主张有行」与「一行都没有」。
//
// AuditAuthorityConfigured 照实转写而不是折成`可审核`：授权未配置时审核停在未决，
// 不默认放行也不虚构授权人（UC-SA-004）；把它译成一个动作可用性，就是在读面上替
// 审核步骤做了那个判断。
type SupplierBillReceptionCatalogueRow struct {
	Claim                    string
	ClaimVersion             string
	Supplier                 string
	LegalEntity              string
	Period                   string
	Currency                 string
	LineCount                int64
	MatchCount               int64
	AuditAuthorityConfigured bool
	RecordedAt               time.Time
}

// StatementCatalogueRead 是对账页的伴生列表读端口：客户对账单册（UC-SA-003）与供应商
// 账单接收册（UC-SA-004）各上各的。
type StatementCatalogueRead interface {
	ListCustomerStatements(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CustomerStatementCatalogueRow, error)

	ListSupplierBillReceptions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]SupplierBillReceptionCatalogueRow, error)
}

// FundsMappingEntry 是一条真实收付映射：把某笔外部资金事实指向一个未结项，照
// funds_mapping 转写。目标种类是封闭集（STATEMENT / PAYABLE / CREDIT_NOTE）。
// 映射不是核销——事实接收、金额责任确认、真实到账与核销是不同结果（CONTEXT
// 「真实收付映射」词条），所以映射与核销分两个列上列，不合成一格。
type FundsMappingEntry struct {
	Mapping    string
	TargetKind string
	Target     string
	Basis      string
	MappedAt   time.Time
}

// SettlementApplicationEntry 是一次核销，照 settlement_application 转写。
// AllocationCount 是带方向分配片段的**片段数**；片段明细（目标身份、借贷方向、
// 带方向金额）属详情面。ReversalBasis 与 ReversedAt 成对为空表示未撤销——撤销形成
// 可追溯的反向关系，不删除原核销历史。
type SettlementApplicationEntry struct {
	Application     string
	AppliedMinor    int64
	Basis           string
	AppliedAt       time.Time
	AllocationCount int64
	ReversalBasis   string
	ReversedAt      *time.Time
}

// ExternalFundsFactCatalogueRow 是已采用外部资金事实册上列的一行：事实身份、来源
// 登记引用、事实种类、金额、业务发生时刻与更正留痕，外加挂在本事实上的映射与核销。
//
// **AppliedMinor 与 ReversedMinor 是两笔各算各的和，不是一个净额。** 前者只加未撤销
// 的核销，后者只加已撤销的；UnappliedMinor = 事实金额 − AppliedMinor。三个数摆开
// 是因为「从未核销过」与「核销过又撤销了」在一个净额上长着同一张脸，而这两态的续办
// 相反：前者要人去分配，后者要人去看当初为什么撤。撤销不删历史，所以那一截金额必须
// 在某处仍然看得见。
//
// 事实种类照实转写不译成收付方向：kind 的封闭集是 RECEIPT_CONFIRMED / PAYMENT_FAILED
// / FUNDS_RETURNED，三格里只有一格是「收到了钱」，折成收/付两向会把「付款失败」与
// 「资金退回」压成同一格。
type ExternalFundsFactCatalogueRow struct {
	Fact             string
	Source           string
	Kind             string
	Currency         string
	AmountMinor      int64
	Version          string
	OccurredAt       time.Time
	Corrects         string
	CorrectedAt      *time.Time
	AppliedMinor     int64
	ReversedMinor    int64
	UnappliedMinor   int64
	ApplicationCount int64
	Mappings         []FundsMappingEntry
	Applications     []SettlementApplicationEntry
}

// FundsApplicationCatalogueRead 是收付款核销页的伴生列表读端口。行对象是已采用的
// 外部资金事实，映射与核销挂在它下面——页的行对象就是「已确认外部收付款及其分配
// 关系」，不是对账单（收付款事实与核销关系同对账单分别管理）。
type FundsApplicationCatalogueRead interface {
	ListExternalFundsFacts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ExternalFundsFactCatalogueRow, error)
}

// OperatingComponentEntry 是经营结果的一个组成项：来源金额身份、对指标的封闭二向
// （INCREASES / DECREASES）与金额。
//
// 组成逐项上列而不只给毛利：CONTEXT 硬要求审核应付与供应商费用贷项按各自借贷方向
// 分别计入一次，「当前有效审核应付」不得被解释为已经静默净含贷项——净额把这条要求
// 抹掉之后，页面上再也看不出它有没有被遵守。
// Role 照册上原样转写（ADR-0087 决定三补的角色维）。读面不按 Source 猜某一项是客户侧
// 还是审核应付还是贷项——那正是票 admin-skeleton-closure-batch/04 撤掉经营页四栏的理由：
// 由读面代贴标签，等于把那条硬要求重新藏起来。现在角色在册上，转写它就是如实。
//
// 也不在这里按角色合计成四栏：合计是页面按角色分组就能做的事，读面多做一层就多一处
// 定义，而两处一旦不一致，页面上看到的是没人验过的那个数（同 MarginMinor 那条理由）。
type OperatingComponentEntry struct {
	Source      string
	Role        string
	Effect      string
	AmountMinor int64
}

// OperatingResultCatalogueRow 是经营结果快照册上列的一行：口径三件（分析范围、账期、
// 基准）、币种、组成、毛利、计算版本与截至时点。
//
// MarginMinor 照库上那一列转写，读面不重算：毛利只能派生不能直接修改，而派生它的门
// 在写口（重建时复验组成与毛利是否相符）。读口再算一遍就成了第二处定义，且两处一旦
// 不一致，页面上看到的会是读口那个没人验过的数。负毛利即经营损失，不另立一列——同一
// 个数按正负分两列会让「零」落进两列都不占的缝里。
type OperatingResultCatalogueRow struct {
	Scope       string
	Period      string
	Basis       string
	Currency    string
	MarginMinor int64
	Version     string
	AsOf        time.Time
	Corrects    string
	RecordedAt  time.Time
	Components  []OperatingComponentEntry
}

// AllocationPortionEntry 是一次成本分摊里归因到某个分析对象的一份份额。
type AllocationPortionEntry struct {
	Target      string
	AmountMinor int64
}

// CostAllocationCatalogueRow 是成本分摊册上列的一行：来源金额与身份、采用的规则版本、
// 份额、未分摊余额、版本与纠错回指。
//
// UnallocatedMinor 照实转写：已分摊与未分摊之和严格等于来源金额是写口的不变量，未分摊
// 余额是第一类结果不是尾差——没有合格对象或分母为零时来源金额整笔留在这里等新依据，
// 读面把它显式摆出来，不折进份额里凑平。
// Portions 为空数组即「全额未分摊」，库上 portions 允许空数组正是为这一格。
type CostAllocationCatalogueRow struct {
	Allocation       string
	Source           string
	SourceMinor      int64
	Currency         string
	Rule             string
	UnallocatedMinor int64
	Version          string
	AllocatedAt      time.Time
	Corrects         string
	RecordedAt       time.Time
	Portions         []AllocationPortionEntry
}

// OperatingCatalogueRead 是经营核算页的伴生列表读端口：经营结果快照册与成本分摊册。
// 两册同属 UC-SA-006（分摊与指标由同一个用例形成、经同一个 OperatingIntent 交下游），
// 因此同页上列；分摊只改变经营归因，不转移原债权债务责任，故它不出现在费用页。
type OperatingCatalogueRead interface {
	ListOperatingResults(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]OperatingResultCatalogueRow, error)

	ListCostAllocations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CostAllocationCatalogueRow, error)
}
