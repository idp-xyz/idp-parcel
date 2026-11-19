package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// operatorCatalogueReadLimit 是操作者渠道查阅的页大小，与别的上下文的目录查阅同一个渠道契约值。
const operatorCatalogueReadLimit = 200

// OperatorOperationsReadIntake 是操作者渠道在本包查阅面的 Intake（ADR-0100 决定四、ADR-0077）。不写册、不形成生产事实。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ CatalogueIntake = (*OperatorOperationsReadIntake)(nil)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// IntakeCatalogueQuery 不读请求体。租户只取自信封。
func (intake *OperatorOperationsReadIntake) IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return CatalogueQuery{}, err
	}
	return CatalogueQuery{Tenant: tenant, Limit: operatorCatalogueReadLimit}, nil
}
