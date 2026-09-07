package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardVersionLoader 按（租户 + 方案版本引用）读回一版已登记价卡——**「原评价用的那一版」，
// 不是「此刻适用的那一版」**。两者在方案换版之后不是同一张：LoadApplicable 按（方向 + 适用范围 +
// 计价基准时点）选卡，换版后同一基准时点选出来的是新版；重放（CONTEXT「重放必须使用原版本清单，
// 不读取当前最新版本替代」）要的是 original.PlanReference() 指名的那一版，键因此只能是版本引用。
//
// 另立接口而不扩 PriceCardCatalog，理由同 catalogue_read.go 与 reference_series_register.go 那一条：
// 扩写侧接口会拆全部写侧测试替身；ReferenceSeriesVersionLoader 就是同一形状的先例。
type PriceCardVersionLoader interface {
	// FindByReference 按（租户、方案版本引用三元）读回那一版方案。读回的登记已过领域重建门
	// （RehydratePriceCardRegistration 整图重验，含按规范化版本重算内容摘要自校）与比对列交叉核。
	//
	// 那一版不在册答 false 不答 error——**也不退回在用版本**：对重放而言这是「结构上重放不了」
	// 的一种，如实交给编排答未形成，拿此刻适用的那一版顶上会让一次结构失败长得像内容冲突。
	// 快照按另一套规范化版本折装、本构建重建不了时交回包着 domain.ErrCanonicalizationVersionUnsupported
	// 的错误（ADR-0014）：那也是结构上重放不了，但恢复动作与「不在册」不同——后者能登，前者只能等
	// 支持该形状的构建——编排按 errors.Is 分格。引用的种类不是 pricing-plan 是调用方的错，答 error。
	FindByReference(ctx context.Context, tenant domain.TenantID, reference domain.VersionReference) (domain.PricingPlanVersion, bool, error)
}
