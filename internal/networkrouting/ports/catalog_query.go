package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// CatalogPage 是目录上列读回的一页（ADR-0144 决定六）：本页行、下一页游标（空串即已到末页），以及与本页
// 同一组条件下的总数。
type CatalogPage[Row any] struct {
	Rows  []Row
	Next  string
	Total int64
}

// 各族的查询声明（ADR-0144 决定三、四、七）。可排维、筛选维按答复体的 JSON 字段名点名；family 是册子选择器，
// 保持原义、不兼作筛选维，各族各自声明。
//
// 缺省序：七张版本表都没有登记时间一格，ADR-0144 的缺省序 -registeredAt 无从取；网络目录是按对象看版本的册，
// 缺省序于是取身份升序——各族取 code，服务日历取 targetKind（其身份以对象种类打头）。决胜键是行标识、与排序
// 同向（决定三），所以同一对象内的版本按版本号升序排，不再是此前的「最新版在前」。缺省序与各表主键同形，
// 登记表不另补索引（决定七要的那条就是主键）。缺省序是否顺手归 NR owner 复核（ADR-0144 越权风险点 1）。
var (
	NodeVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name:        "network-catalog/node",
		Sorts:       []cataloguepage.SortDimension{byCode, byEffectiveFrom},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters:     []cataloguepage.FilterDimension{{Name: "code"}},
		Selectors:   familySelector,
	})
	ConnectionVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: "network-catalog/connection",
		Sorts: []cataloguepage.SortDimension{
			byCode,
			{Name: "fromNode", Kind: cataloguepage.Text},
			{Name: "toNode", Kind: cataloguepage.Text},
			byEffectiveFrom,
		},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters:     []cataloguepage.FilterDimension{{Name: "code"}, {Name: "fromNode"}, {Name: "toNode"}},
		Selectors:   familySelector,
	})
	LineVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name:        "network-catalog/line",
		Sorts:       []cataloguepage.SortDimension{byCode, byEffectiveFrom},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters:     []cataloguepage.FilterDimension{{Name: "code"}},
		Selectors:   familySelector,
	})
	ServiceAreaVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name:        "network-catalog/service-area",
		Sorts:       []cataloguepage.SortDimension{byCode, byEffectiveFrom},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters:     []cataloguepage.FilterDimension{{Name: "code"}},
		Selectors:   familySelector,
	})
	ServiceCalendarVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: "network-catalog/service-calendar",
		Sorts: []cataloguepage.SortDimension{
			{Name: "targetKind", Kind: cataloguepage.Text},
			{Name: "targetCode", Kind: cataloguepage.Text},
			byEffectiveFrom,
		},
		DefaultSort: cataloguepage.Sort{Field: "targetKind"},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Integer},
		Filters: []cataloguepage.FilterDimension{
			{Name: "targetKind", Vocabulary: targetKindVocabulary},
			{Name: "targetCode"},
		},
		Selectors: familySelector,
	})
	AvailabilityAdjustmentCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: "network-catalog/availability-adjustment",
		Sorts: []cataloguepage.SortDimension{
			byCode,
			{Name: "targetCode", Kind: cataloguepage.Text},
			{Name: "effectiveAt", Kind: cataloguepage.Instant},
		},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters: []cataloguepage.FilterDimension{
			{Name: "code"},
			{Name: "targetKind", Vocabulary: targetKindVocabulary},
			{Name: "targetCode"},
			{Name: "kind", Vocabulary: []string{
				AdjustmentSuspension.String(),
				AdjustmentClosure.String(),
				AdjustmentResumption.String(),
				AdjustmentScopeAdjustment.String(),
			}},
		},
		Selectors: familySelector,
	})
	RouteStrategyVersionCatalogue = cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name:        "network-catalog/route-strategy",
		Sorts:       []cataloguepage.SortDimension{byCode, byEffectiveFrom},
		DefaultSort: cataloguepage.Sort{Field: "code"},
		Identity:    codeAndVersion,
		Filters:     []cataloguepage.FilterDimension{{Name: "code"}},
		Selectors:   familySelector,
	})
)

var (
	byCode          = cataloguepage.SortDimension{Name: "code", Kind: cataloguepage.Text}
	byEffectiveFrom = cataloguepage.SortDimension{Name: "effectiveFrom", Kind: cataloguepage.Instant}
	codeAndVersion  = []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Integer}
	familySelector  = []string{"family"}

	targetKindVocabulary = []string{TargetNode.String(), TargetConnection.String(), TargetLine.String()}
)
