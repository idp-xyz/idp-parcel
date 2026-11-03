package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ExpectedCostAwaitingSupplier 是一版还没记供应商的预期成本：补记要的只有它的版本与它所依的那一版协议。
type ExpectedCostAwaitingSupplier struct {
	Version   domain.SupplierCostVersionID
	Agreement domain.SupplierAgreementReference
}

// ExpectedCostSupplierBackfill 是补记旧行供应商的两问（ADR-0358 决定三）。旧行是本记录之前形成的预期成本：迁移只加了一列
// 可空的供应商、没有跨上下文回填，补记由运营经补记口按各自的协议问 party-commercial 之后写下。
type ExpectedCostSupplierBackfill interface {
	// ListExpectedCostsAwaitingSupplier 按版本序交回这个租户里还没记供应商的各版。
	ListExpectedCostsAwaitingSupplier(
		ctx context.Context,
		tenant domain.TenantID,
	) ([]ExpectedCostAwaitingSupplier, error)
	// RecordExpectedCostSupplier 只在这一版的供应商仍为空时写下，recorded=true；已记过（不论记的是哪一家）或这一版不在这个
	// 租户的册上，recorded=false、什么都不改——已记的一格不改，是「记下后不改」在库上的那一半。
	RecordExpectedCostSupplier(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SupplierCostVersionID,
		supplier domain.SupplierPartyReference,
	) (bool, error)
}
