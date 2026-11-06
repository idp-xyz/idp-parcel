package ports

import (
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/02）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec 取舍 1）。

// ShipmentRequestViewPages 是委托查阅列表（GET /shipment-request-views 的列表分支）的声明。
//
// 缺省序是提交时刻倒序，与迁移前同一口径；委托没有另立登记时刻，提交时刻就是它进册的时刻。state 放出作筛选维（决定四），
// 词表取自领域 ShipmentRequestState；customerAccountId 在授权作用域之内再收窄，作用域外的账户照旧答不出行。按单据号、
// 按一组标识与按标识取单份的三条分支是有界查找，不走本声明。
var ShipmentRequestViewPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "shipment-request-views",
	Sorts: []cataloguepage.SortDimension{
		{Name: "submittedAt", Kind: cataloguepage.Instant},
		{Name: "shipmentRequestId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "state", Vocabulary: cataloguepage.Words[domain.ShipmentRequestState]()},
		{Name: "customerAccountId"},
	},
})

// AcceptanceReviewQueuePages 与 AuthorizedDispositionQueuePages 是两个委托队列（GET /acceptance-review-queue、
// GET /authorized-disposition-queue 的列表分支）的声明。
//
// 缺省序是提交时刻正序：队列先来先办，与迁移前同一口径。队列本身就是一个等待态，没有别的状态可放出作筛选维（决定四的
// 「有状态的册」指册内行各处不同状态，这里全都停在同一格）；customerAccountId 在作用域之内收窄。
var (
	AcceptanceReviewQueuePages      = cataloguepage.MustCatalogue(shipmentRequestQueueSpec("acceptance-review-queue"))
	AuthorizedDispositionQueuePages = cataloguepage.MustCatalogue(shipmentRequestQueueSpec("authorized-disposition-queue"))
)

func shipmentRequestQueueSpec(name string) cataloguepage.Spec {
	return cataloguepage.Spec{
		Name: name,
		Sorts: []cataloguepage.SortDimension{
			{Name: "submittedAt", Kind: cataloguepage.Instant},
			{Name: "shipmentRequestId", Kind: cataloguepage.Text},
		},
		DefaultSort: cataloguepage.Sort{Field: "submittedAt"},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
		Filters:     []cataloguepage.FilterDimension{{Name: "customerAccountId"}},
	}
}

// TiedChannelSelectionPages 是渠道择优并列冲突列表（GET /channel-selection-decisions 的列表分支）的声明。
//
// 缺省序是决定时刻倒序：最近一次冲突在最上面，与迁移前同一口径。结论已被这一口钉成并列冲突，没有状态维可放；view、
// scope、mapping 是端点既有的参数，保持原义登为选择器（决定四），取值进游标摘要——换了收窄对象就是另一份列表。
var TiedChannelSelectionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "tied-channel-selection-decisions",
	Sorts: []cataloguepage.SortDimension{
		{Name: "decidedAt", Kind: cataloguepage.Instant},
		{Name: "decisionId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "decidedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"view", "scope", "mapping"},
})

// ControlReleaseRefusalPages 是释放未受理册的未处理行（GET /control-release-refusals）的声明。
//
// 缺省序是登记时刻正序：最早登记的先处理，与迁移前同一口径。行标识是（委托、提交版本、控制结果），即册行的主键去掉租户。
// 「未处理」是册的定义而不是行上的状态，没有状态维；customerAccountId 放出作筛选维。
var ControlReleaseRefusalPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "control-release-refusals",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "customerAccountId"}},
})

// CancellationOrderConflictPages 是取消先后冲突册的未解开行（GET /cancellation-order-conflicts）的声明。
//
// 缺省序是发现时刻正序，与迁移前同一口径。行标识是（包裹、边界种类、边界那一代、取消），即冲突行的主键去掉租户——迁移前的
// 次序漏了边界种类一段，同一件包裹两种边界同刻冲突时次序不确定，补上之后才稳定。boundaryKind 放出作筛选维，词表取自
// 领域 CancellationBoundaryKind。
var CancellationOrderConflictPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "cancellation-order-conflicts",
	Sorts:       []cataloguepage.SortDimension{{Name: "detectedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "detectedAt"},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text,
	},
	Filters: []cataloguepage.FilterDimension{
		{Name: "boundaryKind", Vocabulary: cataloguepage.Words[domain.CancellationBoundaryKind]()},
	},
})

// PostIntakeDispositionRequestPages 是等回应的处置请求（GET /post-intake-disposition-requests）的声明。
//
// 缺省序是记下时刻正序，与迁移前同一口径。行标识是（请求键、包裹）。两路来源（待处置记录、不成立取消回落的取消）都是
// 「等回应」这一格，没有状态维；来源一格是派生的，不作筛选维。
var PostIntakeDispositionRequestPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "post-intake-disposition-requests",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
})

// PostIntakeManualItemPages 是待人工处理项（GET /post-intake-disposition-manual-items 不带 parcel 的那一支）的声明。
//
// 缺省序是所属决定的决定时刻正序，与迁移前同一口径（项写入时取自所属决定，读面落在项上的那一列）。行标识是（包裹、决定
// 版本、所有者、动作），即项的主键去掉租户。owner 与 action 放出作筛选维，词表取自领域 DispositionOwner 与
// DispositionOwnerAction。带 parcel 的那一支按包裹取全部项，是有界查找，不走本声明。
var PostIntakeManualItemPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "post-intake-disposition-manual-items",
	Sorts:       []cataloguepage.SortDimension{{Name: "decidedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "decidedAt"},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Integer, cataloguepage.Text, cataloguepage.Text,
	},
	Filters: []cataloguepage.FilterDimension{
		{Name: "owner", Vocabulary: cataloguepage.Words[domain.DispositionOwner]()},
		{Name: "action", Vocabulary: cataloguepage.Words[domain.DispositionOwnerAction]()},
	},
})

// LabelTransactionPages 是面单交易查阅（GET /label-transactions）的声明。
//
// 本册的行是面单交易，答复再把一笔交易摊成「交易 × 包裹」几行：page 的 size 与 total 数的都是交易，不是摊开后的行。
// 缺省序是建立时刻倒序，与迁移前同一口径；交易没有登记时刻一格，建立时刻就是它进册的时刻。transactionResult 放出作
// 筛选维（决定四：有状态的册至少放出状态一维），词表取自领域 LabelTransactionState。
var LabelTransactionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "label-transactions",
	Sorts: []cataloguepage.SortDimension{
		{Name: "establishedAt", Kind: cataloguepage.Instant},
		{Name: "transactionId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "establishedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "transactionResult", Vocabulary: cataloguepage.Words[domain.LabelTransactionState]()},
	},
})
