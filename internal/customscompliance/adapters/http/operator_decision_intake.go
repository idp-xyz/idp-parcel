package customshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorDecision 是本包各口对应的运营决定种类（ADR-0227 决定一），字面与操作者册的决定种类相同。三口各一种：能建案的
// 不因此能确认授权，能确认授权的不因此能发起提交——关务 CONTEXT 要求批准与实际提交权限分别表达，授予的粒度就是权力的粒度。
type OperatorDecision string

const (
	OperatorDecisionCaseEstablishment       OperatorDecision = "CUSTOMS_CASE_ESTABLISHMENT"
	OperatorDecisionSubmissionAuthorization OperatorDecision = "CUSTOMS_SUBMISSION_AUTHORIZATION"
	OperatorDecisionDeclarationSubmission   OperatorDecision = "CUSTOMS_DECLARATION_SUBMISSION"
)

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与做出决定的操作者（「发行方#sub」）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorDecisionAuthenticator 认证一次运营决定出示，交回出示者的身份。失败只交回本包的格：ErrAccessChannelNotConfigured、
// ErrOperatorCredentialRejected、ErrOperatorNotGranted、ErrOutsideAdmissionScope、ErrIdentityDependencyUnavailable；从共享接入
// 身份能力译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorDecisionAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// CaseEstablishmentIntake 译建案请求（`/customs-cases`）。
type CaseEstablishmentIntake interface {
	IntakeCaseEstablishment(ctx context.Context, request *http.Request) (application.EstablishCaseForParcelsCommand, error)
}

// SubmissionAuthorizationIntake 译提交授权确认（`/customs-submission-authorizations`）。
type SubmissionAuthorizationIntake interface {
	IntakeSubmissionAuthorization(ctx context.Context, request *http.Request) (application.ConfirmSubmissionAuthorizationCommand, error)
}

// DeclarationSubmissionIntake 译申报提交执行（`/customs-declaration-submissions`）。
type DeclarationSubmissionIntake interface {
	IntakeDeclarationSubmission(ctx context.Context, request *http.Request) (application.SubmitDeclarationCommand, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在本包各口的 Intake（ADR-0151 决定一、ADR-0227 决定一）。
//
// 载荷只有内容、没有身份：租户与操作者只取认证结果，载荷带 tenantId 或别的身份格即是未知字段、答坏报文。先认证、后解载荷：
// 令牌不过的调用方看不到载荷校验的结果。
type OperatorDecisionIntake struct {
	authenticator OperatorDecisionAuthenticator
}

var (
	_ CaseEstablishmentIntake       = (*OperatorDecisionIntake)(nil)
	_ SubmissionAuthorizationIntake = (*OperatorDecisionIntake)(nil)
	_ DeclarationSubmissionIntake   = (*OperatorDecisionIntake)(nil)
)

func NewOperatorDecisionIntake(authenticator OperatorDecisionAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("customs http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

// CaseRolePayload 是一项初始关务参与方角色快照的线格式；三件都要给（domain.CaseRoleSnapshot 的完整性门）。
type CaseRolePayload struct {
	Role      string `json:"role"`
	Party     string `json:"party"`
	Authority string `json:"authority"`
}

// CaseEstablishmentPayload 是建案口的线格式。包裹只给身份：客户归属与来源资料引用由编排查当前已接受委托取，这里没有这两格。
type CaseEstablishmentPayload struct {
	Jurisdiction string            `json:"jurisdiction"`
	Direction    string            `json:"direction"`
	Procedure    string            `json:"procedure"`
	Obligation   string            `json:"obligation"`
	Parcels      []string          `json:"parcels"`
	Roles        []CaseRolePayload `json:"roles"`
}

// Command 把载荷译成编排命令。空着的格交零值，留给编排答`未受理`（200，形成了的业务答案）；方向给了却不是两值之一是坏报文
// （400）——两者的续办不同，不折成一格。
func (payload CaseEstablishmentPayload) Command(tenant domain.TenantID) (application.EstablishCaseForParcelsCommand, error) {
	direction, err := parseDirection(payload.Direction)
	if err != nil {
		return application.EstablishCaseForParcelsCommand{}, err
	}
	roles := make([]domain.CaseRoleSnapshot, 0, len(payload.Roles))
	for _, role := range payload.Roles {
		roles = append(roles, domain.CaseRoleSnapshot{Role: role.Role, Party: role.Party, Authority: role.Authority})
	}
	return application.EstablishCaseForParcelsCommand{
		TenantID:     tenant,
		Jurisdiction: optionalReference(payload.Jurisdiction, domain.NewRegulatoryJurisdictionReference),
		Direction:    direction,
		Procedure:    optionalReference(payload.Procedure, domain.NewCustomsProcedureReference),
		Obligation:   optionalReference(payload.Obligation, domain.NewObligationScopeReference),
		Parcels:      append([]string(nil), payload.Parcels...),
		Roles:        roles,
	}, nil
}

// SubmissionAuthorizationPayload 是授权确认口的线格式：只指名申报单元。确认人是认证出来的那个人，载荷里没有这一格。
type SubmissionAuthorizationPayload struct {
	UnitID string `json:"unitId"`
}

// DeclarationSubmissionPayload 是提交执行口的线格式。没有尝试结果与发送时间两格：提交执行口不发送任何东西，首次尝试一律
// 登记为`已登记待发送`（ADR-0227 决定四）；让请求方说「已回执」「何时发出」就是让他替一次没发生的发送作证。
type DeclarationSubmissionPayload struct {
	UnitID    string   `json:"unitId"`
	CaseID    string   `json:"caseId"`
	Procedure string   `json:"procedure"`
	Members   []string `json:"members"`
	Dossier   string   `json:"dossier"`
	Roles     string   `json:"roles"`
	Target    string   `json:"target"`
}

func (payload DeclarationSubmissionPayload) Command(tenant domain.TenantID) application.SubmitDeclarationCommand {
	return application.SubmitDeclarationCommand{
		TenantID:      tenant,
		UnitID:        payload.UnitID,
		CaseID:        payload.CaseID,
		Procedure:     payload.Procedure,
		Members:       append([]string(nil), payload.Members...),
		Dossier:       payload.Dossier,
		Roles:         payload.Roles,
		Target:        payload.Target,
		InitialResult: domain.AttemptRegisteredPendingSend,
	}
}

// IntakeCaseEstablishment 按建案这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCaseEstablishment(ctx context.Context, request *http.Request) (application.EstablishCaseForParcelsCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionCaseEstablishment)
	if err != nil {
		return application.EstablishCaseForParcelsCommand{}, err
	}
	var payload CaseEstablishmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.EstablishCaseForParcelsCommand{}, err
	}
	return payload.Command(operator.Tenant)
}

// IntakeSubmissionAuthorization 按授权确认这一种决定认证；确认人与所凭授予取自认证结果与本口的决定种类。
func (intake *OperatorDecisionIntake) IntakeSubmissionAuthorization(ctx context.Context, request *http.Request) (application.ConfirmSubmissionAuthorizationCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionSubmissionAuthorization)
	if err != nil {
		return application.ConfirmSubmissionAuthorizationCommand{}, err
	}
	var payload SubmissionAuthorizationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.ConfirmSubmissionAuthorizationCommand{}, err
	}
	return application.ConfirmSubmissionAuthorizationCommand{
		TenantID:  operator.Tenant,
		UnitID:    payload.UnitID,
		Confirmer: operator.Operator,
		Grant:     string(OperatorDecisionSubmissionAuthorization),
	}, nil
}

// IntakeDeclarationSubmission 按申报提交执行这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeDeclarationSubmission(ctx context.Context, request *http.Request) (application.SubmitDeclarationCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionDeclarationSubmission)
	if err != nil {
		return application.SubmitDeclarationCommand{}, err
	}
	var payload DeclarationSubmissionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.SubmitDeclarationCommand{}, err
	}
	return payload.Command(operator.Tenant), nil
}

// decodeClosedPayload 解封闭载荷：未知字段、多余内容、读超限都是坏报文。
func decodeClosedPayload(body io.Reader, target any) error {
	if body == nil {
		return fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	decoder := json.NewDecoder(io.LimitReader(body, maxRegistrationBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}

// parseDirection 译进出口方向：空着交零值（编排答未受理），给了却不是两值之一答坏报文。
func parseDirection(raw string) (domain.ManifestDirection, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return domain.ManifestDirectionInvalid, nil
	case domain.ExportManifest.String():
		return domain.ExportManifest, nil
	case domain.ImportManifest.String():
		return domain.ImportManifest, nil
	default:
		return domain.ManifestDirectionInvalid, fmt.Errorf("%w: direction %q is neither EXPORT nor IMPORT", ErrMalformedRequest, raw)
	}
}

// optionalReference 把一格引用译成领域值；空着或构造不出（如含 NUL）都交零值，由编排的受理门答未受理——那是领域对内容的
// 判断，不是报文形状的错。
func optionalReference[T any](raw string, construct func(string) (T, error)) T {
	var none T
	if strings.TrimSpace(raw) == "" {
		return none
	}
	built, err := construct(raw)
	if err != nil {
		return none
	}
	return built
}
