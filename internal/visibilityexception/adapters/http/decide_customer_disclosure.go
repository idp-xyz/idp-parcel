package visibilityhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是客户可见异常披露的两个运营决定口（UC-VE-006 步 3–5）：对一条异常信号发起披露判断，以及确认发布一份停在
// `待授权`的决定。两口的载荷相同——指名哪件包裹上的哪一类信号；结论、客户与披露内容都不在载荷里：结论由披露规则给，
// 客户由包裹的当前已接受委托反查，内容取规则的快照。

// CustomerDisclosureDecider 是两口转交的编排（application.DecideDisclosureHandler）。
type CustomerDisclosureDecider interface {
	Handle(ctx context.Context, command application.DecideDisclosureCommand) (application.DecideDisclosureResult, error)
	Authorize(ctx context.Context, command application.AuthorizeDisclosureCommand) (application.DecideDisclosureResult, error)
}

type CustomerDisclosureIntake interface {
	IntakeCustomerDisclosureDecision(ctx context.Context, request *http.Request) (application.DecideDisclosureCommand, error)
	IntakeCustomerDisclosureAuthorization(ctx context.Context, request *http.Request) (application.AuthorizeDisclosureCommand, error)
}

var _ CustomerDisclosureIntake = (*OperatorDecisionIntake)(nil)

func NewDecideCustomerDisclosureEndpoint(intake CustomerDisclosureIntake, decider CustomerDisclosureDecider) http.Handler {
	return customerDisclosureEndpoint(intake.IntakeCustomerDisclosureDecision, decider.Handle)
}

func NewAuthorizeCustomerDisclosureEndpoint(intake CustomerDisclosureIntake, decider CustomerDisclosureDecider) http.Handler {
	return customerDisclosureEndpoint(intake.IntakeCustomerDisclosureAuthorization, decider.Authorize)
}

// CustomerDisclosurePayload 是两口共用的线格式。
type CustomerDisclosurePayload struct {
	Parcel     string `json:"parcel"`
	SignalKind string `json:"signalKind"`
}

func (intake *OperatorDecisionIntake) customerDisclosureTarget(
	ctx context.Context,
	request *http.Request,
	decision OperatorDecision,
) (OperatorIdentity, domain.TrackedParcelReference, domain.ExceptionSignalKindReference, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), decision)
	if err != nil {
		return OperatorIdentity{}, domain.TrackedParcelReference{}, domain.ExceptionSignalKindReference{}, err
	}
	var payload CustomerDisclosurePayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return OperatorIdentity{}, domain.TrackedParcelReference{}, domain.ExceptionSignalKindReference{}, err
	}
	parcel, err := domain.NewTrackedParcelReference(payload.Parcel)
	if err != nil {
		return OperatorIdentity{}, domain.TrackedParcelReference{}, domain.ExceptionSignalKindReference{}, fmt.Errorf("%w: parcel: %v", ErrMalformedRequest, err)
	}
	kind, err := domain.NewExceptionSignalKindReference(payload.SignalKind)
	if err != nil {
		return OperatorIdentity{}, domain.TrackedParcelReference{}, domain.ExceptionSignalKindReference{}, fmt.Errorf("%w: signalKind: %v", ErrMalformedRequest, err)
	}
	return operator, parcel, kind, nil
}

// IntakeCustomerDisclosureDecision 按披露判断这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCustomerDisclosureDecision(ctx context.Context, request *http.Request) (application.DecideDisclosureCommand, error) {
	operator, parcel, kind, err := intake.customerDisclosureTarget(ctx, request, OperatorDecisionCustomerDisclosureDecision)
	if err != nil {
		return application.DecideDisclosureCommand{}, err
	}
	return application.DecideDisclosureCommand{TenantID: operator.Tenant, Parcel: parcel, Kind: kind, Operator: operator.Operator}, nil
}

// IntakeCustomerDisclosureAuthorization 按披露授权这一种决定认证。它与披露判断分开授予。
func (intake *OperatorDecisionIntake) IntakeCustomerDisclosureAuthorization(ctx context.Context, request *http.Request) (application.AuthorizeDisclosureCommand, error) {
	operator, parcel, kind, err := intake.customerDisclosureTarget(ctx, request, OperatorDecisionCustomerDisclosureAuthorization)
	if err != nil {
		return application.AuthorizeDisclosureCommand{}, err
	}
	return application.AuthorizeDisclosureCommand{TenantID: operator.Tenant, Parcel: parcel, Kind: kind, Operator: operator.Operator}, nil
}

func customerDisclosureEndpoint[C any](
	intake func(context.Context, *http.Request) (C, error),
	handle func(context.Context, C) (application.DecideDisclosureResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeCaseDecisionIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCustomerDisclosureDecision(response, result)
	})
}

// customerDisclosureResponse 是两口的封闭响应形状。决定各格只在决定在场时交回；reason 只在未决时交回。
type customerDisclosureResponse struct {
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	Customer   string `json:"customer,omitempty"`
	Policy     string `json:"policy,omitempty"`
	Content    string `json:"content,omitempty"`
	DecidedAt  string `json:"decidedAt,omitempty"`
}

// writeCustomerDisclosureDecision 逐名转写。未决分两种：披露规则未配置与客户归属不可确定是业务答案——前者等租户登记
// 规则，后者等包裹有已接受的委托，重试都不会变——答 200 并带原因；其余未决都是某一只依赖读写不动，是没形成答案，
// 答 5xx。决定真落了（本轮判出或本轮确认）才 201。
func writeCustomerDisclosureDecision(response http.ResponseWriter, result application.DecideDisclosureResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := customerDisclosureResponse{Outcome: outcome}
	if result.Outcome() == application.DecideDisclosureUndecided {
		switch result.UndecidedReason() {
		case application.DisclosureRuleNotConfigured, application.DisclosureCustomerAccountUndeterminable:
			body.Reason = result.UndecidedReason().String()
		default:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
	}
	if decision, present := result.Decision(); present {
		body.Conclusion = decision.Conclusion().String()
		body.Customer = decision.Customer().String()
		body.Policy = decision.Policy().String()
		body.DecidedAt = decision.DecidedAt().UTC().Format("2006-01-02T15:04:05.999999Z07:00")
		if content, disclosed := decision.Content(); disclosed {
			body.Content = content.String()
		}
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.DisclosureDecided, application.DisclosureAuthorized:
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
