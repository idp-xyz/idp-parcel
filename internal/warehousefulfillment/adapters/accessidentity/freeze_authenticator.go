package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// OperatorStockFreezeAuthenticator 认证库存冻结与解冻。两种动作共用 STOCK_FREEZE。
type OperatorStockFreezeAuthenticator struct {
	minter Minter
}

var _ warehousehttp.StockFreezeAuthenticator = (*OperatorStockFreezeAuthenticator)(nil)

func NewOperatorStockFreezeAuthenticator(minter Minter) (*OperatorStockFreezeAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: stock freeze authenticator needs a minter")
	}
	return &OperatorStockFreezeAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorStockFreezeAuthenticator) AuthenticateStockFreeze(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionStockFreeze)
}
