package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ReferenceSeriesRegistrationOutcome 是序列登记的写入结果代数。冲突是业务答案不是
// 错误（同价卡登记一条纪律）：登记册拒绝静默替换，事务保持可用，调用方拿答案续办。
type ReferenceSeriesRegistrationOutcome uint8

const (
	ReferenceSeriesRegistrationOutcomeInvalid ReferenceSeriesRegistrationOutcome = iota
	// ReferenceSeriesRegistered：新序列版本已入册。
	ReferenceSeriesRegistered
	// ReferenceSeriesAlreadyRegistered：同版本引用、同规范化形状、同内容摘要已在册
	// ——重复登记是幂等重放，不是错误。
	ReferenceSeriesAlreadyRegistered
	// ReferenceSeriesContentConflict：版本引用相同、规范化形状也相同而内容摘要不同。
	// 取值更正必须形成新版本（CONTEXT），同版本装不同取值不得重放或静默替换；原行
	// 保持原样。
	ReferenceSeriesContentConflict
	// ReferenceSeriesCanonicalizationDiffers：同版本引用已按另一套规范化形状在册。
	// 摘要只在同一形状版本内可比（ADR-0014 纪律），这不是内容冲突也不是幂等重放，
	// 由治理责任方裁决续办；原行保持原样。
	ReferenceSeriesCanonicalizationDiffers
)

// ReferenceSeriesRegister 是计价参考序列登记册的存取口（票 08 的机制三件之①②）。
// 它拥有序列版本的登记行与按计价基准时点的解析（ADR-0013），不生产数值、不选定商业
// 口径——期次取值全属实例半边，本口只承载形状与证据等级。
type ReferenceSeriesRegister interface {
	// Register 登记一版序列取值。行只增不改；同键重复按（规范化形状 + 内容摘要）
	// 比对译成结果代数，原行永不被顶替。
	Register(ctx context.Context, registration domain.ReferenceSeriesRegistration) (ReferenceSeriesRegistrationOutcome, error)
	// ResolveAt 按（租户 + 序列版本引用 + 计价基准时点）解析一期取值，供评价冻结进
	// 输入并写入版本清单。序列版本不在册或时点不落在任何期次内都答未解析（false）
	// ——缺口是证据不足，评价侧据以挂起，不编数值。
	ResolveAt(ctx context.Context, tenant domain.TenantID, reference domain.VersionReference, asOf time.Time) (domain.ResolvedSeriesReading, bool, error)
}

// ReferenceSeriesVersionLoader 按（租户 + 序列版本引用）读回一版已登记序列（整版重验）。
// 复核用例需要它：四眼门要拿到登记责任方，而 domain.NewSeriesReview 以登记为入参。另立
// 接口而不扩 ReferenceSeriesRegister，理由同 catalogue_read.go——扩写侧接口会拆全部写侧
// 测试替身。
type ReferenceSeriesVersionLoader interface {
	// LoadVersion 按（租户、序列标识、版本号）读回。键上不要摘要：复核责任方与受控批量口
	// 手上只有标识与版本号，摘要在登记里，读回之后才有。版本不在册答 false 不答 error；
	// 读回的登记已过领域重建门与比对列交叉核。
	LoadVersion(ctx context.Context, tenant domain.TenantID, seriesID, seriesVersion string) (domain.ReferenceSeriesRegistration, bool, error)
}
