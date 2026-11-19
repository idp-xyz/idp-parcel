package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// WarehousePages 是仓列表的查询声明（ADR-0144 决定三、四、七）：可排维与筛选维按答复体的字段名点名，各维落到哪一列、
// q 覆盖哪几列在 postgres 读面里。
//
// 只开一个可排维：列表的行就是各仓，按仓编码排即缺省序，不为「以后可能要」多开换序的维度——每开一维就多一个要索引、
// 要校验的面。行标识也是仓编码，决胜键与排序同向。筛选维只有是否停用：登记表单与下游挑仓时按它收窄，不能只在第一页里挑。
var WarehousePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-warehouses",
	Sorts: []cataloguepage.SortDimension{
		{Name: "warehouseCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "warehouseCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "deactivated", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})

// ZonePages 是库区列表的查询声明（ADR-0144 决定三、四、七），照仓列表的做法：只开一个可排维，行标识是（仓编码，库区编码）两段，
// 缺省序就是先按仓、再按库区。排序维取 warehouseCode 而不取 zoneCode：库区编码在仓内才唯一，单按它排没有稳定的行序，也不是
// 登记方挑库区的顺序。
//
// 筛选维两个：仓编码（不设词表——仓是租户取值，词表只能是空的）与是否停用。仓编码按答复体的字段名点名、逐字比较。
var ZonePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-zones",
	Sorts: []cataloguepage.SortDimension{
		{Name: "warehouseCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "warehouseCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "deactivated", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})
