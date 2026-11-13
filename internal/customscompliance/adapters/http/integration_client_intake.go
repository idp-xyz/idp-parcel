package customshttp

import (
	"bytes"
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

// ManifestSource 是外部监管舱单引用两口的认证结果：租户，以及客户端在册上绑定的来源身份——它代表的那家承运商。
type ManifestSource struct {
	Tenant domain.TenantID
	// Carrier 是册上绑定的来源身份原文。空串说明这只客户端没绑来源身份，交不了舱单引用。
	Carrier string
}

// ManifestSourceAuthenticator 认证一次交舱单引用的出示。它与 IntegrationClientAuthenticator 分开，是因为那两口只要租户，
// 这两口还要来源身份；并进同一个方法，另两口就得处置一个它们用不上的值。
type ManifestSourceAuthenticator interface {
	AuthenticateManifestSource(ctx context.Context, credential PresentedClientCredential) (ManifestSource, error)
}

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
// 时间照 ADR-0023 从载荷收，接收时刻取注入时钟；凭证登记的译装仍走 registrationjson 那一份，认证出的租户经它的在线
// 译装注入命令。载荷里自报租户即拒，租户键不论大小写。
type IntegrationClientIntake struct {
	authenticator IntegrationClientAuthenticator
	clock         ports.Clock
}

var (
	_ ResultIntake                           = (*IntegrationClientIntake)(nil)
	_ RegulatoryCredentialRegistrationIntake = (*IntegrationClientIntake)(nil)
	_ ManifestReferenceIntake                = (*IntegrationClientIntake)(nil)
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
	return document.command(tenant, intake.clock.Now().UTC())
}

// command 把线格式译成接收命令。集成客户端口与操作员手工录入口共用这一份：两族只差谁在提交、凭什么授予，载荷的读法
// 只有一份。
func (document externalResultDocument) command(tenant domain.TenantID, receivedAt time.Time) (application.ReceiveExternalResultCommand, error) {
	none := application.ReceiveExternalResultCommand{}
	var err error
	command := application.ReceiveExternalResultCommand{
		TenantID:       tenant,
		SourceID:       document.SourceID,
		Role:           document.Role,
		RawSemantics:   document.RawSemantics,
		ClaimedVersion: document.ClaimedVersion,
		Attempt:        document.Attempt,
		Scope:          document.Scope,
		ReceivedAt:     receivedAt,
		Corrects:       document.CorrectsSourceID,
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
	payload, err := readSinglePayload(request.Body)
	if err != nil {
		return application.RegisterCredentialCommand{}, err
	}
	command, err := registrationjson.RegulatoryCredentialFromJSONForTenant(payload, tenant)
	if err != nil {
		return application.RegisterCredentialCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

// manifestSource 认证交舱单引用的客户端，交回租户与它代表的承运商责任。认证器不认这一类事实（没实现
// ManifestSourceAuthenticator）时整口答未配置，不退回只核租户的那条路：那样承运商责任只能从载荷收。
func (intake *IntegrationClientIntake) manifestSource(
	ctx context.Context,
	request *http.Request,
) (domain.TenantID, domain.CarrierResponsibilityReference, error) {
	authenticator, able := intake.authenticator.(ManifestSourceAuthenticator)
	if !able {
		return domain.TenantID{}, domain.CarrierResponsibilityReference{}, ErrAccessChannelNotConfigured
	}
	source, err := authenticator.AuthenticateManifestSource(ctx, presentedClientCredential(request))
	if err != nil {
		return domain.TenantID{}, domain.CarrierResponsibilityReference{}, err
	}
	// 没绑来源身份的客户端代表不了任何承运商：与没有这一类授予同答，由管授予的人去册上补绑定。
	carrier, err := domain.NewCarrierResponsibilityReference(source.Carrier)
	if err != nil {
		return domain.TenantID{}, domain.CarrierResponsibilityReference{}, ErrIntegrationClientNotGranted
	}
	return source.Tenant, carrier, nil
}

func (intake *IntegrationClientIntake) IntakeManifestReference(
	ctx context.Context,
	request *http.Request,
) (application.ReceiveManifestCommand, error) {
	none := application.ReceiveManifestCommand{}
	tenant, carrier, err := intake.manifestSource(ctx, request)
	if err != nil {
		return none, err
	}
	var document manifestReferenceDocument
	if err := decodeClosedDocument(request.Body, &document); err != nil {
		return none, err
	}
	return document.command(tenant, carrier, intake.clock.Now().UTC())
}

// command 把引用载荷译成接收命令。集成客户端口与操作员手工录入口共用这一份。
func (document manifestReferenceDocument) command(tenant domain.TenantID, carrier domain.CarrierResponsibilityReference, acceptedAt time.Time) (application.ReceiveManifestCommand, error) {
	none := application.ReceiveManifestCommand{}
	var err error
	spec := domain.ExternalManifestReferenceSpec{
		Carrier:    carrier,
		SourceFact: strings.TrimSpace(document.SourceFact),
		AcceptedAt: acceptedAt,
	}
	if spec.Manifest, err = domain.NewExternalManifestID(document.ManifestID); err != nil {
		return none, fmt.Errorf("%w: manifestId: %v", ErrMalformedRequest, err)
	}
	if spec.Version, err = domain.NewManifestSourceVersion(document.Version); err != nil {
		return none, fmt.Errorf("%w: version: %v", ErrMalformedRequest, err)
	}
	if spec.Procedure, err = domain.NewCustomsProcedureReference(document.Procedure); err != nil {
		return none, fmt.Errorf("%w: procedure: %v", ErrMalformedRequest, err)
	}
	if spec.Direction, err = manifestDirectionFromWord(document.Direction); err != nil {
		return none, err
	}
	if spec.Scope, err = domain.NewDecisionScopeReference(document.Scope); err != nil {
		return none, fmt.Errorf("%w: scope: %v", ErrMalformedRequest, err)
	}
	if spec.SourceFact == "" {
		return none, fmt.Errorf("%w: sourceFact is required", ErrMalformedRequest)
	}
	return application.ReceiveManifestCommand{TenantID: tenant, Spec: spec}, nil
}

func (intake *IntegrationClientIntake) IntakeManifestRevision(
	ctx context.Context,
	request *http.Request,
) (application.ReviseManifestCommand, error) {
	none := application.ReviseManifestCommand{}
	tenant, carrier, err := intake.manifestSource(ctx, request)
	if err != nil {
		return none, err
	}
	var document manifestRevisionDocument
	if err := decodeClosedDocument(request.Body, &document); err != nil {
		return none, err
	}
	return document.command(tenant, carrier, intake.clock.Now().UTC())
}

// command 把版本推进载荷译成推进命令。两个口共用这一份。
func (document manifestRevisionDocument) command(tenant domain.TenantID, carrier domain.CarrierResponsibilityReference, at time.Time) (application.ReviseManifestCommand, error) {
	none := application.ReviseManifestCommand{}
	var err error
	command := application.ReviseManifestCommand{
		TenantID:   tenant,
		Carrier:    carrier,
		SourceFact: strings.TrimSpace(document.SourceFact),
		At:         at,
	}
	if command.Manifest, err = domain.NewExternalManifestID(document.ManifestID); err != nil {
		return none, fmt.Errorf("%w: manifestId: %v", ErrMalformedRequest, err)
	}
	if command.Version, err = domain.NewManifestSourceVersion(document.Version); err != nil {
		return none, fmt.Errorf("%w: version: %v", ErrMalformedRequest, err)
	}
	if command.Scope, err = domain.NewDecisionScopeReference(document.Scope); err != nil {
		return none, fmt.Errorf("%w: scope: %v", ErrMalformedRequest, err)
	}
	if command.SourceFact == "" {
		return none, fmt.Errorf("%w: sourceFact is required", ErrMalformedRequest)
	}
	return command, nil
}

// manifestReferenceDocument 是舱单引用首登口的载荷。没有租户与承运商两格：前者归认证结果，后者归册上的来源身份绑定；
// 也没有接受时刻，那是服务端自己的时刻。载荷多带任何一格即拒（decodeClosedDocument）。
type manifestReferenceDocument struct {
	ManifestID string `json:"manifestId"`
	Version    string `json:"version"`
	Procedure  string `json:"procedure"`
	Direction  string `json:"direction"`
	Scope      string `json:"scope"`
	SourceFact string `json:"sourceFact"`
}

// manifestRevisionDocument 是来源版本推进口的载荷：程序与方向随在册引用，不随版本改。
type manifestRevisionDocument struct {
	ManifestID string `json:"manifestId"`
	Version    string `json:"version"`
	Scope      string `json:"scope"`
	SourceFact string `json:"sourceFact"`
}

func manifestDirectionFromWord(raw string) (domain.ManifestDirection, error) {
	for _, direction := range []domain.ManifestDirection{domain.ImportManifest, domain.ExportManifest} {
		if direction.String() == raw {
			return direction, nil
		}
	}
	return domain.ManifestDirectionInvalid, fmt.Errorf("%w: direction=%q is not a manifest direction word", ErrMalformedRequest, raw)
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
	// CorrectsSourceID 是来源声明本结果所更正的那一份的来源标识，可缺；指不指得到由编排判。
	CorrectsSourceID string `json:"correctsSourceId,omitempty"`
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

// readSinglePayload 收一份载荷原文：只许一个 JSON 对象、其后无尾随内容。认证租户不往原文里拼——拼回去再解，
// 载荷里 `tenantid` 一类变体会按 encoding/json 的大小写折叠盖掉拼进去的那一个（票 operator-channel/52）。
func readSinglePayload(body io.Reader) ([]byte, error) {
	decoder := json.NewDecoder(body)
	var payload json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("{")) {
		return nil, fmt.Errorf("%w: the payload must be a JSON object", ErrMalformedRequest)
	}
	return payload, nil
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
