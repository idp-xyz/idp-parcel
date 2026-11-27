package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// WarehouseScopeReader 在同一次查阅读成立之后交回看得见的仓。实现了它的认证器，仓册按这份范围切；
// 没实现的保持整租户，那是测试替身还没接上，不是「没授等于全部」。
type WarehouseScopeReader interface {
	ReadWarehouseScope(ctx context.Context, bearerToken string) (domain.TenantID, ports.WarehouseScope, error)
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

// IntakeCatalogueQuery 不读请求体。租户只取自信封。认证器交得出仓可见范围时，范围一并放进查询。
func (intake *OperatorOperationsReadIntake) IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error) {
	token := httpapi.BearerToken(request)
	if reader, ok := intake.authenticator.(WarehouseScopeReader); ok {
		tenant, scope, err := reader.ReadWarehouseScope(ctx, token)
		if err != nil {
			return CatalogueQuery{}, err
		}
		return CatalogueQuery{Tenant: tenant, Limit: operatorCatalogueReadLimit, Warehouses: scope}, nil
	}
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, token)
	if err != nil {
		return CatalogueQuery{}, err
	}
	return CatalogueQuery{Tenant: tenant, Limit: operatorCatalogueReadLimit}, nil
}
