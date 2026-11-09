package ports

import (
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/08）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列、q 覆盖哪几列在 postgres 读面里。缺省序保迁移前的次序
// （该票 spec「缺省序保今天的次序」）。
//
// 按登记身份排的几册另放一维登记时刻：ADR-0144 的缺省序是登记时间倒序，这几册为保今天的次序没取它作缺省，操作者要看新登的
// 在前时用 `sort=-<那一维>`。`registry` 是 /settlement-charges、/settlement-statements、/settlement-operating-results 按册分派的
// 选择器，每个取值各是一册；各册的检索参数（主张身份、供应商、账单号、应付单号）保持原义登为选择器，由端点照旧读进各自的
// 检索维结构——它们各有空串即不筛、按账单号检索时供应商比号册上的分配方之类的规则，不是筛选维的「维内为或」。

// CustomerChargePages 是客户费用册（GET /settlement-charges?registry=customer-charge）的声明。
//
// 缺省序是费用标识正序，与迁移前同一口径；另放形成时刻 formedAt。stage 放出作筛选维，词表取自领域 ChargeStage。q 覆盖费用
// 标识、费用项目、评价、结算相对方、来源事实与确认依据——后两格是带包裹号的引用，按包裹号检索靠它们。
var CustomerChargePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customer-charges",
	Sorts: []cataloguepage.SortDimension{
		{Name: "charge", Kind: cataloguepage.Text},
		{Name: "formedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "charge"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "stage", Vocabulary: cataloguepage.Words[domain.ChargeStage]()},
	},
	Selectors: []string{"registry"},
})

// SupplierExpectedCostPages 是供应商预期成本版本册（GET /settlement-charges?registry=supplier-expected-cost）的声明。
//
// 缺省序是发生项正序，与迁移前同一口径：同一（发生项＋费用项目）的版本链排在一起、链内按版本。行标识因此是（费用项目、版本）
// 两段而不只是主键里的版本——版本本身唯一，多一段不改唯一性，只为同一发生项下的各条链不交错。另放登记时刻 recordedAt。
// 版本册没有状态维。q 覆盖版本、发生项、费用项目、协议、供应商与评价。
var SupplierExpectedCostPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "supplier-expected-costs",
	Sorts: []cataloguepage.SortDimension{
		{Name: "occurrence", Kind: cataloguepage.Text},
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "occurrence"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// CustomerStatementPages 是已发布客户对账单册（GET /settlement-statements?registry=customer-statement）的声明。
//
// 缺省序是单号正序，与迁移前同一口径；另放发布时刻 publishedAt。voided 是状态维：单据状态「已发布 → 已作废」（CONTEXT 生命周期
// 「对账单」）在册上就是这一行作废留痕两格的在场，读面在 SQL 里由本行导出；词即答复体里 voided 那个布尔的两种写法。「已替代」
// 不另成一词：替代单用新单号另成一行，本册的行上没有替代关系可读。对账状态与结清状态分别表达（CONTEXT），不并进这一维。q 覆盖
// 单号、结算账户、账期与单上各项争议的争议号——客户报来的常是争议号，争议挂在单上、不另成一册。
var CustomerStatementPages = cataloguepage.MustCatalogue(customerStatementSpec("customer-statements", "registry"))

// DisputedCustomerStatementPages 是挂着未了结争议的对账单（GET /settlement-disputed-statements）的声明：行形、次序、状态维与 q
// 同对账单册（作废之前已开的争议照旧等人处理到终局，队列里也有已作废的单），册名分开——两份列表的游标不通用。张数与 truncated
// 两格的口径见 DisputedCustomerStatements。
var DisputedCustomerStatementPages = cataloguepage.MustCatalogue(customerStatementSpec("disputed-customer-statements"))

func customerStatementSpec(name string, selectors ...string) cataloguepage.Spec {
	return cataloguepage.Spec{
		Name: name,
		Sorts: []cataloguepage.SortDimension{
			{Name: "statementNumber", Kind: cataloguepage.Text},
			{Name: "publishedAt", Kind: cataloguepage.Instant},
		},
		DefaultSort: cataloguepage.Sort{Field: "statementNumber"},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
		Filters:     []cataloguepage.FilterDimension{{Name: "voided", Vocabulary: []string{"true", "false"}}},
		Selectors:   selectors,
	}
}

// SupplierBillReceptionPages 是供应商账单接收册（GET /settlement-statements?registry=supplier-bill-reception）的声明。
//
// 缺省序是收到时刻倒序，与迁移前同一口径：收到时刻是账单到达的业务时刻，比落库时刻更贴近操作者说的「新来的账单」。它原本只
// 在主张文档里，迁移 0056 把它导出成取值函数 settlement_accounting.supplier_bill_received_at，以约束保证非空、可比，再建同形的
// 表达式索引（该票 spec「可排维与行标识的列必须非空」）。同刻的决胜键（主张身份、版本）改为与排序同向倒序。另放
// 账期 period。不放状态维：账单册上没有领域状态，管理台的接收进度（待审核、审核人未配置、有争议待再审、已审完）是页面从读口的
// 几个和派生的呈现词，其中一格还读此刻审核授权册上有没有这一组——把它搬进 SQL 就是在服务端另立一份状态规则，这一类的取舍在
// ADR-0372 草案。q 覆盖主张身份、账单号、供应商、账期与法人。claim、supplier、supplierBillNumber 是登记表单精确检索页外账单的
// 参数，登为选择器（见文件头注）。
var SupplierBillReceptionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "supplier-bill-receptions",
	Sorts: []cataloguepage.SortDimension{
		{Name: "receivedAt", Kind: cataloguepage.Instant},
		{Name: "period", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "receivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Selectors:   []string{"registry", "claim", "supplier", "supplierBillNumber"},
})

// ExternalFundsFactPages 是已采用外部资金事实册（GET /settlement-funds-applications）的声明。
//
// 缺省序是资金事实标识正序，与迁移前同一口径；另放发生时刻 occurredAt。一笔事实只列链头那一版，行标识仍取主键的（事实、版本）
// 两段。kind 放出作筛选维，词表取自领域 FundsFactKind。q 覆盖事实标识与来源登记。
var ExternalFundsFactPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "external-funds-facts",
	Sorts: []cataloguepage.SortDimension{
		{Name: "fact", Kind: cataloguepage.Text},
		{Name: "occurredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "fact"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "kind", Vocabulary: cataloguepage.Words[domain.FundsFactKind]()},
	},
})

// OperatingResultPages 是经营结果快照册（GET /settlement-operating-results?registry=operating-result）的声明。
//
// 缺省序是分析范围正序，同一范围按（账期、基准），与迁移前同一口径；另放落库时刻 recordedAt。basis 放出作筛选维，词表取自
// 领域 OperatingBasis。q 覆盖分析范围、账期与计算版本。
var OperatingResultPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "operating-results",
	Sorts: []cataloguepage.SortDimension{
		{Name: "scope", Kind: cataloguepage.Text},
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "scope"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "basis", Vocabulary: cataloguepage.Words[domain.OperatingBasis]()},
	},
	Selectors: []string{"registry"},
})

// CostAllocationPages 是成本分摊册（GET /settlement-operating-results?registry=cost-allocation）的声明。
//
// 缺省序是分摊标识正序，与迁移前同一口径；另放落库时刻 recordedAt。分摊册没有状态维。q 覆盖分摊标识、来源与规则版本。
var CostAllocationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "cost-allocations",
	Sorts: []cataloguepage.SortDimension{
		{Name: "allocation", Kind: cataloguepage.Text},
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "allocation"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// AuditedPayablePages 是审核应付册（GET /settlement-audited-payables）的声明。
//
// 缺省序是审核时刻倒序，与迁移前同一口径：新形成的应付在前。同刻的决胜键应付标识改为与排序同向倒序。应付单号可缺（记无号
// 原因），不放作可排维（该票 spec「可排维与行标识的列必须非空」）。q 覆盖应付标识、应付单号、主张身份、账单号、供应商与账期。documentNumber、claim、supplier
// 是精确检索的参数，登为选择器（见文件头注）。
var AuditedPayablePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "audited-payables",
	Sorts:       []cataloguepage.SortDimension{{Name: "auditedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "auditedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"documentNumber", "claim", "supplier"},
})
