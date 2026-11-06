package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// 本文件是路由判断两册、改路建议与初始路由并列四册的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/07）：
// 可排维与筛选维按答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。网络目录各族的声明另在
// catalog_query.go，答页用那边的 CatalogPage；这四册答共用件的 cataloguepage.Result。
//
// 缺省序保迁移前的次序。决胜键是行标识、与排序同向（决定三）：迁移前四册都是首维倒序、标识段正序，如今首维同值的几行改按标识
// 倒序排。四册都只放出缺省序那一维：初始路由按包裹、复核按事件计量，是本上下文量最大的册，索引只配与缺省序同形的那一条（决定七），
// 另放的维在这几册上每取一页都要把本租户整册排一遍。
//
// q 覆盖各册的标识列：判断键里的客户账户、委托、受理基线与包裹，再加各册自己的触发关联与计划版本。服务目的不进 q——它是一格
// 目的词，不是哪个对象的标识，搜它只会命中整册。

// InitialRoutePages 是初始路由判断册（GET /route-plans?register=initial-route）的声明。
//
// recordedAt 是判断落册的时刻，即本册的登记时刻：缺省序就是决定三的「登记时刻倒序」，只是字段名照答复体叫 recordedAt。行标识是
// 判断键去掉租户的五段，各段都进标识——同一包裹换了受理基线或服务目的就是另一行。conclusion 放出作筛选维，词表照迁移里结论列的
// CHECK 约束。register 是册子选择器，保持原义、不兼作筛选维。
var InitialRoutePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "route-plans/initial-route",
	Sorts:       []cataloguepage.SortDimension{byRecordedAt},
	DefaultSort: newestRecordedFirst,
	Identity:    judgmentKeyIdentity,
	Filters: []cataloguepage.FilterDimension{
		{Name: "conclusion", Vocabulary: []string{"ROUTE_FORMED", "NO_CURRENT_ROUTE"}},
	},
	Selectors: registerSelector,
})

// RouteReassessmentPages 是路由复核册（GET /route-plans?register=reassessment）的声明。
//
// 缺省序与初始路由册同一口径；行标识是触发关联，一次触发只复核一次。conclusion 放出作筛选维，词表取自 ReassessmentConclusionKind。
var RouteReassessmentPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "route-plans/reassessment",
	Sorts:       []cataloguepage.SortDimension{byRecordedAt},
	DefaultSort: newestRecordedFirst,
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "conclusion", Vocabulary: cataloguepage.Words[ReassessmentConclusionKind]()},
	},
	Selectors: registerSelector,
})

// RerouteSuggestionPages 是改路建议读面（GET /route-reroute-suggestions）的声明。
//
// 建议住在复核行上，缺省序取那次复核落册的时刻，与复核册同一口径；答复体为此带出 recordedAt。行标识是建议所在复核的触发关联。
// state 放出作筛选维，词表是 RerouteSuggestionState 的各格。
var RerouteSuggestionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "route-reroute-suggestions",
	Sorts:       []cataloguepage.SortDimension{byRecordedAt},
	DefaultSort: newestRecordedFirst,
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{{Name: "state", Vocabulary: []string{
		string(RerouteSuggestionPending), string(RerouteSuggestionDecided), string(RerouteSuggestionSuperseded),
	}}},
})

// InitialRouteTiePages 是初始路由并列读面（GET /route-initial-route-ties）的声明。
//
// 缺省序不是登记时刻：未形成登记同一判断键再停一次就改写那一行，settledAt 是最近一次停住的时刻，册上看的正是「最近停在并列的」；
// 登记时刻只记头一次落库，答复体里也没有它。settledAt 随改写前移，翻页期间被改写的那一行可能挪进已翻过的区间。行标识是判断键
// 去掉租户的五段，段序照迁移前的决胜次序以包裹打头。state 放出作筛选维，词表是 InitialRouteTieState 的各格。
var InitialRouteTiePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "route-initial-route-ties",
	Sorts:       []cataloguepage.SortDimension{{Name: "settledAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "settledAt", Descending: true},
	Identity:    judgmentKeyIdentity,
	Filters: []cataloguepage.FilterDimension{{Name: "state", Vocabulary: []string{
		string(InitialRouteTiePending), string(InitialRouteTieDecided), string(InitialRouteTieFormedOtherwise),
	}}},
})

var (
	byRecordedAt        = cataloguepage.SortDimension{Name: "recordedAt", Kind: cataloguepage.Instant}
	newestRecordedFirst = cataloguepage.Sort{Field: "recordedAt", Descending: true}
	judgmentKeyIdentity = []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text,
	}
	registerSelector = []string{"register"}
)
