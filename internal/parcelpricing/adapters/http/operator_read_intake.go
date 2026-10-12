package pricinghttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格，与登记
// 写面同一组；从共享接入身份能力译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture
// 的边界门禁）。
//
// 只交回租户、不交操作者主体：挂这一面的命令里没有操作者那一格——谁发起了这次试算不进命令，与回放口不记触发者同理。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在本包主数据与运营查阅读面的 Intake（ADR-0100 决定四）。今天挂它的只有试算
// 一口（ADR-0152 决定六）：试算不写任何册、不形成生产事实，能力面按语义归查阅读、不按 HTTP 方法归，所以不挂
// OperatorRegistryIntake。译法各在该口自己的文件里；先认证、后解载荷。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("pricing http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// authenticate 是各口的第一步：令牌不过的请求，请求体一个字节都不读。认证交回空租户是 Intake 一侧的错，不折成坏报文。
func (intake *OperatorOperationsReadIntake) authenticate(ctx context.Context, request *http.Request) (domain.TenantID, io.Reader, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, nil, err
	}
	if tenant.String() == "" {
		return domain.TenantID{}, nil, ErrOperatorIdentityMissing
	}
	if request.Body == nil {
		return domain.TenantID{}, nil, fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	return tenant, io.LimitReader(request.Body, maxRegistrationBytes), nil
}
