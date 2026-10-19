package nodeopshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格；
// 从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在节点作业查阅页的 Intake（ADR-0100 决定四）。查阅不登记、不编辑，不形成作业
// 事实，所以不挂作业事实那一面（ADR-0149），也不判准入范围。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ CatalogueQueryIntake = (*OperatorOperationsReadIntake)(nil)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// 作用域引用是授权结果的审计引用，不参与行过滤，取能力面的名字。页大小是这一渠道的产品策略，不采信请求，也不随租户变。
const (
	operatorCatalogueScopeReference = "MASTER_DATA_AND_OPERATIONS_READ"
	operatorCatalogueReadLimit      = 200
)

// IntakeCatalogueQuery 不读请求体。租户只取自信封。
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
