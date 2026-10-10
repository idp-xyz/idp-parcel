package customshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 集成客户端族在本包两口的答复（ADR-0149 决定三、ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列。
// 每一格一种恢复动作（ADR-0029）。登记册配置写面的操作者哨兵不复用：持令牌的是外部系统，错误码若仍叫操作者，
// 管授予的人会去操作者册里找一个不存在的人。
var (
	// ErrIntegrationClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）。
	ErrIntegrationClientCredentialRejected = errors.New("customs http: integration client credential rejected")
	// ErrIntegrationClientNotGranted：令牌有效，但客户端不在册，或此刻没有所交那一类事实的生效授予（403）。
	// 未登记、授予不含该类、区间外与已撤销同答这一格，不说哪一半不对。
	ErrIntegrationClientNotGranted = errors.New("customs http: integration client holds no effective grant for this fact type")
	// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）——阶段治理去登记区间。
	ErrOutsideAdmissionScope = errors.New("customs http: production write is outside the admission scope")
)

const (
	codeIntegrationClientCredentialRejected = "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"
	codeIntegrationClientNotGranted         = "INTEGRATION_CLIENT_NOT_GRANTED"
	codeOutsideAdmissionScope               = "OUTSIDE_ADMISSION_SCOPE"
)

// IntegrationClientFact 是本包两口各自要核的事实类型，字面与集成客户端册的授予单位相同。
type IntegrationClientFact string

const (
	FactCustomsExternalResult IntegrationClientFact = "CUSTOMS_EXTERNAL_RESULT"
	FactRegulatoryCredential  IntegrationClientFact = "REGULATORY_CREDENTIAL"
)

// PresentedClientCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。
type PresentedClientCredential struct {
	Token       string
	Certificate []byte
}

// IntegrationClientAuthenticator 认证一次集成客户端出示，交回册上绑定的租户。失败只交回本包的格；从共享接入身份
// 能力译过来的那一层在 adapters/accessidentity。
type IntegrationClientAuthenticator interface {
	Authenticate(ctx context.Context, credential PresentedClientCredential, fact IntegrationClientFact) (domain.TenantID, error)
}

// IntegrationClientIntake 是集成客户端族在外部结果与监管凭证登记两口的 Intake（ADR-0149 决定三；票 operator-channel/11）。
//
// 先认证、后读载荷：令牌不过的调用方看不到载荷校验的结果。租户只取认证结果。外部结果的来源标识、层、原文语义与发生
// 时间照 ADR-0023 从载荷收，接收时刻取注入时钟；凭证登记的译装仍走 registrationjson 那一份，本包只把认证出的租户拼回
// tenantId。载荷里自报租户即拒。
type IntegrationClientIntake struct {
	authenticator IntegrationClientAuthenticator
	clock         ports.Clock
}

var (
	_ ResultIntake                           = (*IntegrationClientIntake)(nil)
	_ RegulatoryCredentialRegistrationIntake = (*IntegrationClientIntake)(nil)
)

func NewIntegrationClientIntake(authenticator IntegrationClientAuthenticator, clock ports.Clock) (*IntegrationClientIntake, error) {
	if authenticator == nil || clock == nil {
		return nil, errors.New("customs http: integration client intake needs an authenticator and a clock")
	}
	return &IntegrationClientIntake{authenticator: authenticator, clock: clock}, nil
}

func (intake *IntegrationClientIntake) IntakeResult(
	ctx context.Context,
	request *http.Request,
) (application.ReceiveExternalResultCommand, error) {
	none := application.ReceiveExternalResultCommand{}
	tenant, err := intake.authenticator.Authenticate(ctx, presentedClientCredential(request), FactCustomsExternalResult)
	if err != nil {
		return none, err
	}
	var document externalResultDocument
	if err := decodeClosedDocument(request.Body, &document); err != nil {
		return none, err
	}
	command := application.ReceiveExternalResultCommand{
		TenantID:       tenant,
		SourceID:       document.SourceID,
		Role:           document.Role,
		RawSemantics:   document.RawSemantics,
		ClaimedVersion: document.ClaimedVersion,
		Attempt:        document.Attempt,
		Scope:          document.Scope,
		ReceivedAt:     intake.clock.Now().UTC(),
	}
	if document.Layer != "" {
		if command.Layer, err = resultLayerFromWord(document.Layer); err != nil {
			return none, err
		}
	}
	if raw := strings.TrimSpace(document.OccurredAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
		}
		command.OccurredAt = at.UTC()
	}
	if document.Release != nil {
		if command.Release, err = document.Release.content(); err != nil {
			return none, err
		}
	}
	return command, nil
}

func (intake *IntegrationClientIntake) IntakeRegulatoryCredentialRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterCredentialCommand, error) {
	tenant, err := intake.authenticator.Authenticate(ctx, presentedClientCredential(request), FactRegulatoryCredential)
	if err != nil {
		return application.RegisterCredentialCommand{}, err
	}
	snapshot, err := snapshotWithAuthenticatedTenant(request.Body, tenant.String())
	if err != nil {
		return application.RegisterCredentialCommand{}, err
	}
	command, err := registrationjson.RegulatoryCredentialFromJSON(snapshot)
	if err != nil {
		return application.RegisterCredentialCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

// externalResultDocument 是外部结果口的载荷，逐格镜像 application.ReceiveExternalResultCommand 去掉租户与接收时间——前者归
// 认证结果，后者是服务端自己的时刻。
type externalResultDocument struct {
	SourceID       string                   `json:"sourceId"`
	Layer          string                   `json:"layer"`
	Role           string                   `json:"role"`
	RawSemantics   string                   `json:"rawSemantics"`
	ClaimedVersion string                   `json:"claimedVersion"`
	Attempt        int                      `json:"attempt"`
	Scope          string                   `json:"scope"`
	OccurredAt     string                   `json:"occurredAt"`
	Release        *externalReleaseDocument `json:"release,omitempty"`
}

type externalReleaseDocument struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
	Condition string `json:"condition,omitempty"`
}

func (release externalReleaseDocument) content() (*application.ReleaseContent, error) {
	kind, err := releaseKindFromWord(release.Kind)
	if err != nil {
		return nil, err
	}
	authority, err := domain.NewRegulatoryAuthorityReference(release.Authority)
	if err != nil {
		return nil, fmt.Errorf("%w: release.authority: %v", ErrMalformedRequest, err)
	}
	return &application.ReleaseContent{Kind: kind, Authority: authority, Condition: release.Condition}, nil
}

func resultLayerFromWord(raw string) (domain.ResultLayer, error) {
	for _, layer := range []domain.ResultLayer{
		domain.RegulatoryReceiptLayer, domain.BusinessAcceptanceLayer, domain.ProcessDecisionLayer,
		domain.AssessedDutyLayer, domain.ReleaseResultLayer, domain.DispositionDecisionLayer,
	} {
		if layer.String() == raw {
			return layer, nil
		}
	}
	return domain.ResultLayerInvalid, fmt.Errorf("%w: layer=%q is not a result layer word", ErrMalformedRequest, raw)
}

func releaseKindFromWord(raw string) (domain.ReleaseKind, error) {
	for _, kind := range []domain.ReleaseKind{domain.FullRelease, domain.PartialRelease, domain.ConditionalRelease} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.ReleaseKindInvalid, fmt.Errorf("%w: release.kind=%q is not a release kind word", ErrMalformedRequest, raw)
}

func presentedClientCredential(request *http.Request) PresentedClientCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedClientCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}

// snapshotWithAuthenticatedTenant 读一份登记快照、拒自报租户，把认证出的租户拼回 tenantId。键在场即拒、不看值。
func snapshotWithAuthenticatedTenant(body io.Reader, tenantID string) ([]byte, error) {
	decoder := json.NewDecoder(body)
	var snapshot map[string]json.RawMessage
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("%w: the payload must be a JSON object", ErrMalformedRequest)
	}
	if _, present := snapshot["tenantId"]; present {
		return nil, fmt.Errorf(
			"%w: tenantId must not be carried in the payload; the tenant grid is filled by the access channel, not by the request",
			ErrMalformedRequest,
		)
	}
	tenant, err := json.Marshal(tenantID)
	if err != nil {
		return nil, fmt.Errorf("customs compliance http: encode authenticated tenant: %w", err)
	}
	snapshot["tenantId"] = tenant
	return json.Marshal(snapshot)
}

func decodeClosedDocument(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}
