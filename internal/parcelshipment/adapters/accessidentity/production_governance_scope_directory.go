package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	pspilot "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ProductionGovernanceScopeDirectory 是 pspilot.GovernanceScopeDirectory 的生产实现（ADR-0308）：读接入身份的准入范围对照册，
// 按（租户、能力、事实类型）取这一刻生效的对象范围，换成治理坐标。
//
// 只换算、不代拟：租户认不出、对照没登、登了而此刻不生效，都答「显式未配置」（false），编排照旧答`权威未确定`；对照读不动、
// 同键两行同时生效（绕过登记口直插的记录错误）答错误，不折成缺席——缺席与故障的运维动作不同（ADR-0029）。
// 对象范围上此刻有没有本产品的区间不在这里判，那是 ProductionOwnershipAdapter 读治理区间的事。
type ProductionGovernanceScopeDirectory struct {
	mappings identity.AdmissionScopeMappingReader
	clock    psports.Clock
}

var _ pspilot.GovernanceScopeDirectory = (*ProductionGovernanceScopeDirectory)(nil)

func NewProductionGovernanceScopeDirectory(
	mappings identity.AdmissionScopeMappingReader,
	clock psports.Clock,
) (*ProductionGovernanceScopeDirectory, error) {
	if mappings == nil || clock == nil {
		return nil, errors.New("parcel shipment accessidentity: production governance scope directory needs a mapping reader and a clock")
	}
	return &ProductionGovernanceScopeDirectory{mappings: mappings, clock: clock}, nil
}

func (directory *ProductionGovernanceScopeDirectory) FindGovernanceScope(
	ctx context.Context,
	scope psdomain.AdmissionScope,
) (pspilot.GovernanceScope, bool, error) {
	// 租户从拟受理范围认回，不另解析引用串；认不出（不是按客户账户立的那一形）就无从证明它属哪个租户。
	tenant, _, recognised := scope.CustomerAccount()
	if !recognised {
		return pspilot.GovernanceScope{}, false, nil
	}
	requirement := identity.ShipmentRequestSubmissionAdmission()
	mappings, err := directory.mappings.FindAdmissionScopeMappings(ctx, tenant.String(), requirement)
	if err != nil {
		return pspilot.GovernanceScope{}, false, fmt.Errorf("read admission scope mappings: %w", err)
	}
	objectScope, found, err := identity.ResolveAdmissionObjectScope(mappings, directory.clock.Now().UTC())
	if err != nil {
		return pspilot.GovernanceScope{}, false, err
	}
	if !found {
		return pspilot.GovernanceScope{}, false, nil
	}
	governance, err := pspilot.NewGovernanceScopeOfObjectScope(objectScope, requirement.Capability, requirement.FactKind)
	if err != nil {
		return pspilot.GovernanceScope{}, false, err
	}
	return governance, true, nil
}
