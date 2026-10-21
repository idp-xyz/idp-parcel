package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ExpectedCostSuccessionView 答一版供应商预期成本有没有后继：别的哪一版回指它（ADR-0232 决定七、八）。供应商账单逐行匹配据它判点名的
// 那一版是不是此刻的当前有效预期成本——被纠正过的一版说的已不是这份成本此刻的样子。另立文件、不加进 ExpectedCostView：那一口有别处的替身。
type ExpectedCostSuccessionView interface {
	LoadExpectedCostSuccessor(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SupplierCostVersionID,
	) (domain.SupplierCostVersionID, bool, error)
}
