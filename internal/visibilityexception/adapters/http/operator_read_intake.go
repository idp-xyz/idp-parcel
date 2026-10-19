package visibilityhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格；
// 从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在运营查阅面的 Intake（ADR-0100 决定四），收 OperationsTrackingIntake 的各口
// 共用。它只实现运营查阅：客户查阅面的 QueryIntake 刻意不实现——那一面的客户维是调用方自己的身份主张，归客户渠道
// （ADR-0139 一族）；索赔与各登记命令口也不实现。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ OperationsTrackingIntake = (*OperatorOperationsReadIntake)(nil)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("visibility http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// 作用域引用是授权结果的审计引用，不参与行过滤，取能力面的名字。页大小是这一渠道的产品策略，不采信请求，也不随租户变。
const (
	operatorOperationsScopeReference = "MASTER_DATA_AND_OPERATIONS_READ"
	operatorOperationsReadLimit      = 200
)

// IntakeOperationsQuery 不读请求体。租户只取自信封；定位与分派参数由各端点自己读。
func (intake *OperatorOperationsReadIntake) IntakeOperationsQuery(ctx context.Context, request *http.Request) (OperationsTrackingQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return OperationsTrackingQuery{}, err
	}
	reference, err := domain.NewOperationsScopeReference(operatorOperationsScopeReference)
	if err != nil {
		return OperationsTrackingQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return OperationsTrackingQuery{}, err
	}
	return OperationsTrackingQuery{Scope: scope, Limit: operatorOperationsReadLimit}, nil
}
