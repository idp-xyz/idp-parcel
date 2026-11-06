package ports

import (
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/02）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec 取舍 1）。

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
