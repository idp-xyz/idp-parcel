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
