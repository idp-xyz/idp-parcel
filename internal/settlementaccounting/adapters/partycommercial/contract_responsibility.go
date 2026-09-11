package partycommercial

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// UnconfiguredContractResponsibility 是 ports.ContractResponsibilityView 的显式未配置实现：对任何
// （租户、客户、税费义务）都答 found=false，编排据此停在 `CONTRACT_UNCONFIGURED` 未决——回收需要合同依据，
// 未配置不默认可回收（UC-SA-001 步 5「不以当前合同覆盖历史；未配置时保持待判断」）。
//
// 它不是替身：合同责任目录属实例半边（`PAR-SET-08`），今天没有租户，party-commercial 侧也还没有按
// （客户 + 税费义务）交合同责任的读口；生产装配要一个诚实的口而不是 nil——nil 在 FormRecovery 走到那一步时
// 是 panic，与「未配置」在恢复动作上完全不同。租户出现、PC 读口落成那天，换成读 PC 的适配器，编排一行不改。
type UnconfiguredContractResponsibility struct{}

var _ ports.ContractResponsibilityView = UnconfiguredContractResponsibility{}

func (UnconfiguredContractResponsibility) LoadContractResponsibility(
	context.Context,
	domain.TenantID,
	domain.RecoveryCustomerReference,
	domain.TaxObligationReference,
) (domain.ContractResponsibilityReference, bool, error) {
	return domain.ContractResponsibilityReference{}, false, nil
}
