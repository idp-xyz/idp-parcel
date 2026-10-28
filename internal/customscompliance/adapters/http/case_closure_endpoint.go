package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是关务案件关闭口（`POST /customs-case-closures`，UC-CC-010 的关闭入口，票 launch-wiring/02）：有权角色对进行中的案件请求关闭，
// 关闭核对逐项盘点义务，任一未解决或冲突项都阻止关闭并列出各项（code/03）。

// OperatorDecisionCustomsCaseClosure 字面与操作者册的决定种类相同。
const OperatorDecisionCustomsCaseClosure OperatorDecision = "CUSTOMS_CASE_CLOSURE"

// CaseCloser 是关闭口转交的编排（application.CloseCustomsCaseHandler）。
type CaseCloser interface {
	Handle(ctx context.Context, command application.CloseCustomsCaseCommand) (application.CloseCustomsCaseResult, error)
}

// CaseClosureIntake 译关闭请求。
type CaseClosureIntake interface {
	IntakeCaseClosure(ctx context.Context, request *http.Request) (application.CloseCustomsCaseCommand, error)
}

var _ CaseClosureIntake = (*OperatorDecisionIntake)(nil)

func NewCloseCaseEndpoint(intake CaseClosureIntake, handler CaseCloser) http.Handler {
	return decisionEndpoint(intake.IntakeCaseClosure, handler.Handle, writeCaseClosure)
}

// CaseClosurePayload 是关闭口的线格式：案件与业务截点。决定人是认证出的操作者，载荷里没有这一格；业务截点由请求方给——义务盘点按它
// 取当时适用的义务，产品不拿处理时刻代它。
type CaseClosurePayload struct {
	CaseID   string `json:"caseId"`
	CutoffAt string `json:"cutoffAt"`
}

// IntakeCaseClosure 按关务案件关闭这一种决定认证。空着的格交零值，留给编排答`未受理`；截点给了却不是 RFC 3339 是坏报文。
func (intake *OperatorDecisionIntake) IntakeCaseClosure(ctx context.Context, request *http.Request) (application.CloseCustomsCaseCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionCustomsCaseClosure)
	if err != nil {
		return application.CloseCustomsCaseCommand{}, err
	}
	var payload CaseClosurePayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CloseCustomsCaseCommand{}, err
	}
	cutoff, err := optionalInstant(payload.CutoffAt, "cutoffAt")
	if err != nil {
		return application.CloseCustomsCaseCommand{}, err
	}
	return application.CloseCustomsCaseCommand{
		TenantID:  operator.Tenant,
		CaseRef:   optionalReference(payload.CaseID, domain.NewCustomsCaseID),
		CutoffAt:  cutoff,
		DecidedBy: operator.Operator,
	}, nil
}

// closureItemResponse 是一项关闭依据项的线格式。状态取领域原名；冲突项带冲突依据，承接项带接收责任方。
type closureItemResponse struct {
	Obligation    string   `json:"obligation"`
	Scope         string   `json:"scope"`
	State         string   `json:"state"`
	Basis         string   `json:"basis"`
	HandedTo      string   `json:"handedTo,omitempty"`
	ConflictsWith []string `json:"conflictsWith,omitempty"`
}

// caseClosureResponse 是关闭口的封闭响应形状。被阻时 blockingItems 列出各未解决与冲突项——恢复动作是逐项处置，不是重试。
type caseClosureResponse struct {
	Outcome          string                `json:"outcome"`
	CaseID           string                `json:"caseId,omitempty"`
	ClosedAt         string                `json:"closedAt,omitempty"`
	DecidedBy        string                `json:"decidedBy,omitempty"`
	BlockingItems    []closureItemResponse `json:"blockingItems,omitempty"`
	UndecidedReason  string                `json:"undecidedReason,omitempty"`
	HandoffReference string                `json:"handoffReference,omitempty"`
}

// writeCaseClosure 逐名转写。真关了才 201；已关重放、被阻、未受理与未决都是形成了的业务答案，200（被阻是业务负向，不是失败）。
func writeCaseClosure(response http.ResponseWriter, result application.CloseCustomsCaseResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := caseClosureResponse{
		Outcome:          outcome,
		UndecidedReason:  result.UndecidedReason().String(),
		HandoffReference: result.HandoffReference(),
	}
	if closure, present := result.Closure(); present {
		body.CaseID = closure.CaseRef().String()
		body.ClosedAt = closure.ClosedAt().UTC().Format(time.RFC3339Nano)
		body.DecidedBy = closure.DecidedBy()
	}
	for _, item := range result.Unresolved() {
		body.BlockingItems = append(body.BlockingItems, closureItemResponse{
			Obligation:    item.Obligation,
			Scope:         item.Scope,
			State:         item.State.String(),
			Basis:         item.Basis,
			HandedTo:      item.HandedTo,
			ConflictsWith: item.ConflictsWith,
		})
	}
	status := http.StatusOK
	if result.Outcome() == application.CaseClosed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
