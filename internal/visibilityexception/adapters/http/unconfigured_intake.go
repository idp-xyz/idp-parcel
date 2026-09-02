package visibilityhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实渠道的认证方式
// 属 `PAR-INT-01` 待提供，装配点上还没有一行真渠道 Intake（ADR-0055）。
//
// 与 ErrMalformedRequest、ErrMalformedClaim 各分一格的判据同 ADR-0029——恢复动作不同：
// 这一格要接入方去提供并配置渠道参数，改请求或重试都不会好。哨兵只此一个而不随端点分
// 设：未配置是渠道这一层的状态，本包各端点等的是同一件事，按端点分设哨兵会让装配点
// 看起来能只配一半。本包据以回 403 + ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与
// 「产品没有这个能力」不可分辨，折进 INTAKE_FAILED（5xx）会让客户端把一件人不来配就
// 永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("visibility exception http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造查询键或命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 QueryIntake / ClaimIntake 注释所禁的「开发用」采信实现——那条红线禁的是采信
// 客户自报的账户号（穿透账户隔离）；本类型恰是其反面，分界同 ADR-0052：「读一个空
// 登记册并如实答未配置不是默认实现，恰恰是它想保护的东西」。这里的空登记册就是装配点
// 本身。
//
// 与 ADR-0029 的探针纪律不冲突：403 披露的是「本产品有此端点、渠道未配置」，属产品
// 表面；对象是否存在、是否属于别的客户仍由 VIEW_NOT_FOUND 一格同答，那条约束在真渠道
// Intake 就位后照常适用（ADR-0055 第四条）。真渠道就位时在装配点替换，本类型随之退场。
type UnconfiguredIntake struct{}

var (
	_ QueryIntake              = UnconfiguredIntake{}
	_ ClaimIntake              = UnconfiguredIntake{}
	_ OperationsTrackingIntake = UnconfiguredIntake{}
)

// IntakeQuery 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeQuery(context.Context, *http.Request) (TrackingViewQuery, error) {
	return TrackingViewQuery{}, ErrAccessChannelNotConfigured
}

// IntakeClaim 同 IntakeQuery：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeClaim(context.Context, *http.Request) (application.ReceiveClaimCommand, error) {
	return application.ReceiveClaimCommand{}, ErrAccessChannelNotConfigured
}

// IntakeOperationsQuery 同 IntakeQuery：不读请求，只答未配置。运营接入面的认证方式
// 同属接入渠道实例半边（ADR-0076 第三条），未登记前不铸造任何作用域。
func (UnconfiguredIntake) IntakeOperationsQuery(context.Context, *http.Request) (OperationsTrackingQuery, error) {
	return OperationsTrackingQuery{}, ErrAccessChannelNotConfigured
}

// 六类配置登记命令口的未配置实现（ADR-0085）：与查阅口同一分界——不读业务内容、不采信
// 自报身份、不构造命令。隔离读放行（ADR-0078）不实现这六个接口，写行换不了。
var (
	_ MilestoneMappingRegistrationIntake   = UnconfiguredIntake{}
	_ TriageRulesRegistrationIntake        = UnconfiguredIntake{}
	_ NotificationPolicyRegistrationIntake = UnconfiguredIntake{}
	_ ClaimEligibilityRegistrationIntake   = UnconfiguredIntake{}
	_ ClaimAuthorizationRegistrationIntake = UnconfiguredIntake{}
	_ DisclosurePolicyRegistrationIntake   = UnconfiguredIntake{}
)

// IntakeMilestoneMappingRegistration 不读请求，判据同上。以下五个同此。
func (UnconfiguredIntake) IntakeMilestoneMappingRegistration(
	context.Context,
	*http.Request,
) (application.RegisterMilestoneMappingCommand, error) {
	return application.RegisterMilestoneMappingCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeTriageRulesRegistration(
	context.Context,
	*http.Request,
) (application.RegisterTriageRulesCommand, error) {
	return application.RegisterTriageRulesCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeNotificationPolicyRegistration(
	context.Context,
	*http.Request,
) (application.RegisterNotificationPolicyCommand, error) {
	return application.RegisterNotificationPolicyCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeClaimEligibilityRegistration(
	context.Context,
	*http.Request,
) (application.RegisterClaimEligibilityCommand, error) {
	return application.RegisterClaimEligibilityCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeClaimAuthorizationRegistration(
	context.Context,
	*http.Request,
) (application.RegisterClaimAuthorizationCommand, error) {
	return application.RegisterClaimAuthorizationCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDisclosurePolicyRegistration(
	context.Context,
	*http.Request,
) (application.RegisterDisclosurePolicyCommand, error) {
	return application.RegisterDisclosurePolicyCommand{}, ErrAccessChannelNotConfigured
}
