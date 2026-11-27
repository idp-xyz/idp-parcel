package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// PreAdvicePages 是一个货主名下入库预报列表的查询声明（ADR-0144 决定三、四、七），照商品列表的做法：列表在一个货主之内，
// 货主在读口的签名上，行标识就是预报标识一段，缺省序按预报标识排；只开这一个可排维。q 覆盖预报标识与仓编码。
// 状态是筛选维，词表是预报状态的封闭集。仓编码也是筛选维，不设词表：仓是租户取值，逐字比较。
// 这两维不在读口收窄，翻到下一页就看不见这一页之外的同状态、同仓预报。
var PreAdvicePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-inbound-pre-advices",
	Sorts: []cataloguepage.SortDimension{
		{Name: "preAdviceId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "preAdviceId"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: []string{"ADVISED", "RECEIVING", "CLOSED", "CANCELLED"}},
		{Name: "warehouseCode"},
		// 按现行告知的预计到达收窄。各收一个时刻，起点含、终点含；缺的一端不收窄。
		{Name: "expectedFrom"},
		{Name: "expectedUntil"},
	},
})
