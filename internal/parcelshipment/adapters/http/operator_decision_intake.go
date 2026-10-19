package shipmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
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
	OperatorDecisionControlledClosure      OperatorDecision = "CONTROLLED_CLOSURE"
	OperatorDecisionControlledReopening    OperatorDecision = "CONTROLLED_REOPENING"
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

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在委托侧各口的 Intake（ADR-0151 决定一、二）：复核完成、主动拒绝、
// 授权处置按委托寻址，受控关闭与重开按包裹寻址（ADR-0183）。
//
// 线格式是管理台今天送出的草案：主动拒绝只有委托标识与理由；复核完成与授权处置另带它审的那个提交版本，授权处置再带去向。
// 载荷只有内容、没有身份：来源身份由服务端按信封的租户与委托标识查出（ADR-0151 决定二「委托寻址」），复核人、
// 决定人、处置人取认证出的提交操作者；载荷里出现任何身份格即拒（封闭解码）。先认证、后解载荷。受控关闭与重开改按
// 包裹寻址、请求方与货主账户取那份委托的客户账户（ADR-0183），提交操作者只进操作证据那一格（ADR-0187 决定三）。
type OperatorDecisionIntake struct {
	authenticator OperatorAuthenticator
	targets       ports.OperatorDecisionTargets
	parcels       ports.CurrentAcceptedParcelTargetView
}

var (
	_ ManualReviewCompletionIntake   = (*OperatorDecisionIntake)(nil)
	_ ActiveRejectionIntake          = (*OperatorDecisionIntake)(nil)
	_ AuthorizedDispositionIntake    = (*OperatorDecisionIntake)(nil)
	_ ContinuedAttemptDecisionIntake = (*OperatorDecisionIntake)(nil)
)

func NewOperatorDecisionIntake(
	authenticator OperatorAuthenticator,
	targets ports.OperatorDecisionTargets,
	parcels ports.CurrentAcceptedParcelTargetView,
) (*OperatorDecisionIntake, error) {
	if authenticator == nil || targets == nil || parcels == nil {
		return nil, errors.New("parcel shipment http: operator decision intake needs an authenticator, a target lookup and a parcel lookup")
	}
	return &OperatorDecisionIntake{authenticator: authenticator, targets: targets, parcels: parcels}, nil
}

// controlledClosurePayload 是受控关闭的线格式（ADR-0183）：只指名包裹，不收委托标识与请求方。责任主体只在运营操作与
// 外部限制两种来源上由载荷给；货主指令的主体就是请求方，由服务端取。
//
// effectiveAt 由调用方在形成请求时固定、重投原样带上：同一次请求的重投靠它与其余几格认出（ADR-0183 复核记录二），
// 重提交时重算生效时间就是另一次请求。
type controlledClosurePayload struct {
	Parcel                string `json:"parcel"`
	Reason                string `json:"reason"`
	EffectiveAt           string `json:"effectiveAt"`
	ResponsibilitySource  string `json:"responsibilitySource"`
	ResponsibilitySubject string `json:"responsibilitySubject"`
	// ShipperInstructionEvidence 是货主那份指令的证据引用，由操作者如实记下；只在货主指令的关闭上带、且必带（ADR-0187
	// 决定五）。
	ShipperInstructionEvidence string `json:"shipperInstructionEvidence"`
}

// reopeningPayload 是重开的线格式（ADR-0183）：货主账户与请求方都不在载荷里；货主重授权证据是一份证据的引用，由操作者
// 如实记下。
//
// effectiveAt 的纪律同受控关闭：调用方形成请求时固定、重投原样带上。
type reopeningPayload struct {
	Parcel                       string `json:"parcel"`
	Reason                       string `json:"reason"`
	EffectiveAt                  string `json:"effectiveAt"`
	RelatedPriorClosure          string `json:"relatedPriorClosure"`
	ShipperAuthorizationEvidence string `json:"shipperAuthorizationEvidence"`
}

// IntakeControlledClosure 译受控关闭（`/shipment-requests/continued-attempt-closures`）。按包裹寻址到它当前已接受的委托，
// 来源身份取那一份（ADR-0183 决定一）；请求方由责任来源推出——货主指令才有，且只能是那份委托的客户账户（决定二、三）。
// 提交操作者只作操作证据：决定方、授权角色与授权依据快照由授权答复给出（ADR-0151 决定二；ADR-0187 决定三）。
func (intake *OperatorDecisionIntake) IntakeControlledClosure(ctx context.Context, request *http.Request) (application.FormControlledClosureCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionControlledClosure)
	if err != nil {
		return application.FormControlledClosureCommand{}, err
	}
	var payload controlledClosurePayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.FormControlledClosureCommand{}, err
	}
	kind, err := parseClosureResponsibilitySource(payload.ResponsibilitySource)
	if err != nil {
		return application.FormControlledClosureCommand{}, err
	}
	reason, effectiveAt, err := continuedAttemptFacts(payload.Reason, payload.EffectiveAt)
	if err != nil {
		return application.FormControlledClosureCommand{}, err
	}
	// 责任主体的两道校验只看载荷，排在按包裹寻址之前：坏载荷答 400（ADR-0183 决定三），不因包裹查不到先落进「与越权
	// 探针同答」那一格。全空白与没给同读，与编排合成关闭责任来源时去两端空白同一口径。
	shipperInstruction := kind == application.ShipperInstructionResponsibilitySource
	subject := strings.TrimSpace(payload.ResponsibilitySubject)
	if shipperInstruction && subject != "" {
		return application.FormControlledClosureCommand{}, fmt.Errorf(
			"%w: responsibilitySubject: a shipper instruction's subject is the request's own customer account", ErrMalformedRequest)
	}
	if !shipperInstruction && subject == "" {
		return application.FormControlledClosureCommand{}, fmt.Errorf("%w: responsibilitySubject is required for %s", ErrMalformedRequest, kind)
	}
	// 货主指令的证据同是纯载荷校验、同排在寻址之前（ADR-0187 决定五）：有请求方就得说得出凭什么；另两种来源没有请求方，
	// 带了证据就无主可证。全空白与没带同读，与编排落证据同一判据。
	evidenced := application.HasRequestEvidence(payload.ShipperInstructionEvidence)
	if shipperInstruction && !evidenced {
		return application.FormControlledClosureCommand{}, fmt.Errorf("%w: shipperInstructionEvidence is required for a shipper instruction", ErrMalformedRequest)
	}
	if !shipperInstruction && evidenced {
		return application.FormControlledClosureCommand{}, fmt.Errorf("%w: shipperInstructionEvidence belongs to a shipper instruction only", ErrMalformedRequest)
	}
	parcel, identity, err := intake.locateParcel(ctx, operator.Tenant, payload.Parcel)
	if err != nil {
		return application.FormControlledClosureCommand{}, err
	}
	command := application.FormControlledClosureCommand{
		Identity:                 identity,
		Parcel:                   parcel,
		Reason:                   reason,
		EffectiveAt:              effectiveAt,
		ResponsibilitySourceKind: kind,
		OperationEvidence:        operatorEvidence(operator),
	}
	if shipperInstruction {
		if command.Requester, err = application.ShipperRequesterOf(identity); err != nil {
			return application.FormControlledClosureCommand{}, fmt.Errorf("parcel shipment http: shipper requester: %w", err)
		}
		command.ResponsibilitySourceSubject = application.ShipperInstructionSubjectOf(identity)
		command.RequestEvidence = payload.ShipperInstructionEvidence
		return command, nil
	}
	command.ResponsibilitySourceSubject = subject
	return command, nil
}

// IntakeReopening 译重开（`/shipment-requests/continued-attempt-reopenings`）。寻址同受控关闭；货主账户恒取那份委托的客户
// 账户，载荷带了货主重授权证据时请求方同取它（ADR-0183 决定四），证据随之落成请求证据（ADR-0187 决定一）；提交操作者只作
// 操作证据。原关闭是不是货主指令形成的、证据够不够，归编排那道门。
func (intake *OperatorDecisionIntake) IntakeReopening(ctx context.Context, request *http.Request) (application.FormReopeningCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionControlledReopening)
	if err != nil {
		return application.FormReopeningCommand{}, err
	}
	var payload reopeningPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.FormReopeningCommand{}, err
	}
	reason, effectiveAt, err := continuedAttemptFacts(payload.Reason, payload.EffectiveAt)
	if err != nil {
		return application.FormReopeningCommand{}, err
	}
	var related domain.ContinuedAttemptDecisionID
	if payload.RelatedPriorClosure != "" {
		if related, err = domain.NewContinuedAttemptDecisionID(payload.RelatedPriorClosure); err != nil {
			return application.FormReopeningCommand{}, fmt.Errorf("%w: relatedPriorClosure: %v", ErrMalformedRequest, err)
		}
	}
	parcel, identity, err := intake.locateParcel(ctx, operator.Tenant, payload.Parcel)
	if err != nil {
		return application.FormReopeningCommand{}, err
	}
	shipper, err := application.ShipperRequesterOf(identity)
	if err != nil {
		return application.FormReopeningCommand{}, fmt.Errorf("parcel shipment http: shipper requester: %w", err)
	}
	command := application.FormReopeningCommand{
		Identity:                     identity,
		Parcel:                       parcel,
		Reason:                       reason,
		EffectiveAt:                  effectiveAt,
		RelatedPriorClosure:          related,
		ShipperAccount:               shipper,
		ShipperAuthorizationEvidence: payload.ShipperAuthorizationEvidence,
		OperationEvidence:            operatorEvidence(operator),
	}
	// 「带了证据」与编排那道门同一判据：否则全空白的证据会让请求方记成客户，门却当作没带。
	if application.HasRequestEvidence(payload.ShipperAuthorizationEvidence) {
		command.Requester = shipper
	}
	return command, nil
}

// locateParcel 按信封的租户与请求指名的包裹查出它当前已接受的委托，交回来源身份（ADR-0183 决定一）。查不到与越权探针同答；
// 有歧义（多于一份）原样上抛——那是数据不一致，任选一份等于替运营企业挑了委托。
func (intake *OperatorDecisionIntake) locateParcel(ctx context.Context, tenant domain.TenantID, rawParcel string) (domain.DeclaredParcelID, domain.SourceIdentity, error) {
	parcel, err := domain.NewDeclaredParcelID(rawParcel)
	if err != nil {
		return domain.DeclaredParcelID{}, domain.SourceIdentity{}, fmt.Errorf("%w: parcel: %v", ErrMalformedRequest, err)
	}
	target, found, err := intake.parcels.FindCurrentAcceptedByParcel(ctx, tenant, parcel)
	if err != nil {
		return domain.DeclaredParcelID{}, domain.SourceIdentity{}, fmt.Errorf("parcel shipment http: locate parcel: %w", err)
	}
	if !found {
		return domain.DeclaredParcelID{}, domain.SourceIdentity{}, ErrOperatorNotGranted
	}
	return parcel, target.Identity(), nil
}

// continuedAttemptFacts 收关闭与重开共有的两件事实：原因必填（立不起来是坏报文，同本包别的决定口）；生效时间给了却解不出是
// 坏报文，缺席交给编排答`输入未受理`——生效时间是决定的一部分，服务端不补。
func continuedAttemptFacts(rawReason, rawEffectiveAt string) (domain.ContinuedAttemptReasonReference, time.Time, error) {
	reason, err := domain.NewContinuedAttemptReasonReference(rawReason)
	if err != nil {
		return domain.ContinuedAttemptReasonReference{}, time.Time{}, fmt.Errorf("%w: reason: %v", ErrMalformedRequest, err)
	}
	if rawEffectiveAt == "" {
		return reason, time.Time{}, nil
	}
	effectiveAt, err := time.Parse(time.RFC3339Nano, rawEffectiveAt)
	if err != nil {
		return domain.ContinuedAttemptReasonReference{}, time.Time{}, fmt.Errorf("%w: effectiveAt: %v", ErrMalformedRequest, err)
	}
	return reason, effectiveAt.UTC(), nil
}

// parseClosureResponsibilitySource 把认不出的种类词译成坏报文；认词本身归 application.ParseClosureResponsibilitySourceKind。
func parseClosureResponsibilitySource(raw string) (application.ClosureResponsibilitySourceKind, error) {
	kind := application.ParseClosureResponsibilitySourceKind(raw)
	if kind == application.ClosureResponsibilitySourceKindInvalid {
		return kind, fmt.Errorf("%w: responsibilitySource %q is not a closure responsibility source", ErrMalformedRequest, raw)
	}
	return kind, nil
}

type decisionReasonPayload struct {
	ShipmentRequestID string `json:"shipmentRequestId"`
	Reason            string `json:"reason"`
}

type manualReviewCompletionPayload struct {
	ShipmentRequestID   string `json:"shipmentRequestId"`
	SubmissionVersionID string `json:"submissionVersionId"`
	Reason              string `json:"reason"`
}

type authorizedDispositionPayload struct {
	ShipmentRequestID   string `json:"shipmentRequestId"`
	SubmissionVersionID string `json:"submissionVersionId"`
	Choice              string `json:"choice"`
	Reason              string `json:"reason"`
}

// IntakeManualReviewCompletion 译复核完成（`/shipment-requests/manual-review-completions`）。命令里没有原因格，
// 管理台送来的复核理由就是复核留痕的证据引用。
//
// 提交版本取载荷指名的那一份，必填：版本是复核的对象，不是服务端能代填的默认值。拿此刻的当前版顶替，新版本形成之后
// 同一请求重投、或操作员按旧页面提交，旧版本的复核就签到新版本的任务上——ADR-0045 决定三否决「改钉新版本号」的正是
// 这件事；编排那道`版本已换代`的门（ADR-0132 决定二）也就只剩寻址与编排之间的竞态窗口走得到。版本不属于 ADR-0151
// 决定二不采信自报的身份格。版本与理由只看载荷，排在委托寻址之前：坏载荷答 400，不因委托查不到先落进「与越权探针
// 同答」那一格。
func (intake *OperatorDecisionIntake) IntakeManualReviewCompletion(ctx context.Context, request *http.Request) (application.CompleteManualReviewCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionManualReviewCompletion)
	if err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	var payload manualReviewCompletionPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	version, err := domain.NewSubmissionVersionID(payload.SubmissionVersionID)
	if err != nil {
		return application.CompleteManualReviewCommand{}, fmt.Errorf("%w: submissionVersionId: %v", ErrMalformedRequest, err)
	}
	evidence, err := domain.NewReviewEvidenceReference(payload.Reason)
	if err != nil {
		return application.CompleteManualReviewCommand{}, fmt.Errorf("%w: reason: %v", ErrMalformedRequest, err)
	}
	requestID, target, err := intake.locate(ctx, operator.Tenant, payload.ShipmentRequestID)
	if err != nil {
		return application.CompleteManualReviewCommand{}, err
	}
	reviewer, err := domain.NewReviewerReference(operator.Operator)
	if err != nil {
		return application.CompleteManualReviewCommand{}, fmt.Errorf("parcel shipment http: reviewer: %w", err)
	}
	return application.CompleteManualReviewCommand{
		Identity:          target.Identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: version,
		Reviewer:          reviewer,
		Evidence:          evidence,
	}, nil
}

// IntakeActiveRejection 译主动拒绝（`/shipment-requests/rejections`）。理由进结构化原因格；证据格取认证出的操作者
// ——登录操作人可以作为操作证据（PS CONTEXT）。
//
// 提交版本仍取服务端当前版本：拒绝一旦成立，委托就不会再形成新版本，同一请求重投时的当前版仍是拒绝签下的那一版，
// 重放认的对象不会被换掉。拒绝要不要绑定操作员看过的那一版，归 PS owner（票 operator-channel/34 把它划在范围外）。
func (intake *OperatorDecisionIntake) IntakeActiveRejection(ctx context.Context, request *http.Request) (application.RejectShipmentRequestCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionActiveRejection)
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
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionAuthorizedDisposition)
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
