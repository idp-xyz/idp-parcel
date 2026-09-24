package pricinghttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道:运营接入认证属
// 操作者渠道(ADR-0100),其真 Intake 未就位,装配点上还没有一行真渠道 Intake
// (ADR-0055、ADR-0077 Decision 三)。
//
// 恢复动作判据同 ADR-0029:这一格要接入方去提供并配置渠道参数,改请求或重试都不会
// 好。哨兵只此一个而不随端点分设:未配置是渠道这一层的状态,按端点分设哨兵会让装配
// 点看起来能只配一半。本包据以回 403 + ACCESS_CHANNEL_NOT_CONFIGURED;折进 404 会与
// 「产品没有这个能力」不可分辨,折进 INTAKE_FAILED(5xx)会让客户端把一件人不来配就
// 永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("parcel pricing http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态,不命名参数(ADR-0055):这里等的是哪个
// 登记册行由参数登记册说,错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复:对每个请求不读业务内容、不采信
// 任何自报身份、不构造查询,一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信自报租户(穿透 ADR-0003 的
// 隔离);本类型恰是其反面,分界同 ADR-0052:「读一个空登记册并如实答未配置不是默认
// 实现,恰恰是它想保护的东西」。这里的空登记册就是装配点本身。真渠道就位时在装配点
// 替换,本类型随之退场。
type UnconfiguredIntake struct{}

var _ PricingCatalogueIntake = UnconfiguredIntake{}

// IntakeCatalogueQuery 不读请求。参数刻意匿名:连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

// 登记命令口的未配置实现（ADR-0085）：与查阅口同一分界——不读业务内容、不采信
// 自报身份、不构造命令。隔离读放行（ADR-0078）不实现这两个接口，写行换不了。
var (
	_ PriceCardRegistrationIntake          = UnconfiguredIntake{}
	_ ReferenceSeriesRegistrationIntake    = UnconfiguredIntake{}
	_ ReferenceSeriesReviewIntake          = UnconfiguredIntake{}
	_ ReferenceSeriesPreviewIntake         = UnconfiguredIntake{}
	_ ReferenceCatalogueRegistrationIntake = UnconfiguredIntake{}
	_ EvaluationReplayIntake               = UnconfiguredIntake{}
	_ EstimateIntake                       = UnconfiguredIntake{}
)

// IntakeEstimate 不读请求，判据同回放口：等的是操作者信封接线（ADR-0100，与回放同批）。从请求里取一个租户就是自报身份，
// 这一口不能有那种「开发用」版本（ADR-0152 决定六）。
func (UnconfiguredIntake) IntakeEstimate(context.Context, *http.Request) (application.FormEstimateEvaluationsCommand, error) {
	return application.FormEstimateEvaluationsCommand{}, ErrAccessChannelNotConfigured
}

// IntakeEvaluationReplay 不读请求，判据同复核口：等的是操作者信封接线（ADR-0100），载荷形状已在
// DecodeEvaluationReplayPayload。**从请求里铸一个触发者、或替它填一个证据层级**，都是这一口
// 不能有的「开发用」版本——前者是自报身份，后者是把 `S` 写成默认（ADR-0124 决定三）。
func (UnconfiguredIntake) IntakeEvaluationReplay(context.Context, *http.Request) (application.ReplayPricingEvaluationCommand, error) {
	return application.ReplayPricingEvaluationCommand{}, ErrAccessChannelNotConfigured
}

// IntakeReferenceCatalogueRegistration 不读请求，判据同登记口：等的是操作者信封接线（ADR-0100），
// 载荷形状（模板导入）已在 DecodeReferenceCataloguePayload。
func (UnconfiguredIntake) IntakeReferenceCatalogueRegistration(context.Context, *http.Request) (application.RegisterReferenceCatalogueCommand, error) {
	return application.RegisterReferenceCatalogueCommand{}, ErrAccessChannelNotConfigured
}

// IntakePriceCardRegistration 不读请求，判据同上。
func (UnconfiguredIntake) IntakePriceCardRegistration(context.Context, *http.Request) (application.RegisterPriceCardCommand, error) {
	return application.RegisterPriceCardCommand{}, ErrAccessChannelNotConfigured
}

// IntakeReferenceSeriesRegistration 不读请求，判据同上。
func (UnconfiguredIntake) IntakeReferenceSeriesRegistration(context.Context, *http.Request) (application.RegisterReferenceSeriesCommand, error) {
	return application.RegisterReferenceSeriesCommand{}, ErrAccessChannelNotConfigured
}

// IntakeReferenceSeriesReview 不读请求，判据同上。**这一口尤其不能有「开发用」版本**：
// 复核责任方是四眼门的一半，从请求内容里铸一个出来就等于把那道门拆了。
//
// 它等的与上面两个登记口不同：那两个等渠道接入契约，这一个等 ADR-0100 的操作者信封接线
// ——载荷形状不在等待之列，ADR-0101 决定一已把运营操作者面的它划归产品。展开在
// ReferenceSeriesReviewIntake 的自注。
func (UnconfiguredIntake) IntakeReferenceSeriesReview(context.Context, *http.Request) (application.ReviewReferenceSeriesCommand, error) {
	return application.ReviewReferenceSeriesCommand{}, ErrAccessChannelNotConfigured
}

// IntakeReferenceSeriesPreview 不读请求，判据同上。预览不写库，但拟登本体要信封里的租户与
// 登记责任方才立得住，等的与登记口是同一样东西——从载荷里取一个登记责任方来「先预览着」，
// 登记时换成信封里的那个，摘要就不是同一个了。
func (UnconfiguredIntake) IntakeReferenceSeriesPreview(context.Context, *http.Request) (application.PreviewReferenceSeriesCommand, error) {
	return application.PreviewReferenceSeriesCommand{}, ErrAccessChannelNotConfigured
}
