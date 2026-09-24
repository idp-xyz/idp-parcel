package pricinghttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 操作者渠道在本包登记写面的三格答复（ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列，各对一种恢复动作
// （ADR-0029）。登记写面不判准入范围，所以没有「不在准入范围」那一格。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("pricing http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有登记册配置写的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("pricing http: operator holds no registry configuration grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("pricing http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// maxRegistrationBytes 给一份登记载荷设读取上限：设限只为不让一个坏掉的调用方把整段请求读进内存，超限答坏报文。
const maxRegistrationBytes = 1 << 20

// OperatorIdentity 是一次登记册配置写出示认证出的身份：租户与提交操作者的引用（发行方 + sub）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorRegistryAuthenticator 认证一次登记册配置写出示。失败只交回本包的格；从共享接入身份能力译过来的那一层在
// adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// OperatorRegistryIntake 是操作者渠道在本包参考序列登记、预览、复核与参考目录登记四口的 Intake（ADR-0100 决定四；
// 票 operator-channel/04）。
//
// 线格式是本包既有的运营操作者面载荷（ADR-0101 决定一）：序列登记与预览共用 ReferenceSeriesRegistrationPayload，目录
// 登记用 ReferenceCataloguePayload，它们的注释本就写明「租户与登记责任方从 OperatorEnvelope 来，由 Intake 作为入参交
// 进来」——本类型就是那个 Intake，登记责任方与复核人都取认证出的提交操作者。价卡登记口不在这里：价卡的在线导入属
// price-card-import 那一批（ADR-0101）。先认证、后解载荷。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ ReferenceSeriesRegistrationIntake    = (*OperatorRegistryIntake)(nil)
	_ ReferenceSeriesPreviewIntake         = (*OperatorRegistryIntake)(nil)
	_ ReferenceSeriesReviewIntake          = (*OperatorRegistryIntake)(nil)
	_ ReferenceCatalogueRegistrationIntake = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("pricing http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeReferenceSeriesRegistration(ctx context.Context, request *http.Request) (application.RegisterReferenceSeriesCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.RegisterReferenceSeriesCommand{}, err
	}
	payload, err := DecodeReferenceSeriesRegistrationPayload(body)
	if err != nil {
		return application.RegisterReferenceSeriesCommand{}, err
	}
	return payload.RegistrationCommand(operator.Tenant, operator.Operator)
}

func (intake *OperatorRegistryIntake) IntakeReferenceSeriesPreview(ctx context.Context, request *http.Request) (application.PreviewReferenceSeriesCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.PreviewReferenceSeriesCommand{}, err
	}
	payload, err := DecodeReferenceSeriesRegistrationPayload(body)
	if err != nil {
		return application.PreviewReferenceSeriesCommand{}, err
	}
	return payload.PreviewCommand(operator.Tenant, operator.Operator)
}

func (intake *OperatorRegistryIntake) IntakeReferenceCatalogueRegistration(ctx context.Context, request *http.Request) (application.RegisterReferenceCatalogueCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.RegisterReferenceCatalogueCommand{}, err
	}
	payload, err := DecodeReferenceCataloguePayload(body)
	if err != nil {
		return application.RegisterReferenceCatalogueCommand{}, err
	}
	return payload.RegistrationCommand(operator.Tenant, operator.Operator)
}

// IntakeReferenceSeriesReview 译序列复核。复核人取认证出的提交操作者：四眼门（复核人不是登记责任方）由用例与领域判，
// 两边比的都是认证过的身份，不是自报的名字。
func (intake *OperatorRegistryIntake) IntakeReferenceSeriesReview(ctx context.Context, request *http.Request) (application.ReviewReferenceSeriesCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.ReviewReferenceSeriesCommand{}, err
	}
	payload, err := DecodeReferenceSeriesReviewPayload(body)
	if err != nil {
		return application.ReviewReferenceSeriesCommand{}, err
	}
	return payload.Command(operator.Tenant, operator.Operator)
}

func (intake *OperatorRegistryIntake) authenticate(ctx context.Context, request *http.Request) (OperatorIdentity, io.Reader, error) {
	operator, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return OperatorIdentity{}, nil, err
	}
	if request.Body == nil {
		return OperatorIdentity{}, nil, fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	return operator, io.LimitReader(request.Body, maxRegistrationBytes), nil
}

// ReferenceSeriesReviewPayload 是序列复核口的在线载荷：受控批量口复核文档去掉租户与复核人两格——两者都从认证结果来，
// 载荷里出现即按未知键拒。reviewedAt 可缺：缺时由用例取时钟当下（同受控批量口）。
type ReferenceSeriesReviewPayload struct {
	SeriesID      string `json:"seriesId"`
	SeriesVersion string `json:"seriesVersion"`
	Decision      string `json:"decision"`
	Basis         string `json:"basis"`
	ReviewedAt    string `json:"reviewedAt,omitempty"`
}

// DecodeReferenceSeriesReviewPayload 封闭解码：未知键与尾随内容都答坏报文。
func DecodeReferenceSeriesReviewPayload(body io.Reader) (ReferenceSeriesReviewPayload, error) {
	var payload ReferenceSeriesReviewPayload
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ReferenceSeriesReviewPayload{}, fmt.Errorf("%w: reference series review payload: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return ReferenceSeriesReviewPayload{}, fmt.Errorf("%w: reference series review payload: trailing content", ErrMalformedRequest)
	}
	return payload, nil
}

// Command 只做形状翻译（时刻按 RFC 3339 解）；四件齐不齐、结论在不在封闭集、四眼门都留给用例与领域判，同受控批量口。
func (payload ReferenceSeriesReviewPayload) Command(tenant domain.TenantID, reviewer string) (application.ReviewReferenceSeriesCommand, error) {
	if tenant.String() == "" || reviewer == "" {
		return application.ReviewReferenceSeriesCommand{}, ErrOperatorIdentityMissing
	}
	command := application.ReviewReferenceSeriesCommand{
		Tenant:        tenant,
		SeriesID:      payload.SeriesID,
		SeriesVersion: payload.SeriesVersion,
		Reviewer:      reviewer,
		Decision:      domain.SeriesReviewDecision(payload.Decision),
		Basis:         payload.Basis,
	}
	if payload.ReviewedAt != "" {
		reviewedAt, err := time.Parse(time.RFC3339, payload.ReviewedAt)
		if err != nil {
			return application.ReviewReferenceSeriesCommand{}, fmt.Errorf("%w: reviewedAt must be RFC 3339: %v", ErrMalformedRequest, err)
		}
		command.ReviewedAt = reviewedAt
	}
	return command, nil
}
