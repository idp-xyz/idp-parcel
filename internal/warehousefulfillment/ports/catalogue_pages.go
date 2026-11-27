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

// LocationTypePages 是库位类型列表的查询声明（ADR-0144 决定三、四、七），照仓列表的做法：库位类型按租户登记、各仓共用，行标识就是
// 库位类型编码一段，缺省序按编码排；只开这一个可排维，筛选维只有是否停用——登记表单与下游挑库位类型时按它收窄，不能只在第一页里挑。
var LocationTypePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-location-types",
	Sorts: []cataloguepage.SortDimension{
		{Name: "locationTypeCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "locationTypeCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "deactivated", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})

// PackagingMaterialPages 是包材列表的查询声明（ADR-0144 决定三、四、七），照库位类型列表的做法：包材按租户登记、各仓共用，行标识就是包材编码
// 一段，缺省序按编码排；只开这一个可排维，筛选维只有是否停用——打包选包材时按它收窄，不能只在第一页里挑。
var PackagingMaterialPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-packaging-materials",
	Sorts: []cataloguepage.SortDimension{
		{Name: "packagingMaterialCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "packagingMaterialCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "deactivated", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})

// ZonePages 是库区列表的查询声明（ADR-0144 决定三、四、七），照仓列表的做法：只开一个可排维，行标识是（仓编码，库区编码）两段，
// 缺省序就是先按仓、再按库区。排序维取 warehouseCode 而不取 zoneCode：库区编码在仓内才唯一，单按它排没有稳定的行序，也不是
// 登记方挑库区的顺序。
//
// 筛选维是仓编码（不设词表——仓是租户取值，词表只能是空的）与是否停用。仓编码按答复体的字段名点名、逐字比较。
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

// LocationPages 是库位列表的查询声明（ADR-0144 决定三、四、七），照库区列表的做法：只开一个可排维，行标识是（仓编码，库位编码）两段，
// 缺省序就是先按仓、再按库位。排序维取 warehouseCode 而不取 locationCode：库位编码在仓内才唯一，单按它排没有稳定的行序。
//
// 筛选维是仓编码、库区编码（都不设词表——仓与库区是租户取值，词表只能是空的，逐字比较）与是否停用。库区编码在仓内才唯一，所以按它
// 筛时通常与仓编码一起给。
var LocationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-locations",
	Sorts: []cataloguepage.SortDimension{
		{Name: "warehouseCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "warehouseCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "zoneCode"},
		{Name: "deactivated", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})

// UnclaimedDisposalRulePages 是各仓待认领处置规则当前修订列表的查询声明（UC-WF-005；ADR-0144）。
var UnclaimedDisposalRulePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-unclaimed-disposal-rules",
	Sorts: []cataloguepage.SortDimension{
		{Name: "warehouseCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "warehouseCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
	},
})

// UnclaimedArrivalPages 是运营查阅待认领到货列表的查询声明（UC-WF-005；ADR-0144）。
// 筛选维覆盖仓、到货种类与状态；登记时间上下界（receivedFrom/receivedTo）作册子选择器参与游标摘要。
var UnclaimedArrivalPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-unclaimed-arrivals",
	Sorts: []cataloguepage.SortDimension{
		{Name: "receivedAt", Kind: cataloguepage.Instant},
		{Name: "arrivalId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "receivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "kind", Vocabulary: []string{"INBOUND", "RETURN"}},
		{Name: "status", Vocabulary: []string{"UNCLAIMED", "CLAIMED", "DISPOSED"}},
	},
	Selectors: []string{"receivedFrom", "receivedTo"},
})

// StockMovementPages 是库存读口里流水那一种看法的翻页声明（ADR-0144）。流水只追加、越积越多，是库存读口里唯一不能一次答完的
// 一种；其余几种看法答的是读那一刻的整份账，不翻页。
//
// 只开入账时刻一个可排维，缺省新的在前：查阅流水是从最近的一笔往回看，旧的在前时操作者要翻到末页才看得到刚入的账。
// 行标识是流水标识一段。货主、仓与看法是读口自己消费的键，列作选择器进游标摘要——换了货主或仓，上一份列表的游标就不成立。
// 运营面可以带 q，摘要里有它，换了检索词游标就不成立。sort 仍不收：读口在解码之前就拒收。货主面不翻页，也不收 q，
// 由读口自己指定旧的在前、只取第一页。
var StockMovementPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-stock-movements",
	Sorts: []cataloguepage.SortDimension{
		{Name: "postedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "postedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"ownerAccount", "warehouseCode", "view"},
})

// StockImportPages 是运营查阅建账导入列表的查询声明（UC-WF-006）。筛选维覆盖货主、仓与状态。
var StockImportPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-stock-imports",
	Sorts: []cataloguepage.SortDimension{
		{Name: "submittedAt", Kind: cataloguepage.Instant},
		{Name: "importId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "status", Vocabulary: []string{"PENDING", "POSTED", "REJECTED"}},
	},
})
