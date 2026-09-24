package customshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：监管回执经集成客户端族
// 进来（ADR-0149，回执来源的实例取值属 `PAR-INT-03`）、运营查阅接入面的认证归操作者渠道（ADR-0100），
// 两族的真 Intake 都未就位，装配点上还
// 没有一行真通道 Intake（ADR-0055）。哨兵只此一个而不随端点分设：未配置是渠道这一层
// 的状态，按端点分设哨兵会让装配点看起来能只配一半（判据同 visibilityhttp）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置通道参数，改报文或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让通道侧把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("customs compliance http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每份报文不读内容、不采信任何
// 自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 ResultIntake 注释所禁的「开发用」采信实现——那条红线禁的是采信报文自称的租户
// 号（穿透 ADR-0003 的隔离边界）；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答……不是默认实现，恰恰是它想保护的东西」。
// 这里的空登记册就是装配点本身。
// ADR-0023 要求从报文体收的来源标识与发生时间同样无从谈起：连命令都不构造，也就没有
// 任何一样外部事实被服务端代铸。真通道 Intake 就位时在装配点替换，本类型随之退场，
// 路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var (
	_ ResultIntake         = UnconfiguredIntake{}
	_ CatalogueQueryIntake = UnconfiguredIntake{}
)

// IntakeResult 不读报文。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeResult(context.Context, *http.Request) (application.ReceiveExternalResultCommand, error) {
	return application.ReceiveExternalResultCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCatalogueQuery 同 IntakeResult：不读请求，只答未配置。运营接入面的认证方式
// 同属接入渠道实例半边（ADR-0077 Decision 三），未登记前不铸造任何作用域。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

// 五类配置登记命令口的未配置实现（ADR-0085）：与上面两口同一分界——不读业务内容、
// 不采信自报身份、不构造命令。隔离读放行（ADR-0078）不实现这五个接口，写行换不了。
var (
	_ InterpretationRuleRegistrationIntake = UnconfiguredIntake{}
	_ GateCatalogRegistrationIntake        = UnconfiguredIntake{}
	_ CandidatePortRegistrationIntake      = UnconfiguredIntake{}
	_ DeclarationPathRegistrationIntake    = UnconfiguredIntake{}
	_ CaseRequirementRegistrationIntake    = UnconfiguredIntake{}
)

// IntakeInterpretationRuleRegistration 不读请求，判据同上。以下四个同此。
func (UnconfiguredIntake) IntakeInterpretationRuleRegistration(
	context.Context,
	*http.Request,
) (application.RegisterInterpretationRuleCommand, error) {
	return application.RegisterInterpretationRuleCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeGateCatalogRegistration(
	context.Context,
	*http.Request,
) (application.RegisterGateCatalogCommand, error) {
	return application.RegisterGateCatalogCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCandidatePortRegistration(
	context.Context,
	*http.Request,
) (application.RegisterCandidatePortCommand, error) {
	return application.RegisterCandidatePortCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDeclarationPathRegistration(
	context.Context,
	*http.Request,
) (application.RegisterDeclarationPathCommand, error) {
	return application.RegisterDeclarationPathCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCaseRequirementRegistration(
	context.Context,
	*http.Request,
) (application.RegisterCaseRequirementRuleCommand, error) {
	return application.RegisterCaseRequirementRuleCommand{}, ErrAccessChannelNotConfigured
}

// 凭证、税费付款协作事项、税费付款核对三口的未配置实现（票 sa-cc/07 步二），分界同上：
// 不读业务内容、不采信自报身份、不构造命令。写准入不另立形（ADR-0085 Decision 二），
// 隔离读放行同样装不进这三口。
var (
	_ RegulatoryCredentialRegistrationIntake    = UnconfiguredIntake{}
	_ DutyCollaborationRegistrationIntake       = UnconfiguredIntake{}
	_ DutyPaymentVerificationRegistrationIntake = UnconfiguredIntake{}
)

func (UnconfiguredIntake) IntakeRegulatoryCredentialRegistration(
	context.Context,
	*http.Request,
) (application.RegisterCredentialCommand, error) {
	return application.RegisterCredentialCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDutyCollaborationRegistration(
	context.Context,
	*http.Request,
) (application.FormDutyCollaborationCommand, error) {
	return application.FormDutyCollaborationCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDutyPaymentVerificationRegistration(
	context.Context,
	*http.Request,
) (application.VerifyDutyPaymentCommand, error) {
	return application.VerifyDutyPaymentCommand{}, ErrAccessChannelNotConfigured
}
