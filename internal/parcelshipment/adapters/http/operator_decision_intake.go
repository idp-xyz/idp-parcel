package shipmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 操作者渠道在本包的四格答复（ADR-0100 决定四、ADR-0149 决定四），与 ErrAccessChannelNotConfigured 并列，
// 各对一种恢复动作（ADR-0029），writeIntakeProblem 逐格映射、不合并。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("parcel shipment http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户、没有这一种决定的授予，或指名的委托查不到（403）。
	// 查不到与没授予同答，是越权探针同答约束（ADR-0055 决定四、ADR-0151 决定二）。
	ErrOperatorNotGranted = errors.New("parcel shipment http: operator holds no grant for this decision")
	// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）——阶段治理去登记区间。
	ErrOutsideAdmissionScope = errors.New("parcel shipment http: production write is outside the admission scope")
	// ErrIdentityDependencyUnavailable：发行方公钥集、操作者册或准入范围读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("parcel shipment http: identity dependency unavailable")
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
	OperatorDecisionManualReviewCompletion OperatorDecision = "MANUAL_REVIEW_COMPLETION"
	OperatorDecisionActiveRejection        OperatorDecision = "ACTIVE_REJECTION"
	OperatorDecisionAuthorizedDisposition  OperatorDecision = "AUTHORIZED_DISPOSITION"
)

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与提交操作者的引用（发行方 + sub）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorAuthenticator 认证一次运营决定出示。失败只交回本包的格；从共享接入身份能力译过来的那一层在
// adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在委托侧复核完成、主动拒绝、授权处置三口的 Intake
// （ADR-0151 决定一、二）。受控关闭与重开不在这里：它们命令里的请求方格在操作者渠道上怎么形成，要先与 PS owner
// 对齐（票 operator-channel/15 做什么第 3 条），定之前那两口的 Intake 不换。
//
// 线格式是管理台今天送出的草案：复核完成与主动拒绝只有委托标识与理由，授权处置另带它审的那个提交版本与去向。
// 载荷只有内容、没有身份：来源身份由服务端按信封的租户与委托标识查出（ADR-0151 决定二「委托寻址」），复核人、
// 决定人、处置人取认证出的提交操作者；载荷里出现任何身份格即拒（封闭解码）。先认证、后解载荷。
type OperatorDecisionIntake struct {
	authenticator OperatorAuthenticator
	targets       ports.OperatorDecisionTargets
}

var (
	_ ManualReviewCompletionIntake = (*OperatorDecisionIntake)(nil)
	_ ActiveRejectionIntake        = (*OperatorDecisionIntake)(nil)
	_ AuthorizedDispositionIntake  = (*OperatorDecisionIntake)(nil)
)

func NewOperatorDecisionIntake(authenticator OperatorAuthenticator, targets ports.OperatorDecisionTargets) (*OperatorDecisionIntake, error) {
	if authenticator == nil || targets == nil {
		return nil, errors.New("parcel shipment http: operator decision intake needs an authenticator and a target lookup")
	}
	return &OperatorDecisionIntake{authenticator: authenticator, targets: targets}, nil
}

type decisionReasonPayload struct {
	ShipmentRequestID string `json:"shipmentRequestId"`
	Reason            string `json:"reason"`
}

type authorizedDispositionPayload struct {
	ShipmentRequestID   string `json:"shipmentRequestId"`
	SubmissionVersionID string `json:"submissionVersionId"`
	Choice              string `json:"choice"`
	Reason              string `json:"reason"`
}

// IntakeManualReviewCompletion 译复核完成（`/shipment-requests/manual-review-completions`）。命令里没有原因格，
// 管理台送来的复核理由就是复核留痕的证据引用；提交版本取服务端当前版本——管理台这一口的草案不带版本。
func (intake *OperatorDecisionIntake) IntakeManualReviewCompletion(ctx context.Context, request *http.Request) (application.CompleteManualReviewCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, bearerToken(request), OperatorDecisionManualReviewCompletion)
	if err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	var payload decisionReasonPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	requestID, target, err := intake.locate(ctx, operator.Tenant, payload.ShipmentRequestID)
	if err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	reviewer, err := domain.NewReviewerReference(operator.Operator)
	if err != nil {
		return application.CompleteManualReviewCommand{}, fmt.Errorf("parcel shipment http: reviewer: %w", err)
	}
	evidence, err := domain.NewReviewEvidenceReference(payload.Reason)
	if err != nil {
		return application.CompleteManualReviewCommand{}, fmt.Errorf("%w: reason: %v", ErrMalformedRequest, err)
	}
	return application.CompleteManualReviewCommand{
		Identity:          target.Identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: target.CurrentVersion,
		Reviewer:          reviewer,
		Evidence:          evidence,
	}, nil
}

// IntakeActiveRejection 译主动拒绝（`/shipment-requests/rejections`）。理由进结构化原因格；证据格取认证出的操作者
// ——登录操作人可以作为操作证据（PS CONTEXT）。提交版本取服务端当前版本，理由同复核完成。
func (intake *OperatorDecisionIntake) IntakeActiveRejection(ctx context.Context, request *http.Request) (application.RejectShipmentRequestCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, bearerToken(request), OperatorDecisionActiveRejection)
	if err != nil {
		return application.RejectShipmentRequestCommand{}, err
	}
	var payload decisionReasonPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.RejectShipmentRequestCommand{}, err
	}
	requestID, target, err := intake.locate(ctx, operator.Tenant, payload.ShipmentRequestID)
	if err != nil {
		return application.RejectShipmentRequestCommand{}, err
	}
	decider, err := domain.NewDeciderReference(operator.Operator)
	if err != nil {
		return application.RejectShipmentRequestCommand{}, fmt.Errorf("parcel shipment http: decider: %w", err)
	}
	reason, err := domain.NewRejectionReasonReference(payload.Reason)
	if err != nil {
		return application.RejectShipmentRequestCommand{}, fmt.Errorf("%w: reason: %v", ErrMalformedRequest, err)
	}
	evidence, err := domain.NewRejectionEvidenceReference(operatorEvidence(operator))
	if err != nil {
		return application.RejectShipmentRequestCommand{}, fmt.Errorf("parcel shipment http: evidence: %w", err)
	}
	return application.RejectShipmentRequestCommand{
		Identity:          target.Identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: target.CurrentVersion,
		Decider:           decider,
		Reason:            reason,
		Evidence:          evidence,
	}, nil
}

// IntakeAuthorizedDisposition 译授权处置（`/shipment-requests/authorized-dispositions`）。提交版本取草案送来的那一份：
// 处置挂在版本的判断任务上，不指名版本就分不清签给了谁（管理台草案原注）；证据格同主动拒绝。
func (intake *OperatorDecisionIntake) IntakeAuthorizedDisposition(ctx context.Context, request *http.Request) (application.DisposeShipmentRequestCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, bearerToken(request), OperatorDecisionAuthorizedDisposition)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, err
	}
	var payload authorizedDispositionPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.DisposeShipmentRequestCommand{}, err
	}
	choice, err := parseDispositionChoice(payload.Choice)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, err
	}
	version, err := domain.NewSubmissionVersionID(payload.SubmissionVersionID)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, fmt.Errorf("%w: submissionVersionId: %v", ErrMalformedRequest, err)
	}
	requestID, target, err := intake.locate(ctx, operator.Tenant, payload.ShipmentRequestID)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, err
	}
	disposer, err := domain.NewDisposerReference(operator.Operator)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, fmt.Errorf("parcel shipment http: disposer: %w", err)
	}
	reason, err := domain.NewDispositionReasonReference(payload.Reason)
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, fmt.Errorf("%w: reason: %v", ErrMalformedRequest, err)
	}
	evidence, err := domain.NewDispositionEvidenceReference(operatorEvidence(operator))
	if err != nil {
		return application.DisposeShipmentRequestCommand{}, fmt.Errorf("parcel shipment http: evidence: %w", err)
	}
	return application.DisposeShipmentRequestCommand{
		Identity:          target.Identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: version,
		Disposer:          disposer,
		Choice:            choice,
		Reason:            reason,
		Evidence:          evidence,
	}, nil
}

// locate 按信封的租户与请求指名的委托查出来源身份。查不到答 ErrOperatorNotGranted，与越权探针同答。
func (intake *OperatorDecisionIntake) locate(ctx context.Context, tenant domain.TenantID, rawID string) (domain.ShipmentRequestID, ports.OperatorDecisionTarget, error) {
	requestID, err := domain.NewShipmentRequestID(rawID)
	if err != nil {
		return domain.ShipmentRequestID{}, ports.OperatorDecisionTarget{}, fmt.Errorf("%w: shipmentRequestId: %v", ErrMalformedRequest, err)
	}
	target, found, err := intake.targets.FindOperatorDecisionTarget(ctx, tenant, requestID)
	if err != nil {
		return domain.ShipmentRequestID{}, ports.OperatorDecisionTarget{}, fmt.Errorf("parcel shipment http: locate shipment request: %w", err)
	}
	if !found {
		return domain.ShipmentRequestID{}, ports.OperatorDecisionTarget{}, ErrOperatorNotGranted
	}
	return requestID, target, nil
}

// operatorEvidence 是认证出的操作者作为操作证据时的引用。
func operatorEvidence(operator OperatorIdentity) string { return "OPERATOR/" + operator.Operator }

func parseDispositionChoice(raw string) (domain.AuthorizedDispositionChoice, error) {
	for _, choice := range []domain.AuthorizedDispositionChoice{domain.DisposeByRejection, domain.DisposeByCustomerSupplement} {
		if choice.String() == raw {
			return choice, nil
		}
	}
	return domain.AuthorizedDispositionChoiceInvalid, fmt.Errorf("%w: choice %q is not a disposition choice", ErrMalformedRequest, raw)
}

// decodeDecisionPayload 封闭解码：未知字段（包括任何身份格）与尾随内容都答坏报文。
func decodeDecisionPayload(body io.Reader, target any) error {
	if body == nil {
		return fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return fmt.Errorf("%w: trailing content", ErrMalformedRequest)
	}
	return nil
}

// bearerToken 取 `Authorization: Bearer …` 的令牌；缺席或不是 Bearer 即空串，交认证方答「令牌缺失」。
func bearerToken(request *http.Request) string {
	scheme, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
