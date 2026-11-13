package visibilityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// OperatorDecision 是本包各口对应的运营决定种类，字面与操作者册的决定种类相同。三口各一种：能接单的不因此能下关闭结论，
// 能关闭的不因此能把一份案件并进另一份。
type OperatorDecision string

const (
	OperatorDecisionExceptionCaseTakeUp  OperatorDecision = "EXCEPTION_CASE_TAKE_UP"
	OperatorDecisionExceptionCaseClosure OperatorDecision = "EXCEPTION_CASE_CLOSURE"
	OperatorDecisionExceptionCaseMerge   OperatorDecision = "EXCEPTION_CASE_MERGE"
)

// 证据的三种决定：登记、评价、准备对外披露版本。同样各按自己那一种认证。
const (
	OperatorDecisionEvidenceSubmission            OperatorDecision = "EVIDENCE_SUBMISSION"
	OperatorDecisionEvidenceAppraisal             OperatorDecision = "EVIDENCE_APPRAISAL"
	OperatorDecisionEvidenceDisclosurePreparation OperatorDecision = "EVIDENCE_DISCLOSURE_PREPARATION"
	// 客户可见异常披露上的两种：发起披露判断、确认发布一份待授权的决定。
	OperatorDecisionCustomerDisclosureDecision      OperatorDecision = "CUSTOMER_DISCLOSURE_DECISION"
	OperatorDecisionCustomerDisclosureAuthorization OperatorDecision = "CUSTOMER_DISCLOSURE_AUTHORIZATION"
	// 把一份已成立的披露决定通知客户（首发与重发）。
	OperatorDecisionCustomerExceptionNotification OperatorDecision = "CUSTOMER_EXCEPTION_NOTIFICATION"
)

// maxDecisionBytes 是一份运营决定载荷的上限：三口的载荷都只有几个标识和一段结论。
const maxDecisionBytes = 64 << 10

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与作出决定的操作者（「发行方#sub」）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorDecisionAuthenticator 认证一次运营决定出示，交回出示者的身份。失败只交回本包的格：ErrAccessChannelNotConfigured、
// ErrOperatorCredentialRejected、ErrOperatorNotGranted、ErrOutsideAdmissionScope、ErrIdentityDependencyUnavailable；从共享
// 接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorDecisionAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在本包各口的 Intake。租户与决定人只取认证结果；载荷是封闭的，
// 多带任何一格即拒——自报租户或操作者的请求到不了编排。
type OperatorDecisionIntake struct {
	authenticator OperatorDecisionAuthenticator
}

func NewOperatorDecisionIntake(authenticator OperatorDecisionAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("visibility http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

// CaseTakeUpPayload、CaseClosurePayload 与 CaseMergePayload 是三口的线格式。没有时刻一格：接单、关闭与归并的时刻是服务端
// 落这次决定的时刻，不收补报。
type CaseTakeUpPayload struct {
	CaseID string `json:"caseId"`
}

type CaseClosurePayload struct {
	CaseID     string `json:"caseId"`
	Conclusion string `json:"conclusion"`
}

type CaseMergePayload struct {
	CaseID     string `json:"caseId"`
	MainCaseID string `json:"mainCaseId"`
}

// IntakeCaseTakeUp 按案件接单这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCaseTakeUp(ctx context.Context, request *http.Request) (application.TakeUpCaseCommand, error) {
	none := application.TakeUpCaseCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExceptionCaseTakeUp)
	if err != nil {
		return none, err
	}
	var payload CaseTakeUpPayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := caseIDOf(payload.CaseID, "caseId")
	if err != nil {
		return none, err
	}
	return application.TakeUpCaseCommand{TenantID: operator.Tenant, Case: id, Operator: operator.Operator}, nil
}

// IntakeCaseClosure 按案件关闭这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCaseClosure(ctx context.Context, request *http.Request) (application.CloseCaseCommand, error) {
	none := application.CloseCaseCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExceptionCaseClosure)
	if err != nil {
		return none, err
	}
	var payload CaseClosurePayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := caseIDOf(payload.CaseID, "caseId")
	if err != nil {
		return none, err
	}
	return application.CloseCaseCommand{TenantID: operator.Tenant, Case: id, Conclusion: payload.Conclusion, Operator: operator.Operator}, nil
}

// IntakeCaseMerge 按案件归并这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCaseMerge(ctx context.Context, request *http.Request) (application.MergeCaseCommand, error) {
	none := application.MergeCaseCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExceptionCaseMerge)
	if err != nil {
		return none, err
	}
	var payload CaseMergePayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := caseIDOf(payload.CaseID, "caseId")
	if err != nil {
		return none, err
	}
	main, err := caseIDOf(payload.MainCaseID, "mainCaseId")
	if err != nil {
		return none, err
	}
	return application.MergeCaseCommand{TenantID: operator.Tenant, Case: id, Main: main, Operator: operator.Operator}, nil
}

func caseIDOf(raw, field string) (domain.CaseID, error) {
	id, err := domain.NewCaseID(raw)
	if err != nil {
		return domain.CaseID{}, fmt.Errorf("%w: %s: %v", ErrMalformedRequest, field, err)
	}
	return id, nil
}

// decodeClosedDecision 收一份封闭载荷：只许一个 JSON 对象、不认识的格即拒、其后无尾随内容。
func decodeClosedDecision(body io.Reader, target any) error {
	if body == nil {
		return fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	decoder := json.NewDecoder(io.LimitReader(body, maxDecisionBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}
