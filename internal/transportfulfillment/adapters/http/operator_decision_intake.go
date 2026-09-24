package tfhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 操作者渠道在本包的四格答复（ADR-0100 决定四、ADR-0149 决定四），与 ErrAccessChannelNotConfigured 并列，
// 各对一种恢复动作（ADR-0029），commandEndpoint 逐格映射、不合并。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("transport fulfillment http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有这一种决定的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("transport fulfillment http: operator holds no grant for this decision")
	// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）——阶段治理去登记区间。
	ErrOutsideAdmissionScope = errors.New("transport fulfillment http: production write is outside the admission scope")
	// ErrIdentityDependencyUnavailable：发行方公钥集、操作者册或准入范围读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("transport fulfillment http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeOutsideAdmissionScope         = "OUTSIDE_ADMISSION_SCOPE"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// OperatorDecision 是本包各口对应的运营决定种类（ADR-0151 决定一），字面与操作者册的决定种类相同。
type OperatorDecision string

const (
	OperatorDecisionSegmentClosure                      OperatorDecision = "SEGMENT_CLOSURE"
	OperatorDecisionDispatchTaskRegistration            OperatorDecision = "DISPATCH_TASK_REGISTRATION"
	OperatorDecisionLoadAssignment                      OperatorDecision = "LOAD_ASSIGNMENT"
	OperatorDecisionParticipationTermination            OperatorDecision = "PARTICIPATION_TERMINATION"
	OperatorDecisionEffectiveTimeJudgment               OperatorDecision = "EFFECTIVE_TIME_JUDGMENT"
	OperatorDecisionCarrierFirstEffectivePickupJudgment OperatorDecision = "CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT"
)

// OperatorAuthenticator 认证一次运营决定出示，交回出示者的租户。失败只交回本包的格：ErrAccessChannelNotConfigured、
// ErrOperatorCredentialRejected、ErrOperatorNotGranted、ErrOutsideAdmissionScope、ErrIdentityDependencyUnavailable；
// 从共享接入身份能力译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (domain.TenantID, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在本包各口的 Intake（ADR-0151 决定一）。
//
// 它与 IsolatedCommandIntake 走同一套线格式：各口文件里的 `…Payload` 与它的 Command，载荷只有内容、没有身份；两者只差
// 租户从哪来——隔离形态是装配点注入的合成值，这里是认证结果。先认证、后解载荷：令牌不过的调用方看不到载荷校验的
// 结果。每口按自己的决定种类认证，持有关段授予的人建不了派送任务。
type OperatorDecisionIntake struct {
	authenticator OperatorAuthenticator
}

var (
	_ SegmentClosureIntake           = (*OperatorDecisionIntake)(nil)
	_ DispatchTaskIntake             = (*OperatorDecisionIntake)(nil)
	_ EffectiveTimeJudgmentIntake    = (*OperatorDecisionIntake)(nil)
	_ CarrierPickupJudgmentIntake    = (*OperatorDecisionIntake)(nil)
	_ LoadAssignmentIntake           = (*OperatorDecisionIntake)(nil)
	_ ParticipationTerminationIntake = (*OperatorDecisionIntake)(nil)
)

func NewOperatorDecisionIntake(authenticator OperatorAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("transport fulfillment http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

// IntakeSegmentClosure 译关段声明（`/transport-fulfillment-segment-closures`）。
func (intake *OperatorDecisionIntake) IntakeSegmentClosure(ctx context.Context, request *http.Request) (application.CloseFulfillmentSegmentCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionSegmentClosure)
	if err != nil {
		return application.CloseFulfillmentSegmentCommand{}, err
	}
	var payload SegmentClosurePayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CloseFulfillmentSegmentCommand{}, err
	}
	return payload.Command(tenant)
}

// IntakeDispatchTask 译授权角色建立派送任务（`/transport-fulfillment-dispatch-task-registrations`）。
func (intake *OperatorDecisionIntake) IntakeDispatchTask(ctx context.Context, request *http.Request) (application.OpenDispatchTaskCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionDispatchTaskRegistration)
	if err != nil {
		return application.OpenDispatchTaskCommand{}, err
	}
	var payload DispatchTaskPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.OpenDispatchTaskCommand{}, err
	}
	return payload.Command(tenant)
}

// IntakeEffectiveTimeJudgment 译外部承运轨迹事实有效时间的显式判断（`/transport-fulfillment-effective-time-judgments`）。
func (intake *OperatorDecisionIntake) IntakeEffectiveTimeJudgment(ctx context.Context, request *http.Request) (application.JudgeEffectiveTimeCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionEffectiveTimeJudgment)
	if err != nil {
		return application.JudgeEffectiveTimeCommand{}, err
	}
	var payload EffectiveTimeJudgmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.JudgeEffectiveTimeCommand{}, err
	}
	return payload.Command(tenant)
}

// IntakeCarrierPickupJudgment 译实际承运商首次有效收寄的显式判断（`/transport-fulfillment-carrier-first-effective-pickup-judgments`）。
func (intake *OperatorDecisionIntake) IntakeCarrierPickupJudgment(ctx context.Context, request *http.Request) (application.JudgeCarrierFirstEffectivePickupCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionCarrierFirstEffectivePickupJudgment)
	if err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, err
	}
	var payload CarrierPickupJudgmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, err
	}
	return payload.Command(tenant)
}

// IntakeLoadAssignment 译装载分配登记（`/transport-fulfillment-load-assignment-registrations`）。
func (intake *OperatorDecisionIntake) IntakeLoadAssignment(ctx context.Context, request *http.Request) (application.FormLoadAssignmentCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionLoadAssignment)
	if err != nil {
		return application.FormLoadAssignmentCommand{}, err
	}
	var payload LoadAssignmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.FormLoadAssignmentCommand{}, err
	}
	return payload.Command(tenant)
}

// IntakeParticipationTermination 译一次明确终止（`/transport-fulfillment-participation-terminations`）。
func (intake *OperatorDecisionIntake) IntakeParticipationTermination(ctx context.Context, request *http.Request) (ParticipationTermination, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionParticipationTermination)
	if err != nil {
		return ParticipationTermination{}, err
	}
	var payload ParticipationTerminationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return ParticipationTermination{}, err
	}
	return payload.Termination(tenant)
}

// tenantFor 交回出示者在这一种决定上的租户。租户只取自认证结果（ADR-0003），载荷里不收租户。
func (intake *OperatorDecisionIntake) tenantFor(ctx context.Context, request *http.Request, decision OperatorDecision) (domain.TenantID, error) {
	return intake.authenticator.AuthenticateOperatorDecision(ctx, bearerToken(request), decision)
}

// bearerToken 取 `Authorization: Bearer …` 的令牌；缺席或不是 Bearer 即空串，交核验方答「令牌缺失」。
func bearerToken(request *http.Request) string {
	scheme, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
