package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// OperatorStockCountAuthenticator 认证盘点。创建、开始、完成与作废共用 STOCK_COUNT。
type OperatorStockCountAuthenticator struct {
	minter Minter
}

var _ warehousehttp.StockCountAuthenticator = (*OperatorStockCountAuthenticator)(nil)

func NewOperatorStockCountAuthenticator(minter Minter) (*OperatorStockCountAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: stock count authenticator needs a minter")
	}
	return &OperatorStockCountAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorStockCountAuthenticator) AuthenticateStockCount(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionStockCount)
}

// OperatorStockApprovalAuthenticator 认证盘点差额的批准与驳回。
type OperatorStockApprovalAuthenticator struct {
	minter Minter
}

var _ warehousehttp.StockApprovalAuthenticator = (*OperatorStockApprovalAuthenticator)(nil)

func NewOperatorStockApprovalAuthenticator(minter Minter) (*OperatorStockApprovalAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: stock approval authenticator needs a minter")
	}
	return &OperatorStockApprovalAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorStockApprovalAuthenticator) AuthenticateStockApproval(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionStockAdjustmentApproval)
}

// OperatorStockAdjustmentAuthenticator 认证库存调整的提交。
type OperatorStockAdjustmentAuthenticator struct {
	minter Minter
}

var _ warehousehttp.StockAdjustmentAuthenticator = (*OperatorStockAdjustmentAuthenticator)(nil)

func NewOperatorStockAdjustmentAuthenticator(minter Minter) (*OperatorStockAdjustmentAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: stock adjustment authenticator needs a minter")
	}
	return &OperatorStockAdjustmentAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorStockAdjustmentAuthenticator) AuthenticateStockAdjustment(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionStockAdjustmentSubmission)
}
