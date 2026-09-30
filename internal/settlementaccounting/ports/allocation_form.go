package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// AllocationFormRegister 写下一条分摊规则版本选用的分法。读口是 AllocationFormView。
// 分摊规则适用表只存版本引用，分法不写回去。各对象的权重也不在这本册里。
type AllocationFormRegister interface {
	SaveAllocationForm(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.AllocationFormRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// AllocationFormView 按分摊规则版本取选用的分法。found=false 表示没有这一行：
// 答未配置，不默认按重，也不均摊。
type AllocationFormView interface {
	LoadAllocationForm(
		ctx context.Context,
		tenant domain.TenantID,
		rule domain.AllocationRuleVersionReference,
	) (domain.AllocationForm, bool, error)
}
