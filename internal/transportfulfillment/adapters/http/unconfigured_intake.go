package tfhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实渠道的认证方式
// 属 `PAR-INT-01` 待提供，装配点上还没有一行真渠道 Intake（ADR-0055）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置渠道参数，改请求或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让离线设备把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("transport fulfillment http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 DeliveryIntake 注释所禁的「开发用」采信实现——那条红线禁的是从请求内容铸造
// 来源信封；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答未配置不是
// 默认实现，恰恰是它想保护的东西」。这里的空登记册就是装配点本身。ADR-0023 要求的
// POD 证据与更正时间同样无从谈起：连命令都不构造，也就没有任何一样东西被代铸。
//
// 首登与更正两个方法都堵住：未配置是渠道这一层的状态，不是某个端点的状态，只堵一个
// 就是给另一个留了条无渠道也能进的路。真渠道 Intake 就位时在装配点替换，本类型随之
// 退场，路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var _ DeliveryIntake = UnconfiguredIntake{}
var _ CatalogueQueryIntake = UnconfiguredIntake{}
var _ HandoverIntake = UnconfiguredIntake{}
var _ PickupRegistrationIntake = UnconfiguredIntake{}
var _ PickupAttemptIntake = UnconfiguredIntake{}
var _ MovementFactIntake = UnconfiguredIntake{}
var _ SegmentClosureIntake = UnconfiguredIntake{}
var _ DispatchTaskIntake = UnconfiguredIntake{}
var _ LoadAssignmentIntake = UnconfiguredIntake{}
var _ ParticipationTerminationIntake = UnconfiguredIntake{}
var _ CredentialIntake = UnconfiguredIntake{}
var _ EffectiveTimeRuleIntake = UnconfiguredIntake{}

// IntakeRegistration 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeRegistration(context.Context, *http.Request) (application.RegisterEffectiveDeliveryCommand, error) {
	return application.RegisterEffectiveDeliveryCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCorrection 同 IntakeRegistration：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeCorrection(context.Context, *http.Request) (application.CorrectDeliveryProofCommand, error) {
	return application.CorrectDeliveryProofCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCatalogueQuery 同上两个方法：不读请求，只答未配置。命令面与查阅面同堵——
// 本类型注释立的正是这条：未配置是渠道这一层的状态，不是某个端点的状态，只堵命令
// 就是给查阅留了条无渠道也能进的路。未登记前不铸造任何作用域（ADR-0077 Decision
// 三）。隔离读准入启用时由装配点换成 IsolatedOperationsReadIntake，本类型在查阅面
// 随之退场，命令面不受影响（ADR-0078）。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

// IntakeHandoverRegistration 与 IntakeHandoverCorrection 堵住交接两口（票 04）。控制事实入口
// 比交付更要堵严：命令里带着段引用，任何一条穿过去的请求都会在段登记册上立出一个来源
// 不明的实际履约段。同样不读请求、不构造命令。
func (UnconfiguredIntake) IntakeHandoverRegistration(context.Context, *http.Request) (application.RegisterTransportHandoverCommand, error) {
	return application.RegisterTransportHandoverCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeHandoverCorrection(context.Context, *http.Request) (application.CorrectTransportHandoverCommand, error) {
	return application.CorrectTransportHandoverCommand{}, ErrAccessChannelNotConfigured
}

// IntakePickupRegistration 与 IntakePickupAttempt 堵住揽收两口，理由同交接两口。
func (UnconfiguredIntake) IntakePickupRegistration(context.Context, *http.Request) (application.RegisterOffsitePickupCommand, error) {
	return application.RegisterOffsitePickupCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePickupAttempt(context.Context, *http.Request) (application.PerformOffsitePickupCommand, error) {
	return application.PerformOffsitePickupCommand{}, ErrAccessChannelNotConfigured
}

// IntakeMovementFact 堵住移动事实口（票 05）。渠道未就位前「自营执行方」这个身份无从认定，所以
// 连命令都不构造——这一格也是「外部轨迹不从这里进」在渠道层的第一道门。
func (UnconfiguredIntake) IntakeMovementFact(context.Context, *http.Request) (application.RecordMovementFactCommand, error) {
	return application.RecordMovementFactCommand{}, ErrAccessChannelNotConfigured
}

// 四个 admin 写面（票 07）同堵：写准入不另立形（ADR-0085 决定二），运营写决定在渠道就位前一样没有
// 可采信的身份，连命令都不构造。
func (UnconfiguredIntake) IntakeSegmentClosure(context.Context, *http.Request) (application.CloseFulfillmentSegmentCommand, error) {
	return application.CloseFulfillmentSegmentCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDispatchTask(context.Context, *http.Request) (application.OpenDispatchTaskCommand, error) {
	return application.OpenDispatchTaskCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeLoadAssignment(context.Context, *http.Request) (application.FormLoadAssignmentCommand, error) {
	return application.FormLoadAssignmentCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeParticipationTermination(context.Context, *http.Request) (ParticipationTermination, error) {
	return ParticipationTermination{}, ErrAccessChannelNotConfigured
}

// 凭证登记两口（label-channel/18）同堵：一份凭证登进去就会被收编执行器用来把外部轨迹认到某个
// 载运对象上，渠道未就位前没有可采信的登记方身份，连命令都不构造。
func (UnconfiguredIntake) IntakeCredentialRegistration(context.Context, *http.Request) (application.RegisterExternalCarrierCredentialCommand, error) {
	return application.RegisterExternalCarrierCredentialCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCredentialApplicabilityChange(context.Context, *http.Request) (application.ChangeCredentialApplicabilityCommand, error) {
	return application.ChangeCredentialApplicabilityCommand{}, ErrAccessChannelNotConfigured
}

// 有效时间规则登记口（label-channel/19）同堵：一版规则登进去就会让收编执行器替该源此后每一条素材形成
// 有效时间，渠道未就位前没有可采信的登记方身份，连命令都不构造。
func (UnconfiguredIntake) IntakeEffectiveTimeRuleRegistration(context.Context, *http.Request) (application.RegisterEffectiveTimeRuleCommand, error) {
	return application.RegisterEffectiveTimeRuleCommand{}, ErrAccessChannelNotConfigured
}
