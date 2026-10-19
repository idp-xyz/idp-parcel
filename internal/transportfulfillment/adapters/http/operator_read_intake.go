package tfhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。查阅读不判准入范围，所以它
// 交不出「不在准入范围」那一格；其余三格与运营决定口共用本包的哨兵。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在本包查阅面的 Intake（ADR-0100 决定四），收 CatalogueQueryIntake 的各口共用。
// 查阅不形成生产事实；判断落在运营决定口上，那些口挂 OperatorDecisionIntake。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ CatalogueQueryIntake = (*OperatorOperationsReadIntake)(nil)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("transport fulfillment http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// 作用域引用是授权结果的审计引用，不参与行过滤，取能力面的名字。页大小是这一渠道的产品策略，不采信请求，也不随租户变。
const (
	operatorCatalogueScopeReference = "MASTER_DATA_AND_OPERATIONS_READ"
	operatorCatalogueReadLimit      = 200
)

// IntakeCatalogueQuery 不读请求体。租户只取自信封；定位参数由各端点自己读。
func (intake *OperatorOperationsReadIntake) IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return CatalogueQuery{}, err
	}
	reference, err := domain.NewOperationsScopeReference(operatorCatalogueScopeReference)
	if err != nil {
		return CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return CatalogueQuery{}, err
	}
	return CatalogueQuery{Scope: scope, Limit: operatorCatalogueReadLimit}, nil
}
