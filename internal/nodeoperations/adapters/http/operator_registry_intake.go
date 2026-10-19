package nodeopshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回操作者绑定的租户。登记写面不判准入范围（ADR-0100 决定四：登记一行
// 不形成任何生产事实），所以它交不出「不在准入范围」那一格；其余各格与本包别的操作者口共用哨兵。从共享接入身份能力译过来的那一层在
// adapters/accessidentity。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorRegistryIntake 是操作者渠道在本上下文各本登记册写口的 Intake（ADR-0100 决定四；ADR-0085 决定一）。线格式就是各册的载荷
// 类型，租户取认证结果；受控批量口收的是同一份载荷再加租户，译成命令只走载荷自己的 Command，两个入口的严格性不会各自漂移。先认证、
// 后解载荷：令牌不过的调用方看不到载荷校验的结果。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var _ PreAdviceWindowRegistrationIntake = (*OperatorRegistryIntake)(nil)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

// IntakePreAdviceWindowRegistration 译预报有效时窗登记（`/node-operations-pre-advice-window-registrations`）。
func (intake *OperatorRegistryIntake) IntakePreAdviceWindowRegistration(ctx context.Context, request *http.Request) (application.RegisterPreAdviceWindowCommand, error) {
	tenant, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RegisterPreAdviceWindowCommand{}, err
	}
	var payload PreAdviceWindowRegistrationPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return application.RegisterPreAdviceWindowCommand{}, err
	}
	return payload.Command(tenant)
}
