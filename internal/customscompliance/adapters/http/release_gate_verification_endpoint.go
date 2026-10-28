package customshttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是放行门禁核对口（`POST /customs-release-gate-verifications`，UC-CC-009，票 launch-wiring/03）：有权角色对（申报范围，拟执行动作，
// 监管边界）请求一次门禁核对，折出结论就落册并发意图；折不出结论答未决并指名停在哪一处，不落册（code/34）。

// OperatorDecisionReleaseGateVerification 字面与操作者册的决定种类相同。
const OperatorDecisionReleaseGateVerification OperatorDecision = "RELEASE_GATE_VERIFICATION"

// ReleaseGateVerifier 是核对口转交的编排（application.VerifyReleaseGateHandler）。
type ReleaseGateVerifier interface {
	Handle(ctx context.Context, command application.VerifyReleaseGateCommand) (application.VerifyReleaseGateResult, error)
}

// ReleaseGateVerificationIntake 译门禁核对请求。
type ReleaseGateVerificationIntake interface {
	IntakeReleaseGateVerification(ctx context.Context, request *http.Request) (application.VerifyReleaseGateCommand, error)
}

var _ ReleaseGateVerificationIntake = (*OperatorDecisionIntake)(nil)

func NewVerifyReleaseGateEndpoint(intake ReleaseGateVerificationIntake, handler ReleaseGateVerifier) http.Handler {
	return decisionEndpoint(intake.IntakeReleaseGateVerification, handler.Handle, writeReleaseGateVerification)
}

// ReleaseGateVerificationPayload 是核对口的线格式：申报范围、拟执行动作与监管边界——三者共同构成判断身份。租户取自认证信封。
type ReleaseGateVerificationPayload struct {
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Boundary string `json:"boundary"`
}

// IntakeReleaseGateVerification 按放行门禁核对这一种决定认证。动作给了却不在封闭四值里是坏报文；空着的格交零值，留给编排答`未受理`。
func (intake *OperatorDecisionIntake) IntakeReleaseGateVerification(ctx context.Context, request *http.Request) (application.VerifyReleaseGateCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionReleaseGateVerification)
	if err != nil {
		return application.VerifyReleaseGateCommand{}, err
	}
	var payload ReleaseGateVerificationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.VerifyReleaseGateCommand{}, err
	}
	command := application.VerifyReleaseGateCommand{
		TenantID: operator.Tenant,
		Scope:    optionalReference(payload.Scope, domain.NewDecisionScopeReference),
		Boundary: optionalReference(payload.Boundary, domain.NewCustomsProcedureReference),
	}
	if raw := strings.TrimSpace(payload.Action); raw != "" {
		action, ok := domain.ParseGuardedAction(raw)
		if !ok {
			return application.VerifyReleaseGateCommand{}, fmt.Errorf("%w: action %q is not one of the guarded actions", ErrMalformedRequest, raw)
		}
		command.Action = action
	}
	return command, nil
}

// releaseGateDutyPaymentResponse 是「税费付款」那一道的读数：满足态与它所凭的付款核对三态原值，不合成布尔。
type releaseGateDutyPaymentResponse struct {
	State    string `json:"state"`
	Coverage string `json:"coverage"`
	Delta    string `json:"delta"`
	Validity string `json:"validity"`
}

// releaseGateVerificationResponse 是核对口的封闭响应形状。类型上没有放行字段：门禁满足不生成放行（放行结果仍由外部事实接收）。
type releaseGateVerificationResponse struct {
	Outcome          string                          `json:"outcome"`
	UndecidedReason  string                          `json:"undecidedReason,omitempty"`
	Scope            string                          `json:"scope,omitempty"`
	Action           string                          `json:"action,omitempty"`
	Boundary         string                          `json:"boundary,omitempty"`
	Conclusion       string                          `json:"conclusion,omitempty"`
	Preconditions    []string                        `json:"preconditions,omitempty"`
	DutyPayment      *releaseGateDutyPaymentResponse `json:"dutyPayment,omitempty"`
	VerifiedAt       string                          `json:"verifiedAt,omitempty"`
	HandoffReference string                          `json:"handoffReference,omitempty"`
}

// writeReleaseGateVerification 逐名转写。真落了册才 201；重放、未受理与未决都是形成了的业务答案，200——未决的原因分格，
// 「还判不了」与「读不动」靠 undecidedReason 分开，不折成一格。
func writeReleaseGateVerification(response http.ResponseWriter, result application.VerifyReleaseGateResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := releaseGateVerificationResponse{
		Outcome:          outcome,
		UndecidedReason:  result.UndecidedReason().String(),
		HandoffReference: result.HandoffReference(),
	}
	if gate, present := result.Gate(); present {
		body.Scope = gate.Scope().String()
		body.Action = gate.Action().String()
		body.Boundary = gate.Boundary().String()
		body.Conclusion = gate.Conclusion().String()
		body.VerifiedAt = gate.VerifiedAt().UTC().Format(time.RFC3339Nano)
		for _, precondition := range gate.Preconditions() {
			body.Preconditions = append(body.Preconditions, precondition.String())
		}
		if reading, applied := gate.DutyPayment(); applied {
			body.DutyPayment = &releaseGateDutyPaymentResponse{
				State:    reading.State.String(),
				Coverage: reading.Coverage.String(),
				Delta:    reading.Delta.String(),
				Validity: reading.Validity.String(),
			}
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.GateVerificationRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
