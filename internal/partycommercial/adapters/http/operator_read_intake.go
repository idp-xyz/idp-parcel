package commercialhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在本包目录查阅面的 Intake（ADR-0100 决定四、ADR-0077）。不写册、不形成生产事实。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("party commercial http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

const (
	operatorCatalogueScopeReference = "MASTER_DATA_AND_OPERATIONS_READ"
	operatorCatalogueReadLimit      = 200
)

var _ CommercialCatalogueIntake = (*OperatorOperationsReadIntake)(nil)

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
