package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// PreAdviceRead 是入库预报的查阅口，放在 PreAdviceStore 旁边。写册的方法仍只服务受理、告知、撤回与收货缝；
// 列表、按货主读一份、按单据号逐字找不进那张写面。
//
// 读不动走 error，不折成查无。没有单据号的预报不出现在按单据号这一口：那一列是 NULL，逐字相等碰不到。
type PreAdviceRead interface {
	ListPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[domain.PreAdvice], error)
	PreAdviceForOwner(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		id domain.PreAdviceID,
	) (domain.PreAdvice, bool, error)
	PreAdviceByDocumentNumber(
		ctx context.Context,
		tenant domain.TenantID,
		number domain.DocumentNumber,
	) (domain.PreAdvice, bool, error)
}
