package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是计价参考目录（ADR-0109）的登记口、复核口与在用读口。三口都另立而不拓宽序列那几个接口：
// 扩既有接口会拆全部替身（catalogue_read.go 记过这条），且目录的解析多一个邮编路线入参，形状本就不同。

// ReferenceCatalogueRegistrationOutcome 是目录登记的写入结果代数，与序列登记同一条纪律：冲突是业务
// 答案不是错误，登记册拒绝静默替换，原行永不被顶替。
type ReferenceCatalogueRegistrationOutcome uint8

const (
	ReferenceCatalogueRegistrationOutcomeInvalid ReferenceCatalogueRegistrationOutcome = iota
	// ReferenceCatalogueRegistered：新目录版本已入册。
	ReferenceCatalogueRegistered
	// ReferenceCatalogueAlreadyRegistered：同版本引用、同规范化形状、同内容摘要已在册——幂等重放。
	ReferenceCatalogueAlreadyRegistered
	// ReferenceCatalogueContentConflict：版本引用相同、规范化形状也相同而内容摘要不同。映射更正必须形成
	// 新版本，同版本装不同映射不得静默替换。
	ReferenceCatalogueContentConflict
	// ReferenceCatalogueCanonicalizationDiffers：同版本引用已按另一套规范化形状在册，摘要不可比。
	ReferenceCatalogueCanonicalizationDiffers
)

// ReferenceCatalogueRegister 是计价参考目录登记册的存取口。它拥有目录版本的登记行与按计价基准时点、
// 邮编路线的解析（ADR-0109 Decision 二、四），不生产任何一行映射——内容全属实例半边。
type ReferenceCatalogueRegister interface {
	// Register 登记一版目录。行只增不改；同键重复按（规范化形状 + 内容摘要）比对译成结果代数。
	Register(ctx context.Context, registration domain.ReferenceCatalogueRegistration) (ReferenceCatalogueRegistrationOutcome, error)
	// ResolveAt 按（租户 + 目录版本引用 + 计价基准时点 + 邮编路线）解一个读数。版本不在册或时点不在其
	// 生效区间内答 false；在册且适用时读数一定交回——查不到该邮编的读数值缺席、版本引用在，评价侧据以
	// 落待判断，不编默认。
	ResolveAt(ctx context.Context, tenant domain.TenantID, reference domain.VersionReference, asOf time.Time, route domain.PostalRoute) (domain.ResolvedCatalogueValue, bool, error)
}

// ReferenceCatalogueVersionLoader 按（租户 + 目录标识 + 版本号）读回一版已登记目录（整版重验）。复核用例
// 需要它：四眼门要拿到登记责任方。
type ReferenceCatalogueVersionLoader interface {
	LoadVersion(ctx context.Context, tenant domain.TenantID, catalogueID, catalogueVersion string) (domain.ReferenceCatalogueRegistration, bool, error)
}

// ReferenceCatalogueReviewOutcome 是记录一条目录复核的封闭结果，四格与序列复核同义。
type ReferenceCatalogueReviewOutcome uint8

const (
	ReferenceCatalogueReviewOutcomeInvalid ReferenceCatalogueReviewOutcome = iota
	ReferenceCatalogueReviewRecorded
	ReferenceCatalogueReviewAlreadyRecorded
	ReferenceCatalogueReviewConflict
	// ReferenceCatalogueReviewVersionUnknown：被复核的目录版本不在册——先登记那一版。
	ReferenceCatalogueReviewVersionUnknown
)

// ReferenceCatalogueReviewRegister 是目录版本复核的追加写口。
type ReferenceCatalogueReviewRegister interface {
	Record(ctx context.Context, review domain.CatalogueReview) (ReferenceCatalogueReviewOutcome, error)
}

// CatalogueInForceOutcome 是目录在用版本解析的封闭结果，三个「没有」按恢复动作分格（与序列同一判据）。
type CatalogueInForceOutcome uint8

const (
	CatalogueInForceOutcomeInvalid CatalogueInForceOutcome = iota
	// CatalogueVersionInForce：解析到在用版本，引用随答案交回。
	CatalogueVersionInForce
	// CatalogueHasNoRegisteredVersion：该租户内没有这本目录的任何版本。
	CatalogueHasNoRegisteredVersion
	// CatalogueHasNoApprovedVersion：有版本，但在该时刻之前没有任何一版复核通过。
	CatalogueHasNoApprovedVersion
	// CatalogueKindDisagrees：这本目录在册的种类与卡绑定的种类不同——卡绑错了目录，或目录登错了种类。
	CatalogueKindDisagrees
)

// ReferenceCatalogueInForceResolver 按（租户、种类、目录标识、时刻）解析在用版本。目录按计价基准时点
// 选版（ADR-0109 Decision 二）；候选只取、不在 SQL 里裁决，判定在 domain.SelectInForceCatalogueVersion 一处。
type ReferenceCatalogueInForceResolver interface {
	ResolveInForce(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CatalogueKind,
		catalogueID string,
		at time.Time,
	) (domain.VersionReference, CatalogueInForceOutcome, error)
}
