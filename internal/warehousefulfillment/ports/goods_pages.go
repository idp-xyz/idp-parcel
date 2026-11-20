package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// GoodsPages 是商品列表的查询声明（ADR-0144 决定三、四、七），照仓列表的做法：列表在一个货主之内，货主在读口的签名上，行标识就是商品编码一段，
// 缺省序按商品编码排；只开这一个可排维。商品没有停用，也没有别的登记方要在第一页之外按它收窄的格，所以没有筛选维；q 覆盖商品编码与名称。
var GoodsPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-goods",
	Sorts: []cataloguepage.SortDimension{
		{Name: "goodsCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "goodsCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
})
