package ports

import (
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/04）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列、q 覆盖哪几列在 postgres 读面里。缺省序保迁移前的次序
// （该票 spec 取舍 1）；决胜键是行标识、与排序同向（决定三）——迁移前首列倒序、决胜键正序的几册，只在首列完全相同的行之间
// 次序变了。
//
// /pricing-pending-series-evaluations 不在这里：它答的是按序列种类分组的计数，不是一册行，组数以种类的封闭集为界。

// EvaluationPages 是评价登记册（GET /pricing-evaluations）的声明。
//
// 答复体没有 registeredAt 一格，recordedAt 是评价落册的时刻、与登记时间同义；缺省序取它倒序，与迁移前同一口径。可排维只放
// 这一维：评价标识是不透明串，按它换序没有操作意义。status 放出作筛选维（决定四），词表是迁移钉住的闭集。q 覆盖
// evaluationId、semanticDigest 与 planContentDigest。
//
// 评价按请求计量，是本上下文行数最多的一册：total 给精确值以「按租户读、租户级规模」为前提（ADR-0144 越权风险点 3），哪天
// 超出，回那份 ADR 把它改成 null。
var EvaluationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "pricing-evaluations",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: []string{
			string(domain.EvaluationCompleted),
			string(domain.EvaluationPending),
			string(domain.EvaluationConflict),
			string(domain.EvaluationFailed),
			string(domain.EvaluationUnratable),
		}},
	},
})

// PriceCardPages 是价卡目录（GET /pricing-price-cards）的声明，一版一行。
//
// 缺省序照决定三取 -registeredAt，与迁移前同一口径；行标识是（方案标识、方案版本）。planId 另放作可排维：价卡是按方案看版本
// 的册（ADR-0144 越权风险点 1），按它排，同一方案的各版挨在一起。direction 是闭集，放出作筛选维（决定四）；计算目的与
// 方向首发一一对应，不另开一维。q 覆盖 planId、planVersion 与 scope。
//
// 管理台价卡页的状态（admin-web 的 priceCardStatus）是页面拿此刻对适用期与收尾各格现算的，不是登记册上的列，本册不放它。
var PriceCardPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "pricing-price-cards",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "planId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "direction", Vocabulary: []string{
			domain.PricingDirectionBuy.String(),
			domain.PricingDirectionSell.String(),
			domain.PricingDirectionInternal.String(),
		}},
	},
})

// ReferenceSeriesPages 是参考序列登记册（GET /pricing-reference-series）的声明，一版一行。
//
// 缺省序照决定三取 -registeredAt，与迁移前同一口径；行标识是（序列标识、序列版本）。seriesId 另放作可排维，理由同价卡的
// planId。kind 是闭集，放出作筛选维（决定四）；seriesId 也放作筛选维、不设词表——管理台从覆盖摘要「只看这一条序列」是按
// 标识精确取，q 的包含匹配会把同前缀的兄弟序列一并带进来。q 覆盖 seriesId、sourceIdentifier 与 registrant。
//
// 复核事实不进任何一维：它们是按版本从复核册连过来的计数，page 与 total 数的是版本。
var ReferenceSeriesPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "pricing-reference-series",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "seriesId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "kind", Vocabulary: referenceSeriesKinds},
		{Name: "seriesId"},
	},
})

// ReferenceSeriesCoveragePages 是参考序列覆盖（GET /pricing-reference-series-coverage）的声明，一条序列（标识 + 种类）一行。
//
// 这一行是一条序列各版的聚合，没有登记时间一格；缺省序取 seriesId 升序，与迁移前同一口径——覆盖页是按序列找行，不按登记
// 先后。行标识是（序列标识、种类）：版本表没有约束把一条标识钉在一个种类上（ReferenceSeriesCoverageRow 头注），同一标识两个
// 种类各是一行。kind 放出作筛选维（决定四）；q 覆盖 seriesId。page 与 total 数的是序列，不是版本。
var ReferenceSeriesCoveragePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "pricing-reference-series-coverage",
	Sorts:       []cataloguepage.SortDimension{{Name: "seriesId", Kind: cataloguepage.Text}},
	DefaultSort: cataloguepage.Sort{Field: "seriesId"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "kind", Vocabulary: referenceSeriesKinds}},
})

// PriceCardDraftPages 是价卡草稿查阅（GET /pricing-price-card-draft-views）的声明，一版一行。
//
// 草稿不是价卡版本、不进登记册，没有登记时间一格；submittedAt 是这一版最近一次录入的时刻，缺省序取它倒序，与迁移前同一口径。
// 行标识是（方案标识、方案版本），答复体里它们在 plan 对象下、不是顶层字段，所以不作可排维。status 是草稿状态的闭集，放出作
// 筛选维（决定四），词表取自领域 PriceCardDraftStatus；它接替迁移前 Intake 解的那个单值 status（决定六：查询意图不归 Intake），
// 现在同一维可重复给、维内为或。q 覆盖方案标识、方案版本与源文件名。
//
// 草稿表就地更新：`已批准`之前再录会换掉 submittedAt，翻页期间被修订的那一版跳到已翻过的那头，与新录入的同一个去向。
var PriceCardDraftPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "pricing-price-card-draft-views",
	Sorts:       []cataloguepage.SortDimension{{Name: "submittedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: cataloguepage.Words[domain.PriceCardDraftStatus]()},
	},
})

// referenceSeriesKinds 是序列种类的闭集，序列册与覆盖册共用。
var referenceSeriesKinds = []string{
	domain.ReferenceSeriesFuelRate.String(),
	domain.ReferenceSeriesExchangeRate.String(),
	domain.ReferenceSeriesPublishedAmount.String(),
}
